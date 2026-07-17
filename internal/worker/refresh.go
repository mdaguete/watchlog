package worker

import (
	"errors"
	"log"
	"strings"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/models"
	"github.com/mdaguete/watchlog/internal/tmdb"
)

var errTMDBDisabled = errors.New("tmdb not configured")

// CacheProviders fetches the streaming providers for a title in the given
// region and stores them in the per-region provider cache. mediaType is "tv" or
// "movie". Best-effort: silent on error so it never blocks enrichment.
func CacheProviders(database *db.DB, client *tmdb.Client, mediaType string, tmdbID int, region string) {
	if client == nil || !client.Enabled() || tmdbID == 0 {
		return
	}
	provs, err := client.GetWatchProviders(mediaType, tmdbID, region)
	if err != nil {
		return
	}
	ms := make([]models.Provider, 0, len(provs))
	for _, p := range provs {
		ms = append(ms, models.Provider{Name: p.Name, LogoPath: p.LogoPath})
	}
	database.UpsertProviderCache(mediaType, tmdbID, region, ms)
}

// releaseYear returns the 4-digit year from a "YYYY-..." date string, or "".
func releaseYear(s string) string {
	if len(s) >= 4 {
		return s[:4]
	}
	return ""
}

func extractGenreNames(genres []tmdb.Genre) string {
	names := make([]string, len(genres))
	for i, g := range genres {
		names[i] = g.Name
	}
	return strings.Join(names, ", ")
}

// RefreshShowByTMDB (re)fetches a single show's metadata from the given TMDB id
// in both languages and caches its season counts and episode details. It updates
// the show's tmdb_id, so it doubles as the "re-link to the correct TMDB entry"
// operation. User watch history is not touched (episodes are keyed by show id).
func RefreshShowByTMDB(database *db.DB, client *tmdb.Client, showID int64, tmdbID int) error {
	if client == nil || !client.Enabled() {
		return errTMDBDisabled
	}
	result, err := client.GetTVShowLang(tmdbID, "es-ES")
	if err != nil {
		return err
	}
	genres := extractGenreNames(result.Genres)
	database.UpdateShowTMDB(showID, result.ID, tmdb.PosterURL(result.PosterPath, "w342"), tmdb.BackdropURL(result.BackdropPath, "w780"), result.Overview, genres, result.Status, len(result.Seasons))
	if resultEN, err := client.GetTVShowLang(tmdbID, "en-US"); err == nil {
		database.UpdateShowTMDBEN(showID, resultEN.Overview, extractGenreNames(resultEN.Genres))
		database.UpdateShowTMDBNames(showID, result.Name, resultEN.Name)
	}
	newSeasonCount := 0
	for _, s := range result.Seasons {
		if s.SeasonNumber > 0 {
			newSeasonCount++
		}
	}
	database.UnarchiveForNewSeason(showID, newSeasonCount)
	// Drop stale season/episode caches (e.g. leftovers from a previous wrong
	// TMDB match) before repopulating from the current show. User watch history
	// lives in the episodes table and is not affected.
	database.ClearSeasonEpisodes(showID)
	database.ClearEpisodeDetails(showID)
	for _, s := range result.Seasons {
		if s.SeasonNumber == 0 {
			continue
		}
		database.UpsertSeasonEpisodes(showID, s.SeasonNumber, s.EpisodeCount)

		seasonES, err := client.GetSeasonLang(tmdbID, s.SeasonNumber, "es-ES")
		if err != nil {
			continue
		}
		seasonEN, _ := client.GetSeasonLang(tmdbID, s.SeasonNumber, "en-US")
		for _, ep := range seasonES.Episodes {
			d := db.EpisodeDetail{
				ShowID:        showID,
				SeasonNumber:  ep.SeasonNumber,
				EpisodeNumber: ep.EpisodeNumber,
				Name:          ep.Name,
				Overview:      ep.Overview,
				AirDate:       ep.AirDate,
				Runtime:       ep.Runtime,
				StillURL:      tmdb.BackdropURL(ep.StillPath, "w300"),
			}
			if seasonEN != nil {
				for _, epEN := range seasonEN.Episodes {
					if epEN.EpisodeNumber == ep.EpisodeNumber {
						d.NameEN = epEN.Name
						d.OverviewEN = epEN.Overview
						break
					}
				}
			}
			database.UpsertEpisodeDetail(d)
		}
	}
	return nil
}

// RunTMDBRefresh performs a full TMDB metadata refresh for all shows and movies.
// It fetches metadata in both languages and caches episode details.
// Returns the number of shows and movies updated. Progress is logged per item.
func RunTMDBRefresh(database *db.DB, client *tmdb.Client) (int, int) {
	if client == nil || !client.Enabled() {
		return 0, 0
	}

	shows, _ := database.GetAllShowsWithTMDB()
	log.Printf("TMDB REFRESH: updating %d shows (es+en)...", len(shows))
	updated := 0
	for i, show := range shows {
		tmdbID := show.TMDBID
		// Verify the mapping against TheTVDB (TVTime's external_id is a TheTVDB
		// series id). If TMDB's authoritative tvdb->tmdb mapping disagrees with
		// the stored id, the show was mis-matched by name — correct it.
		// (external_id == tmdb_id means the show was added directly from TMDB, skip.)
		if show.ExternalID > 0 && show.ExternalID != int64(show.TMDBID) {
			if correct, ok := client.FindTMDBIDByTVDB(int(show.ExternalID)); ok && correct != show.TMDBID {
				log.Printf("TMDB REFRESH [%d/%d] correcting %q tmdb_id %d -> %d (tvdb=%d)", i+1, len(shows), show.Name, show.TMDBID, correct, show.ExternalID)
				tmdbID = correct
			}
		}
		if err := RefreshShowByTMDB(database, client, show.ID, tmdbID); err != nil {
			log.Printf("TMDB REFRESH [%d/%d] ✗ %q: %v", i+1, len(shows), show.Name, err)
			continue
		}
		updated++
		log.Printf("TMDB REFRESH [%d/%d] ✓ %q (tmdb_id=%d)", i+1, len(shows), show.Name, tmdbID)
	}

	movies, _ := database.GetAllMoviesWithTMDB()
	log.Printf("TMDB REFRESH: updating %d movies (es+en)...", len(movies))
	moviesUpdated := 0
	for i, movie := range movies {
		tmdbID := movie.TMDBID
		storedYear := releaseYear(movie.ReleaseDate)
		detail, err := client.GetMovieLang(tmdbID, "es-ES")
		if err != nil {
			log.Printf("TMDB REFRESH movie [%d/%d] ✗ %q: %v", i+1, len(movies), movie.Name, err)
			continue
		}
		// Movies have no authoritative external id, so verify by release year:
		// if TVTime's year is known and the linked movie's year disagrees, the
		// match is likely wrong — re-resolve by name+year and re-link.
		if storedYear != "" && releaseYear(detail.ReleaseDate) != storedYear {
			if id, ok := client.ResolveMovieID(movie.Name, storedYear); ok && id != tmdbID {
				if d2, err2 := client.GetMovieLang(id, "es-ES"); err2 == nil {
					log.Printf("TMDB REFRESH movie [%d/%d] correcting %q tmdb_id %d -> %d (year=%s)", i+1, len(movies), movie.Name, tmdbID, id, storedYear)
					tmdbID = id
					detail = d2
				}
			}
		}
		genres := extractGenreNames(detail.Genres)
		database.UpdateMovieTMDB(movie.ID, detail.ID, tmdb.PosterURL(detail.PosterPath, "w342"), detail.Overview, genres, detail.Runtime)
		detailEN, err := client.GetMovieLang(tmdbID, "en-US")
		if err == nil {
			database.UpdateMovieTMDBEN(movie.ID, detailEN.Overview, extractGenreNames(detailEN.Genres))
			database.UpdateMovieTMDBNames(movie.ID, detail.Title, detailEN.Title)
		}
		moviesUpdated++
		log.Printf("TMDB REFRESH movie [%d/%d] ✓ %q", i+1, len(movies), movie.Name)
	}

	log.Printf("TMDB REFRESH: complete — shows %d/%d, movies %d/%d", updated, len(shows), moviesUpdated, len(movies))
	return updated, moviesUpdated
}
