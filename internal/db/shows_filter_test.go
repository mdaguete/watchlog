package db

import (
	"testing"

	"github.com/mdaguete/watchlog/internal/models"
)

func hasShow(shows []models.UserShow, id int64) bool {
	for _, s := range shows {
		if s.ID == id {
			return true
		}
	}
	return false
}

// TestShowsFilter_WatchingCompleted verifies the "watching" (episodes left) and
// "completed" (caught up) filters based on available vs watched episodes.
func TestShowsFilter_WatchingCompleted(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.FollowShow(uid, sid)
	db.UpsertSeasonEpisodes(sid, 1, 8) // 8 available

	// Nothing watched yet -> watching, not completed.
	w, _ := db.GetUserShowsFiltered(uid, "name", "watching")
	c, _ := db.GetUserShowsFiltered(uid, "name", "completed")
	if !hasShow(w, sid) {
		t.Error("show with unwatched episodes should be in 'watching'")
	}
	if hasShow(c, sid) {
		t.Error("show with unwatched episodes should not be 'completed'")
	}

	// Watch all 8 -> completed, not watching.
	for ep := 1; ep <= 8; ep++ {
		db.MarkEpisodeWatched(uid, sid, 1, ep)
	}
	w2, _ := db.GetUserShowsFiltered(uid, "name", "watching")
	c2, _ := db.GetUserShowsFiltered(uid, "name", "completed")
	if hasShow(w2, sid) {
		t.Error("caught-up show should not be in 'watching'")
	}
	if !hasShow(c2, sid) {
		t.Error("caught-up show should be 'completed'")
	}
}
