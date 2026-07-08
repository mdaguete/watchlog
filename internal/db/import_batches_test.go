package db

import (
	"os"
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/models"
)

func newImportTestDB(t *testing.T) (*DB, int64) {
	t.Helper()
	f, _ := os.CreateTemp("", "wl-import-test-*.db")
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	database, err := New(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	hash, _ := auth.HashPassword("x")
	uid, _ := database.CreateUser("u", hash)
	return database, uid
}

func TestImportUnmatchedLifecycle(t *testing.T) {
	database, uid := newImportTestDB(t)
	batchID, err := database.CreateImportBatch(uid, "netflix", "f.csv", 10, 2, []string{"Arcane"})
	if err != nil {
		t.Fatal(err)
	}
	err = database.AddImportUnmatched(batchID, []UnmatchedEntry{
		{Kind: "series", NetflixName: "Arcane", Season: 1, NetflixEp: "Bienvenidos", WatchedDate: "2022-01-01"},
		{Kind: "series", NetflixName: "Arcane", Season: 1, NetflixEp: "Algo", WatchedDate: "2022-01-02"},
		{Kind: "movie", NetflixName: "Tyler Rake", WatchedDate: "2020-05-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	groups, _ := database.ListUnmatchedGroups(batchID)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if database.CountUnmatchedGroups(batchID) != 2 {
		t.Errorf("count mismatch")
	}
	arcane, _ := database.ListUnmatchedEntries(batchID, "Arcane")
	if len(arcane) != 2 {
		t.Fatalf("expected 2 Arcane entries, got %d", len(arcane))
	}
	// Resolve Arcane -> only the movie group remains.
	database.MarkUnmatchedResolved(batchID, "Arcane")
	groups, _ = database.ListUnmatchedGroups(batchID)
	if len(groups) != 1 || groups[0].Name != "Tyler Rake" || groups[0].Kind != "movie" {
		t.Fatalf("expected only Tyler Rake movie left, got %+v", groups)
	}
}

func TestMarkEpisodeWatchedAt(t *testing.T) {
	database, uid := newImportTestDB(t)
	showID, _ := database.UpsertShow(models.Show{ExternalID: 5, Name: "Show"})
	at := time.Date(2020, 4, 3, 12, 0, 0, 0, time.Local)
	if err := database.MarkEpisodeWatchedAt(uid, showID, 1, 1, at); err != nil {
		t.Fatal(err)
	}
	got := database.GetEpisodeWatchedAt(uid, showID, 1, 1)
	if got == "" || got[:10] != "2020-04-03" {
		t.Errorf("expected 2020-04-03, got %q", got)
	}
	// Idempotent update to a new date.
	at2 := time.Date(2020, 4, 5, 12, 0, 0, 0, time.Local)
	database.MarkEpisodeWatchedAt(uid, showID, 1, 1, at2)
	if got := database.GetEpisodeWatchedAt(uid, showID, 1, 1); got[:10] != "2020-04-05" {
		t.Errorf("expected updated 2020-04-05, got %q", got)
	}
}
