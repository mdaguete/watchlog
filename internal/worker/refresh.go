package worker

import (
	"log"
	"strings"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/tmdb"
)

// RunTMDBRefresh performs a full TMDB metadata refresh for all shows and movies.
// It fetches metadata in both languages and caches episode details.
func RunTMDBRefresh(database *db.DB, client *tmdb.Client) {
	if client == nil || !client.Enabled() {
		return
	}

	shows, _ := database.GetAllShowsWithTMDB()
	log.Printf("TMDB REFRESH (worker): updating %d shows...", len(shows))
	updated := 0
	for _, show := range shows {
		// Fetch in Spanish (primary)
		result, err := client.GetTVShowLang(show.TMDBID, "es-ES")
		if err != nil {
			continue
		}
		genres := extractGenreNames(result.Genres)
		database.UpdateShowTMDB(show.ID, result.ID, tmdb.PosterURL(result.PosterPath, "w342"), tmdb.BackdropURL(result.BackdropPath, "w780"), result.Overview, genres, result.Status, len(result.Seasons))
		// Fetch in English
		resultEN, err := client.GetTVShowLang(show.TMDBID, "en-US")
		if err == nil {
			database.UpdateShowTMDBEN(show.ID, resultEN.Overview, extractGenreNames(resultEN.Genres))
			database.UpdateShowTMDBNames(show.ID, result.Name, resultEN.Name)
		}
		// Cache season episode counts and episode details
		for _, s := range result.Seasons {
			if s.SeasonNumber == 0 {
				continue
			}
			database.UpsertSeasonEpisodes(show.ID, s.SeasonNumber, s.EpisodeCount)

			seasonES, err := client.GetSeasonLang(show.TMDBID, s.SeasonNumber, "es-ES")
			if err != nil {
				continue
			}
			seasonEN, _ := client.GetSeasonLang(show.TMDBID, s.SeasonNumber, "en-US")
			for _, ep := range seasonES.Episodes {
				d := db.EpisodeDetail{
					ShowID:        show.ID,
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
		updated++
	}

	movies, _ := database.GetAllMoviesWithTMDB()
	log.Printf("TMDB REFRESH (worker): updating %d movies...", len(movies))
	moviesUpdated := 0
	for _, movie := range movies {
		detail, err := client.GetMovieLang(movie.TMDBID, "es-ES")
		if err != nil {
			continue
		}
		genres := extractGenreNames(detail.Genres)
		database.UpdateMovieTMDB(movie.ID, detail.ID, tmdb.PosterURL(detail.PosterPath, "w342"), detail.Overview, genres, detail.Runtime)
		detailEN, err := client.GetMovieLang(movie.TMDBID, "en-US")
		if err == nil {
			database.UpdateMovieTMDBEN(movie.ID, detailEN.Overview, extractGenreNames(detailEN.Genres))
			database.UpdateMovieTMDBNames(movie.ID, detail.Title, detailEN.Title)
		}
		moviesUpdated++
	}

	log.Printf("TMDB REFRESH (worker): complete — shows %d/%d, movies %d/%d", updated, len(shows), moviesUpdated, len(movies))
}

func extractGenreNames(genres []tmdb.Genre) string {
	names := make([]string, len(genres))
	for i, g := range genres {
		names[i] = g.Name
	}
	return strings.Join(names, ", ")
}
