package importer

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/models"
)

func setupDB(t *testing.T) (*db.DB, int64) {
	t.Helper()
	f, err := os.CreateTemp("", "wl-netflix-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	database, err := db.New(f.Name())
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	hash, _ := auth.HashPassword("x")
	uid, _ := database.CreateUser("u", hash)
	return database, uid
}

func TestAnalyzeNetflix_EpisodeByTitleAndMovie(t *testing.T) {
	database, uid := setupDB(t)

	// Show with Spanish name and one episode detail.
	showID, err := database.UpsertShow(models.Show{ExternalID: 1, Name: "Money Heist"})
	if err != nil {
		t.Fatal(err)
	}
	database.UpdateShowTMDBNames(showID, "La casa de papel", "Money Heist")
	if err := database.UpsertEpisodeDetail(db.EpisodeDetail{ShowID: showID, SeasonNumber: 4, EpisodeNumber: 1, Name: "Game Over"}); err != nil {
		t.Fatal(err)
	}
	// Mark the episode watched (date = now), then Netflix says an older date.
	if err := database.MarkEpisodeWatched(uid, showID, 4, 1); err != nil {
		t.Fatal(err)
	}

	// A watched movie.
	movieID, err := database.UpsertMovie(models.Movie{ExternalID: "m1", Name: "Extraction"})
	if err != nil {
		t.Fatal(err)
	}
	database.UpdateMovieTMDBNames(movieID, "Tyler Rake", "Extraction")
	if err := database.MarkMovieWatched(uid, movieID, time.Date(2023, 7, 2, 12, 0, 0, 0, time.Local)); err != nil {
		t.Fatal(err)
	}

	csv := `Title,Date
"La casa de papel: Parte 4: Game Over","4/3/20"
"Tyler Rake","7/9/23"
"Serie Desconocida: Temporada 1: Piloto","1/1/20"
`
	res, err := AnalyzeNetflix(database, uid, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("AnalyzeNetflix: %v", err)
	}

	var epChange, movieChange *NetflixChange
	for i := range res.Changes {
		c := &res.Changes[i]
		if c.Type == "episode" {
			epChange = c
		}
		if c.Type == "movie" {
			movieChange = c
		}
	}
	if epChange == nil {
		t.Fatalf("expected an episode change, got %+v", res.Changes)
	}
	if epChange.ID != showID || epChange.Season != 4 || epChange.Episode != 1 || epChange.NewDate != "2020-04-03" {
		t.Errorf("unexpected episode change: %+v", epChange)
	}
	if epChange.Title != "La casa de papel" {
		t.Errorf("expected DB display name 'La casa de papel', got %q", epChange.Title)
	}
	if movieChange == nil || movieChange.ID != movieID || movieChange.NewDate != "2023-07-09" {
		t.Errorf("unexpected movie change: %+v", movieChange)
	}
	// Unknown series should be reported unmatched.
	found := false
	for _, s := range res.UnmatchedSeries {
		if s == "Serie Desconocida" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Serie Desconocida' in unmatched, got %v", res.UnmatchedSeries)
	}
}

func TestAnalyzeNetflix_OrderFallbackAndDedup(t *testing.T) {
	database, uid := setupDB(t)
	showID, _ := database.UpsertShow(models.Show{ExternalID: 2, Name: "Lupin"})
	// No episode_details → must fall back to chronological order.
	database.MarkEpisodeWatched(uid, showID, 1, 1)
	database.MarkEpisodeWatched(uid, showID, 1, 2)

	// Netflix CSV is newest-first. Reversing gives chronological order:
	// oldest entry → episode 1, next → episode 2.
	csv := `Title,Date
"Lupin: Parte 1: Capítulo 2","1/20/21"
"Lupin: Parte 1: Capítulo 1","1/15/21"
`
	res, err := AnalyzeNetflix(database, uid, strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	dates := map[int]string{}
	for _, c := range res.Changes {
		dates[c.Episode] = c.NewDate
	}
	if dates[1] != "2021-01-15" {
		t.Errorf("episode 1 expected 2021-01-15, got %q", dates[1])
	}
	if dates[2] != "2021-01-20" {
		t.Errorf("episode 2 expected 2021-01-20, got %q", dates[2])
	}
}

func TestAnalyzeNetflix_MovieRewatchKeepsOldest(t *testing.T) {
	database, uid := setupDB(t)
	movieID, _ := database.UpsertMovie(models.Movie{ExternalID: "m2", Name: "Extraction"})
	database.UpdateMovieTMDBNames(movieID, "Tyler Rake", "Extraction")
	database.MarkMovieWatched(uid, movieID, time.Date(2020, 1, 1, 12, 0, 0, 0, time.Local))

	// Same movie watched twice on Netflix (newest-first). Expect a single
	// change with the oldest date (first watch).
	csv := `Title,Date
"Tyler Rake","5/20/23"
"Tyler Rake","3/10/22"
`
	res, err := AnalyzeNetflix(database, uid, strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	movieChanges := 0
	var got string
	for _, c := range res.Changes {
		if c.Type == "movie" {
			movieChanges++
			got = c.NewDate
		}
	}
	if movieChanges != 1 {
		t.Fatalf("expected exactly 1 movie change (deduped), got %d: %+v", movieChanges, res.Changes)
	}
	if got != "2022-03-10" {
		t.Errorf("expected oldest date 2022-03-10, got %q", got)
	}
}
