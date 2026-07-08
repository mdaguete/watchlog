package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/mdaguete/watchlog/internal/models"
)

func TestMarkEpisodeWatched_UpdatesStats(t *testing.T) {
	h, userID, token := newTestHandler(t)
	showID, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})

	body := []byte(`{"season":1,"episode":1}`)
	req := authedRequest("POST", "/api/shows/1/episodes/watched", token, body)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	h.APIMarkEpisodeWatched(rec, req)

	stats, _ := h.DB.GetUserWatchStats(userID)
	total := 0
	for _, s := range stats {
		total += s.Count
	}
	if total < 1 {
		t.Fatalf("expected watch stats to reflect the marked episode, got total=%d", total)
	}
	_ = showID
}
