package db

import (
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/models"
)

// TestWatchedAt_ISOFormat verifies marks store watched_at in a format SQLite
// can parse natively (datetime() returns non-empty).
func TestWatchedAt_ISOFormat(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})

	if err := db.MarkEpisodeWatched(uid, sid, 1, 1); err != nil {
		t.Fatalf("MarkEpisodeWatched: %v", err)
	}
	var raw, parsed string
	db.conn.QueryRow("SELECT watched_at, COALESCE(datetime(watched_at),'') FROM episodes WHERE user_id=? AND show_id=?", uid, sid).Scan(&raw, &parsed)
	if parsed == "" {
		t.Errorf("datetime(%q) failed to parse — not native SQLite datetime", raw)
	}
}

func TestUpdateEpisodeWatchedAt(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.MarkEpisodeWatched(uid, sid, 2, 3)

	want := time.Date(2021, 5, 4, 20, 30, 0, 0, time.UTC)
	n, err := db.UpdateEpisodeWatchedAt(uid, sid, 2, 3, want)
	if err != nil || n != 1 {
		t.Fatalf("UpdateEpisodeWatchedAt: n=%d err=%v", n, err)
	}
	var d, hm string
	db.conn.QueryRow("SELECT date(watched_at), strftime('%H:%M', watched_at) FROM episodes WHERE user_id=? AND show_id=? AND season_number=2 AND episode_number=3", uid, sid).Scan(&d, &hm)
	if d != "2021-05-04" {
		t.Errorf("date(watched_at) = %q, want 2021-05-04", d)
	}
	if hm != "20:30" {
		t.Errorf("time(watched_at) = %q, want 20:30", hm)
	}

	// Updating a non-existent episode affects no rows.
	n, _ = db.UpdateEpisodeWatchedAt(uid, sid, 9, 9, want)
	if n != 0 {
		t.Errorf("update of missing episode affected %d rows, want 0", n)
	}
}

func TestMigrateV13_NormalizesGoFormat(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	// Insert a legacy Go-format value directly, bypassing the driver's
	// datetime handling by writing to a temp column-free statement.
	db.conn.Exec(`INSERT INTO episodes (user_id, external_id, show_id, season_number, episode_number, watched, watched_at, runtime) VALUES (?,0,?,1,1,1,?,0)`,
		uid, sid, "2021-09-16 21:13:47 +0000 UTC")

	tx, _ := db.conn.Begin()
	if err := migrateV13(tx); err != nil {
		tx.Rollback()
		t.Fatalf("migrateV13: %v", err)
	}
	tx.Commit()

	var d, parsed string
	db.conn.QueryRow("SELECT date(watched_at), COALESCE(datetime(watched_at),'') FROM episodes WHERE user_id=? AND show_id=?", uid, sid).Scan(&d, &parsed)
	if d != "2021-09-16" {
		t.Errorf("normalized date = %q, want 2021-09-16", d)
	}
	if parsed == "" {
		t.Error("normalized value is not parseable by SQLite datetime()")
	}
}
