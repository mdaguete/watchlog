package db

import (
	"testing"

	"github.com/mdaguete/watchlog/internal/models"
)

func movieByExternal(t *testing.T, db *DB, ext string) (models.Movie, bool) {
	t.Helper()
	movies, err := db.GetMoviesWithoutTMDB()
	if err != nil {
		t.Fatalf("GetMoviesWithoutTMDB: %v", err)
	}
	for _, m := range movies {
		if m.ExternalID == ext {
			return m, true
		}
	}
	return models.Movie{}, false
}

func TestMovieReleaseDate_Persisted(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.UpsertMovie(models.Movie{ExternalID: "uuid-1", Name: "F1", ReleaseDate: "2025-06-27"}); err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}
	m, ok := movieByExternal(t, db, "uuid-1")
	if !ok || m.ReleaseDate != "2025-06-27" {
		t.Errorf("release_date = %q (ok=%v), want 2025-06-27", m.ReleaseDate, ok)
	}
}

// TestMovieReleaseDate_ReimportBackfill verifies a re-import (upsert on the same
// external_id) backfills release_date for a movie that lacked it — enabling the
// re-import -> refresh fix flow.
func TestMovieReleaseDate_ReimportBackfill(t *testing.T) {
	db := newTestDB(t)
	// First import: no release date (older import).
	db.UpsertMovie(models.Movie{ExternalID: "uuid-2", Name: "Titanic"})
	if m, _ := movieByExternal(t, db, "uuid-2"); m.ReleaseDate != "" {
		t.Fatalf("expected empty release_date initially, got %q", m.ReleaseDate)
	}
	// Re-import with the date now captured.
	db.UpsertMovie(models.Movie{ExternalID: "uuid-2", Name: "Titanic", ReleaseDate: "1997-12-19"})
	m, _ := movieByExternal(t, db, "uuid-2")
	if m.ReleaseDate != "1997-12-19" {
		t.Errorf("release_date after re-import = %q, want 1997-12-19", m.ReleaseDate)
	}
}
