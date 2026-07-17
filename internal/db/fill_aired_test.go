package db

import (
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/models"
)

func TestFillAiredEpisodes_DeriveFromSeason(t *testing.T) {
	database := providersTestDB(t)
	uid, _ := database.CreateUser("u", "x")
	showID, _ := database.UpsertShow(models.Show{ExternalID: 1, Name: "S"})

	// Season 1: 5 episodes with air dates; season 2: 3 episodes.
	for e := 1; e <= 5; e++ {
		database.UpsertEpisodeDetail(EpisodeDetail{ShowID: showID, SeasonNumber: 1, EpisodeNumber: e, Name: "s1", AirDate: "2015-01-0" + string(rune('0'+e))})
	}
	for e := 1; e <= 3; e++ {
		database.UpsertEpisodeDetail(EpisodeDetail{ShowID: showID, SeasonNumber: 2, EpisodeNumber: e, Name: "s2", AirDate: "2016-02-0" + string(rune('0'+e))})
	}

	// Watched S1E2 (2021-05-02) and S1E4 (2021-05-20). Season 2: none watched.
	dX := time.Date(2021, 5, 2, 20, 0, 0, 0, time.Local)
	dY := time.Date(2021, 5, 20, 20, 0, 0, 0, time.Local)
	database.MarkEpisodeWatchedAt(uid, showID, 1, 2, dX)
	database.MarkEpisodeWatchedAt(uid, showID, 1, 4, dY)

	filled, err := database.FillAiredEpisodes(uid, showID)
	if err != nil {
		t.Fatal(err)
	}
	// Gaps filled: S1E1,E3,E5 (3) + S2E1..E3 (3) = 6.
	if filled != 6 {
		t.Fatalf("expected 6 filled, got %d", filled)
	}

	get := func(s, e int) string { return database.GetEpisodeWatchedAt(uid, showID, s, e) }

	// Season 1 gaps derive from nearest watched episode's date.
	if got := get(1, 1); got[:10] != "2021-05-02" {
		t.Errorf("S1E1 should derive from E2 (2021-05-02), got %q", got)
	}
	if got := get(1, 5); got[:10] != "2021-05-20" {
		t.Errorf("S1E5 should derive from E4 (2021-05-20), got %q", got)
	}
	// Season 2 has no watched episode → falls back to air date.
	if got := get(2, 1); got[:10] != "2016-02-01" {
		t.Errorf("S2E1 should use air date 2016-02-01, got %q", got)
	}
	// Existing watched dates untouched.
	if got := get(1, 2); got[:10] != "2021-05-02" {
		t.Errorf("S1E2 real date changed: %q", got)
	}
}
