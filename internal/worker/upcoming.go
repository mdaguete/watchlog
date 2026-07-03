package worker

import (
	"context"
	"log"
	"time"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/tmdb"
)

// StartUpcomingRefresher runs a background goroutine that refreshes
// the upcoming episodes cache once per day. It also runs immediately on start.
// The goroutine stops when the context is cancelled.
func StartUpcomingRefresher(ctx context.Context, database *db.DB, client *tmdb.Client) {
	if client == nil || !client.Enabled() {
		log.Println("Worker: upcoming refresher disabled (no TMDB key)")
		return
	}

	go func() {
		log.Println("Worker: initial upcoming cache refresh starting...")
		RefreshUpcomingCache(database, client)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("Worker: upcoming refresher stopped")
				return
			case <-ticker.C:
				log.Println("Worker: daily upcoming cache refresh starting...")
				RefreshUpcomingCache(database, client)
			}
		}
	}()
}

// RefreshUpcomingCache fetches upcoming episodes from TMDB for all active shows.
func RefreshUpcomingCache(database *db.DB, client *tmdb.Client) {
	shows, err := database.GetActiveShowsWithTMDB()
	if err != nil {
		log.Printf("Worker: error getting active shows: %v", err)
		return
	}

	log.Printf("Worker: checking %d active shows for upcoming episodes...", len(shows))
	updated := 0
	removed := 0
	errors := 0

	for i, show := range shows {
		ep, err := client.GetNextEpisodeToAir(show.TMDBID)
		if err != nil {
			log.Printf("Worker [%d/%d] ✗ %q (tmdb=%d): %v", i+1, len(shows), show.Name, show.TMDBID, err)
			errors++
			continue
		}

		if ep == nil {
			database.DeleteUpcomingCache(show.ID)
			removed++
			continue
		}

		if ep.AirDate != "" && isDatePast(ep.AirDate) {
			database.DeleteUpcomingCache(show.ID)
			removed++
			continue
		}

		database.UpsertUpcomingCache(show.ID, ep.ShowName, show.PosterURL, ep.EpisodeName, ep.Season, ep.Episode, ep.AirDate, ep.Overview)
		updated++
		log.Printf("Worker [%d/%d] ✓ %q → S%02dE%02d %s", i+1, len(shows), show.Name, ep.Season, ep.Episode, ep.AirDate)
	}

	log.Printf("Worker: upcoming cache complete — %d cached, %d removed, %d errors, %d total", updated, removed, errors, len(shows))
}

func isDatePast(dateStr string) bool {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return false
	}
	return t.Before(time.Now().Truncate(24 * time.Hour))
}
