package handlers

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/importer"
	"github.com/mdaguete/watchlog/internal/tmdb"
	"github.com/mdaguete/watchlog/internal/worker"
)

// tmdbCandidate is a search result shown to the user for reconciliation.
type tmdbCandidate struct {
	TMDBID    int
	Title     string
	Year      string
	Overview  string
	PosterURL string
}

// HandleHistoryTMDBSearch searches TMDB for an unmatched series/movie name and
// renders candidate cards (HTMX partial).
func (h *Handler) HandleHistoryTMDBSearch(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	lang := h.getLang(r, userID)
	batchID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.DB.GetImportBatchForUser(batchID, userID); err != nil {
		http.NotFound(w, r)
		return
	}
	if h.TMDB == nil || !h.TMDB.Enabled() {
		http.Error(w, "TMDB not configured", http.StatusServiceUnavailable)
		return
	}
	name := r.FormValue("name")
	kind := r.FormValue("kind")
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}

	var candidates []tmdbCandidate
	if kind == "movie" {
		results, _ := h.TMDB.SearchMovieLang(name, "es-ES")
		for i, m := range results {
			if i >= 5 {
				break
			}
			candidates = append(candidates, tmdbCandidate{
				TMDBID: m.ID, Title: m.Title, Year: yearOf(m.ReleaseDate),
				Overview: truncate(m.Overview, 220), PosterURL: tmdb.PosterURL(m.PosterPath, "w342"),
			})
		}
	} else {
		results, _ := h.TMDB.SearchTVLang(name, "es-ES")
		for i, s := range results {
			if i >= 5 {
				break
			}
			candidates = append(candidates, tmdbCandidate{
				TMDBID: s.ID, Title: s.Name, Year: yearOf(s.FirstAirDate),
				Overview: truncate(s.Overview, 220), PosterURL: tmdb.PosterURL(s.PosterPath, "w342"),
			})
		}
	}

	h.Templates.ExecuteTemplate(w, "history_tmdb_candidates", map[string]any{
		"Lang":       lang,
		"BatchID":    batchID,
		"Name":       name,
		"Kind":       kind,
		"Candidates": candidates,
	})
}

// HandleHistoryResolve adds the chosen TMDB show/movie to the library and marks
// all of the unmatched Netflix entries for that name as watched with their
// Netflix dates. Backs up the DB first.
func (h *Handler) HandleHistoryResolve(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	lang := h.getLang(r, userID)
	batchID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.DB.GetImportBatchForUser(batchID, userID); err != nil {
		http.NotFound(w, r)
		return
	}
	if h.TMDB == nil || !h.TMDB.Enabled() {
		http.Error(w, "TMDB not configured", http.StatusServiceUnavailable)
		return
	}
	name := r.FormValue("name")
	kind := r.FormValue("kind")
	tmdbID, errID := strconv.Atoi(r.FormValue("tmdb_id"))
	if name == "" || errID != nil || tmdbID <= 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	entries, err := h.DB.ListUnmatchedEntries(batchID, name)
	if err != nil || len(entries) == 0 {
		http.Error(w, "no entries", http.StatusBadRequest)
		return
	}

	// Backup before writing.
	if _, err := h.DB.Backup("history-reconcile"); err != nil {
		log.Printf("history reconcile: backup error: %v", err)
		http.Error(w, "backup failed", http.StatusInternalServerError)
		return
	}

	applied := 0
	if kind == "movie" {
		movie, err := h.TMDB.GetMovie(tmdbID)
		if err != nil {
			http.Error(w, "tmdb error", http.StatusBadGateway)
			return
		}
		genres := extractGenreNames(movie.Genres)
		movieID, _ := h.DB.AddMovieFromTMDB(movie.ID, movie.Title, tmdb.PosterURL(movie.PosterPath, "w342"), movie.Overview, genres, movie.Runtime)
		h.DB.AddMovieToLibrary(userID, movieID)
		// Oldest Netflix date across entries.
		oldest := time.Time{}
		for _, e := range entries {
			d, err := time.ParseInLocation("2006-01-02", e.WatchedDate, time.Local)
			if err != nil {
				continue
			}
			if oldest.IsZero() || d.Before(oldest) {
				oldest = d
			}
		}
		if !oldest.IsZero() {
			oldest = time.Date(oldest.Year(), oldest.Month(), oldest.Day(), 12, 0, 0, 0, time.Local)
			h.DB.MarkMovieWatched(userID, movieID, oldest)
			applied = 1
		}
		log.Printf("ACTION: user=%d reconcile movie %q -> tmdb %d (%s)", userID, name, tmdbID, movie.Title)
	} else {
		show, err := h.TMDB.GetTVShow(tmdbID)
		if err != nil {
			http.Error(w, "tmdb error", http.StatusBadGateway)
			return
		}
		genres := extractGenreNames(show.Genres)
		showID, _ := h.DB.AddShowFromTMDB(show.ID, show.Name, tmdb.PosterURL(show.PosterPath, "w342"), tmdb.BackdropURL(show.BackdropPath, "w780"), show.Overview, genres, show.Status, len(show.Seasons))
		h.DB.FollowShow(userID, showID)
		// Enrich episode_details so title matching works, then map + mark.
		if err := worker.RefreshShowByTMDB(h.DB, h.TMDB, showID, tmdbID); err != nil {
			log.Printf("history reconcile: refresh show error: %v", err)
		}
		nes := make([]importer.NetflixEntry, 0, len(entries))
		for _, e := range entries {
			d, err := time.ParseInLocation("2006-01-02", e.WatchedDate, time.Local)
			if err != nil {
				continue
			}
			nes = append(nes, importer.NetflixEntry{Series: name, Season: e.Season, EpTitle: e.NetflixEp, Date: d})
		}
		matches := importer.MatchSeriesEpisodes(h.DB, showID, nes)
		for _, m := range matches {
			at := time.Date(m.Date.Year(), m.Date.Month(), m.Date.Day(), 12, 0, 0, 0, time.Local)
			if err := h.DB.MarkEpisodeWatchedAt(userID, showID, m.Season, m.Episode, at); err != nil {
				log.Printf("history reconcile: mark episode error: %v", err)
				continue
			}
			applied++
		}
		// Record applied changes in the batch for transparency.
		rec := make([]db.ImportChange, 0, len(matches))
		disp := show.Name
		for _, m := range matches {
			rec = append(rec, db.ImportChange{
				Type: "episode", TargetID: showID, Title: disp,
				Season: m.Season, Episode: m.Episode, NetflixTitle: m.NetflixTitle,
				CurrentDate: "", NewDate: m.Date.Format("2006-01-02"),
			})
		}
		if len(rec) > 0 {
			h.recordAppliedChanges(batchID, rec)
		}
		log.Printf("ACTION: user=%d reconcile series %q -> tmdb %d (%s), %d episodes", userID, name, tmdbID, show.Name, applied)
	}

	h.DB.MarkUnmatchedResolved(batchID, name)

	// Return the refreshed unmatched section (HTMX swap).
	groups, _ := h.DB.ListUnmatchedGroups(batchID)
	h.Templates.ExecuteTemplate(w, "history_unmatched_section", map[string]any{
		"Lang":            lang,
		"Batch":           db.ImportBatch{ID: batchID},
		"UnmatchedGroups": groups,
		"TMDBEnabled":     true,
		"Resolved":        map[string]any{"Name": name, "Applied": applied},
	})
}

// recordAppliedChanges inserts already-applied changes into the batch record.
func (h *Handler) recordAppliedChanges(batchID int64, changes []db.ImportChange) {
	if err := h.DB.AddImportChangesApplied(batchID, changes); err != nil {
		log.Printf("history reconcile: record changes error: %v", err)
	}
}

func yearOf(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
