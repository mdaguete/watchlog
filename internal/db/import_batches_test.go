package db

import (
	"os"
	"strings"
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
	// Verify enriched aggregation for the Arcane series group.
	var arcaneG *UnmatchedGroup
	for i := range groups {
		if groups[i].Name == "Arcane" {
			arcaneG = &groups[i]
		}
	}
	if arcaneG == nil {
		t.Fatal("Arcane group missing")
	}
	if arcaneG.Count != 2 || arcaneG.SeasonsLabel != "T1" {
		t.Errorf("Arcane: expected 2 eps / T1, got count=%d seasons=%q", arcaneG.Count, arcaneG.SeasonsLabel)
	}
	if arcaneG.FirstDate != "2022-01-01" || arcaneG.LastDate != "2022-01-02" {
		t.Errorf("Arcane dates: got %q..%q", arcaneG.FirstDate, arcaneG.LastDate)
	}
	if !strings.Contains(arcaneG.Sample, "Bienvenidos") {
		t.Errorf("Arcane sample missing example title, got %q", arcaneG.Sample)
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

func TestListImportBatches_WithCounts(t *testing.T) {
	database, uid := newImportTestDB(t)
	// Two batches, each with changes, to exercise per-batch count subqueries
	// while iterating the batch cursor (regression: single-connection deadlock).
	for b := 0; b < 2; b++ {
		id, err := database.CreateImportBatch(uid, "netflix", "f.csv", 5, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := database.AddImportChanges(id, []ImportChange{
			{Type: "episode", TargetID: 1, Title: "X", Season: 1, Episode: 1, NewDate: "2020-01-01"},
			{Type: "movie", TargetID: 2, Title: "Y", NewDate: "2020-02-02"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	batches, err := database.ListImportBatches(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	for _, b := range batches {
		if b.TotalChanges != 2 || b.SelectedChanges != 2 {
			t.Errorf("batch %d: expected 2 total/selected changes, got total=%d selected=%d", b.ID, b.TotalChanges, b.SelectedChanges)
		}
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

func TestSyncWatchStatsFromDB_IdempotentAndNewPeriods(t *testing.T) {
	database, uid := newImportTestDB(t)
	showID, _ := database.UpsertShow(models.Show{ExternalID: 7, Name: "S"})
	// Two episodes in an old month (as a Netflix import would produce).
	database.MarkEpisodeWatchedAt(uid, showID, 1, 1, time.Date(2018, 3, 2, 12, 0, 0, 0, time.Local))
	database.MarkEpisodeWatchedAt(uid, showID, 1, 2, time.Date(2018, 3, 5, 12, 0, 0, 0, time.Local))
	// A movie in another old month.
	movieID, _ := database.UpsertMovie(models.Movie{ExternalID: "mv1", Name: "M"})
	database.MarkMovieWatched(uid, movieID, time.Date(2019, 7, 1, 12, 0, 0, 0, time.Local))

	if err := database.SyncWatchStatsFromDB(uid); err != nil {
		t.Fatal(err)
	}
	get := func() map[string]int {
		stats, _ := database.GetUserWatchStats(uid)
		m := map[string]int{}
		for _, s := range stats {
			m[s.Period] = s.Count
		}
		return m
	}
	m := get()
	if m["month-2018-03"] != 2 {
		t.Errorf("expected 2 episodes in 2018-03, got %d", m["month-2018-03"])
	}
	if m["month-2019-07"] != 1 {
		t.Errorf("expected 1 movie in 2019-07, got %d", m["month-2019-07"])
	}
	// Idempotent: calling again must not inflate counts.
	database.SyncWatchStatsFromDB(uid)
	m2 := get()
	if m2["month-2018-03"] != 2 || m2["month-2019-07"] != 1 {
		t.Errorf("not idempotent: got %+v", m2)
	}
}
