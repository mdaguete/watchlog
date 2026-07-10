package db

import (
	"testing"

	"github.com/mdaguete/watchlog/internal/models"
)

// TestGetUserMoviesSorted_BareWatchedAt guards against the regression where a
// watched_at stored as bare "YYYY-MM-DD HH:MM:SS" text (as produced by the v13
// normalization) could not be scanned into time.Time, making the movies page
// return an error and show nothing.
func TestGetUserMoviesSorted_BareWatchedAt(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	mid, _ := db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "Titanic"})
	// Insert a watched row with a bare (non-RFC3339) datetime, bypassing helpers.
	if _, err := db.conn.Exec("INSERT INTO user_movies (user_id, movie_id, watched_at) VALUES (?, ?, ?)", uid, mid, "1997-12-19 20:00:00"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	movies, err := db.GetUserMoviesSorted(uid, "recent")
	if err != nil {
		t.Fatalf("GetUserMoviesSorted returned error: %v", err)
	}
	if len(movies) != 1 {
		t.Fatalf("got %d movies, want 1", len(movies))
	}
	if movies[0].WatchedAt.Year() != 1997 {
		t.Errorf("watched year = %d, want 1997", movies[0].WatchedAt.Year())
	}
}
