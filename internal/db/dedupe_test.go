package db

import (
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/models"
)

func TestMigrateV15_MergesDuplicateShows(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")

	// Canonical show (tracked) with episode S1E1.
	canonical, _ := db.UpsertShow(models.Show{ExternalID: 100, Name: "Dup"})
	db.FollowShow(uid, canonical)
	db.MarkEpisodeWatched(uid, canonical, 1, 1)

	// Two duplicate per-episode shows (not tracked), each with one distinct episode.
	d1, _ := db.UpsertShow(models.Show{ExternalID: 200, Name: "Dup"})
	db.InsertEpisode(models.Episode{UserID: uid, ExternalID: 200, ShowID: d1, SeasonNumber: 1, EpisodeNumber: 2, Watched: true, WatchedAt: time.Now()})
	d2, _ := db.UpsertShow(models.Show{ExternalID: 201, Name: "Dup"})
	db.InsertEpisode(models.Episode{UserID: uid, ExternalID: 201, ShowID: d2, SeasonNumber: 1, EpisodeNumber: 3, Watched: true, WatchedAt: time.Now()})

	// Sanity: 3 shows named "Dup" before.
	var before int
	db.conn.QueryRow("SELECT COUNT(*) FROM shows WHERE name='Dup'").Scan(&before)
	if before != 3 {
		t.Fatalf("expected 3 duplicate shows, got %d", before)
	}

	tx, _ := db.conn.Begin()
	if err := migrateV15(tx); err != nil {
		tx.Rollback()
		t.Fatalf("migrateV15: %v", err)
	}
	tx.Commit()

	// Only the canonical remains.
	var after int
	db.conn.QueryRow("SELECT COUNT(*) FROM shows WHERE name='Dup'").Scan(&after)
	if after != 1 {
		t.Errorf("after merge = %d shows, want 1", after)
	}
	var remaining int64
	db.conn.QueryRow("SELECT id FROM shows WHERE name='Dup'").Scan(&remaining)
	if remaining != canonical {
		t.Errorf("surviving show id = %d, want canonical %d", remaining, canonical)
	}
	// All 3 episodes now live on the canonical show.
	eps, _ := db.GetEpisodesByShow(uid, canonical)
	if len(eps) != 3 {
		t.Errorf("canonical episodes = %d, want 3 (merged)", len(eps))
	}
	// Duplicate shows are gone.
	var dupEps int
	db.conn.QueryRow("SELECT COUNT(*) FROM episodes WHERE show_id IN (?, ?)", d1, d2).Scan(&dupEps)
	if dupEps != 0 {
		t.Errorf("episodes still on duplicate shows = %d, want 0", dupEps)
	}
}
