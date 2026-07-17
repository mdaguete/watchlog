package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/models"
)

func TestAPIFillAired(t *testing.T) {
	h, userID, token := newTestHandler(t)
	showID, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	// Two aired episodes with details, none watched yet.
	h.DB.UpsertEpisodeDetail(db.EpisodeDetail{ShowID: showID, SeasonNumber: 1, EpisodeNumber: 1, Name: "E1", AirDate: "2020-01-01"})
	h.DB.UpsertEpisodeDetail(db.EpisodeDetail{ShowID: showID, SeasonNumber: 1, EpisodeNumber: 2, Name: "E2", AirDate: "2020-01-08"})
	// A future episode should NOT be filled.
	h.DB.UpsertEpisodeDetail(db.EpisodeDetail{ShowID: showID, SeasonNumber: 1, EpisodeNumber: 3, Name: "E3", AirDate: "2999-01-01"})

	req := authedRequest("POST", "/api/shows/1/fill-aired", token, nil)
	req.SetPathValue("id", "1")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.APIFillAired(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	// Both aired episodes now watched, future one not.
	if h.DB.GetEpisodeWatchedAt(userID, showID, 1, 1) == "" || h.DB.GetEpisodeWatchedAt(userID, showID, 1, 2) == "" {
		t.Errorf("aired episodes should be marked watched")
	}
	if h.DB.GetEpisodeWatchedAt(userID, showID, 1, 3) != "" {
		t.Errorf("future episode should not be marked watched")
	}
	// Dated by air date.
	if got := h.DB.GetEpisodeWatchedAt(userID, showID, 1, 1); got[:10] != "2020-01-01" {
		t.Errorf("expected air date 2020-01-01, got %q", got)
	}
}
