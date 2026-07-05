package db

import (
	"os"
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/models"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	f, err := os.CreateTemp("", "watchlog-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := New(f.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := New("/nonexistent/path/db.sqlite")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// --- Users ---

func TestCreateUser(t *testing.T) {
	db := newTestDB(t)
	id, err := db.CreateUser("alice", "hash123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestCreateUser_Duplicate(t *testing.T) {
	db := newTestDB(t)
	db.CreateUser("alice", "hash123")
	_, err := db.CreateUser("alice", "other")
	if err == nil {
		t.Error("expected error for duplicate username")
	}
}

func TestGetUserByUsername(t *testing.T) {
	db := newTestDB(t)
	db.CreateUser("bob", "hash456")

	user, err := db.GetUserByUsername("bob")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if user.Username != "bob" {
		t.Errorf("Username = %q, want %q", user.Username, "bob")
	}
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := db.GetUserByUsername("nobody")
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}

func TestGetUserByID(t *testing.T) {
	db := newTestDB(t)
	id, _ := db.CreateUser("carol", "hash")
	user, err := db.GetUserByID(id)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user.Username != "carol" {
		t.Errorf("Username = %q, want %q", user.Username, "carol")
	}
}

func TestGetSetUserLang(t *testing.T) {
	db := newTestDB(t)
	id, _ := db.CreateUser("test", "hash")

	lang := db.GetUserLang(id)
	if lang != "es" {
		t.Errorf("default lang = %q, want %q", lang, "es")
	}

	db.SetUserLang(id, "en")
	lang = db.GetUserLang(id)
	if lang != "en" {
		t.Errorf("after set lang = %q, want %q", lang, "en")
	}
}

func TestGetUserLang_NonexistentUser(t *testing.T) {
	db := newTestDB(t)
	lang := db.GetUserLang(9999)
	if lang != "es" {
		t.Errorf("nonexistent user lang = %q, want %q", lang, "es")
	}
}

// --- Shows ---

func TestUpsertShow(t *testing.T) {
	db := newTestDB(t)
	id, err := db.UpsertShow(models.Show{ExternalID: 100, Name: "Breaking Bad"})
	if err != nil {
		t.Fatalf("UpsertShow: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	// Upsert again should not error
	id2, err := db.UpsertShow(models.Show{ExternalID: 100, Name: "Breaking Bad"})
	if err != nil {
		t.Fatalf("UpsertShow duplicate: %v", err)
	}
	if id2 != id {
		t.Errorf("upsert returned different ID: %d vs %d", id2, id)
	}
}

func TestGetShow(t *testing.T) {
	db := newTestDB(t)
	id, _ := db.UpsertShow(models.Show{ExternalID: 200, Name: "The Wire"})

	show, err := db.GetShow(id)
	if err != nil {
		t.Fatalf("GetShow: %v", err)
	}
	if show.Name != "The Wire" {
		t.Errorf("Name = %q, want %q", show.Name, "The Wire")
	}
}

func TestSearchShows(t *testing.T) {
	db := newTestDB(t)
	db.UpsertShow(models.Show{ExternalID: 1, Name: "Breaking Bad"})
	db.UpsertShow(models.Show{ExternalID: 2, Name: "Better Call Saul"})

	results, err := db.SearchShows("break")
	if err != nil {
		t.Fatalf("SearchShows: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

// --- User Shows ---

func TestUpsertUserShow(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "Show"})

	err := db.UpsertUserShow(uid, sid, true, false, false, 10, time.Now())
	if err != nil {
		t.Fatalf("UpsertUserShow: %v", err)
	}

	show, err := db.GetUserShow(uid, sid)
	if err != nil {
		t.Fatalf("GetUserShow: %v", err)
	}
	if !show.IsFollowed {
		t.Error("expected IsFollowed=true")
	}
	if show.EpisodesSeen != 10 {
		t.Errorf("EpisodesSeen = %d, want 10", show.EpisodesSeen)
	}
}

func TestGetUserShowsSorted(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	s1, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "Alpha"})
	s2, _ := db.UpsertShow(models.Show{ExternalID: 2, Name: "Beta"})
	db.UpsertUserShow(uid, s1, true, false, false, 5, time.Now())
	db.UpsertUserShow(uid, s2, true, false, false, 20, time.Now())

	shows, err := db.GetUserShowsSorted(uid, "name")
	if err != nil {
		t.Fatalf("GetUserShowsSorted: %v", err)
	}
	if len(shows) != 2 {
		t.Fatalf("expected 2 shows, got %d", len(shows))
	}
	if shows[0].Name != "Alpha" {
		t.Errorf("first show = %q, want Alpha", shows[0].Name)
	}

	shows, _ = db.GetUserShowsSorted(uid, "episodes")
	if shows[0].Name != "Beta" {
		t.Errorf("most watched first = %q, want Beta", shows[0].Name)
	}
}

func TestToggleFollow(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	db.ToggleUserShowFollow(uid, sid)
	show, _ := db.GetUserShow(uid, sid)
	if show.IsFollowed {
		t.Error("expected IsFollowed=false after toggle")
	}

	db.ToggleUserShowFollow(uid, sid)
	show, _ = db.GetUserShow(uid, sid)
	if !show.IsFollowed {
		t.Error("expected IsFollowed=true after second toggle")
	}
}

// --- Episodes ---

func TestMarkEpisodeWatched(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	err := db.MarkEpisodeWatched(uid, sid, 1, 1)
	if err != nil {
		t.Fatalf("MarkEpisodeWatched: %v", err)
	}

	eps, _ := db.GetEpisodesByShow(uid, sid)
	if len(eps) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(eps))
	}
	if eps[0].SeasonNumber != 1 || eps[0].EpisodeNumber != 1 {
		t.Errorf("episode = S%dE%d, want S1E1", eps[0].SeasonNumber, eps[0].EpisodeNumber)
	}
}

func TestMarkEpisodeWatched_Idempotent(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	db.MarkEpisodeWatched(uid, sid, 1, 1)
	db.MarkEpisodeWatched(uid, sid, 1, 1) // should not duplicate

	eps, _ := db.GetEpisodesByShow(uid, sid)
	if len(eps) != 1 {
		t.Errorf("expected 1 episode (idempotent), got %d", len(eps))
	}
}

func TestUnmarkEpisodeWatched(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	db.MarkEpisodeWatched(uid, sid, 1, 1)
	err := db.UnmarkEpisodeWatched(uid, sid, 1, 1)
	if err != nil {
		t.Fatalf("UnmarkEpisodeWatched: %v", err)
	}

	eps, _ := db.GetEpisodesByShow(uid, sid)
	if len(eps) != 0 {
		t.Errorf("expected 0 episodes after unmark, got %d", len(eps))
	}
}

func TestMarkSeasonWatched(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	marked, err := db.MarkSeasonWatched(uid, sid, 1, 10)
	if err != nil {
		t.Fatalf("MarkSeasonWatched: %v", err)
	}
	if marked != 10 {
		t.Errorf("marked = %d, want 10", marked)
	}

	eps, _ := db.GetEpisodesByShow(uid, sid)
	if len(eps) != 10 {
		t.Errorf("expected 10 episodes, got %d", len(eps))
	}
}

func TestUnmarkSeasonWatched(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	db.MarkSeasonWatched(uid, sid, 1, 5)
	removed, err := db.UnmarkSeasonWatched(uid, sid, 1)
	if err != nil {
		t.Fatalf("UnmarkSeasonWatched: %v", err)
	}
	if removed != 5 {
		t.Errorf("removed = %d, want 5", removed)
	}

	eps, _ := db.GetEpisodesByShow(uid, sid)
	if len(eps) != 0 {
		t.Errorf("expected 0 episodes after unmark season, got %d", len(eps))
	}
}

// --- Movies ---

func TestUpsertMovie(t *testing.T) {
	db := newTestDB(t)
	id, err := db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "Inception", TMDBID: 123})
	if err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestMarkMovieWatched(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	mid, _ := db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "Inception"})

	err := db.MarkMovieWatched(uid, mid, time.Now())
	if err != nil {
		t.Fatalf("MarkMovieWatched: %v", err)
	}

	movies, _ := db.GetUserMoviesSorted(uid, "name")
	if len(movies) != 1 {
		t.Fatalf("expected 1 movie, got %d", len(movies))
	}
	if movies[0].Name != "Inception" {
		t.Errorf("movie name = %q, want %q", movies[0].Name, "Inception")
	}
}

func TestSearchMovies(t *testing.T) {
	db := newTestDB(t)
	db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "Inception"})
	db.UpsertMovie(models.Movie{ExternalID: "m2", Name: "Interstellar"})

	results, err := db.SearchMovies("incep")
	if err != nil {
		t.Fatalf("SearchMovies: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

// --- Lists ---

func TestCreateAndGetList(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")

	id, err := db.CreateList(uid, "My List", false)
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	lists, _ := db.GetUserLists(uid)
	if len(lists) != 1 {
		t.Fatalf("expected 1 list, got %d", len(lists))
	}
	if lists[0].Name != "My List" {
		t.Errorf("list name = %q, want %q", lists[0].Name, "My List")
	}

	list, err := db.GetListWithItems(id)
	if err != nil {
		t.Fatalf("GetListWithItems: %v", err)
	}
	if list.Name != "My List" {
		t.Errorf("list name = %q, want %q", list.Name, "My List")
	}
}

func TestDeleteList(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	id, _ := db.CreateList(uid, "ToDelete", false)

	err := db.DeleteList(id)
	if err != nil {
		t.Fatalf("DeleteList: %v", err)
	}

	lists, _ := db.GetUserLists(uid)
	if len(lists) != 0 {
		t.Errorf("expected 0 lists after delete, got %d", len(lists))
	}
}

func TestAddShowToList(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "Show"})
	lid, _ := db.CreateList(uid, "Watchlist", false)

	err := db.AddShowToList(lid, sid)
	if err != nil {
		t.Fatalf("AddShowToList: %v", err)
	}

	list, _ := db.GetListWithItems(lid)
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list.Items))
	}
}

// --- Watch Stats ---

func TestUpsertWatchStats(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")

	err := db.UpsertWatchStats(uid, models.WatchStats{Period: "month-2024-01", Count: 10, Runtime: 500})
	if err != nil {
		t.Fatalf("UpsertWatchStats: %v", err)
	}

	stats, _ := db.GetUserWatchStats(uid)
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	if stats[0].Count != 10 {
		t.Errorf("count = %d, want 10", stats[0].Count)
	}
}

func TestIncrementWatchStats(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")

	db.IncrementWatchStats(uid, 3)
	db.IncrementWatchStats(uid, 2)

	stats, _ := db.GetUserWatchStats(uid)
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(stats))
	}
	if stats[0].Count != 5 {
		t.Errorf("count = %d, want 5", stats[0].Count)
	}
}

func TestRecalcWatchStats(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	// Insert episodes with specific watched_at
	db.MarkEpisodeWatched(uid, sid, 1, 1)
	db.MarkEpisodeWatched(uid, sid, 1, 2)

	err := db.RecalcWatchStats(uid)
	if err != nil {
		t.Fatalf("RecalcWatchStats: %v", err)
	}

	stats, _ := db.GetUserWatchStats(uid)
	if len(stats) == 0 {
		t.Fatal("expected at least 1 stat after recalc")
	}
	// Should have 2 episodes for current month
	total := 0
	for _, s := range stats {
		total += s.Count
	}
	if total < 2 {
		t.Errorf("total episodes in stats = %d, want >= 2", total)
	}
}

// --- Dashboard Stats ---

func TestGetDashboardStats(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())
	db.MarkEpisodeWatched(uid, sid, 1, 1)
	db.MarkEpisodeWatched(uid, sid, 1, 2)

	stats, err := db.GetDashboardStats(uid)
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.FollowedShows != 1 {
		t.Errorf("FollowedShows = %d, want 1", stats.FollowedShows)
	}
	if stats.TotalEpisodes != 2 {
		t.Errorf("TotalEpisodes = %d, want 2", stats.TotalEpisodes)
	}
}

// --- Show Progress ---

func TestUpsertShowProgress(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})

	err := db.UpsertShowProgress(uid, models.ShowProgress{
		ShowID: sid, ShowName: "S",
		LastSeasonNumber: 2, LastEpisodeNumber: 5, LastEpisodeID: 100,
		UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("UpsertShowProgress: %v", err)
	}

	p, err := db.GetShowProgress(uid, sid)
	if err != nil {
		t.Fatalf("GetShowProgress: %v", err)
	}
	if p.LastSeasonNumber != 2 || p.LastEpisodeNumber != 5 {
		t.Errorf("progress = S%dE%d, want S2E5", p.LastSeasonNumber, p.LastEpisodeNumber)
	}
}

// --- Additional coverage tests ---

func TestToggleFavorite(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	db.ToggleUserShowFavorite(uid, sid)
	show, _ := db.GetUserShow(uid, sid)
	if !show.IsFavorited {
		t.Error("expected IsFavorited=true after toggle")
	}
}

func TestToggleArchive(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	db.ToggleUserShowArchive(uid, sid)
	show, _ := db.GetUserShow(uid, sid)
	if !show.IsArchived {
		t.Error("expected IsArchived=true after toggle")
	}
}

func TestFollowShow(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})

	err := db.FollowShow(uid, sid)
	if err != nil {
		t.Fatalf("FollowShow: %v", err)
	}
	show, _ := db.GetUserShow(uid, sid)
	if !show.IsFollowed {
		t.Error("expected IsFollowed=true after FollowShow")
	}
}

func TestUpdateShowTMDB(t *testing.T) {
	db := newTestDB(t)
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})

	err := db.UpdateShowTMDB(sid, 12345, "/poster.jpg", "/backdrop.jpg", "Overview", "Drama", "Returning Series", 5)
	if err != nil {
		t.Fatalf("UpdateShowTMDB: %v", err)
	}
	show, _ := db.GetShow(sid)
	if show.TMDBID != 12345 {
		t.Errorf("TMDBID = %d, want 12345", show.TMDBID)
	}
}

func TestGetShowsWithoutTMDB(t *testing.T) {
	db := newTestDB(t)
	db.UpsertShow(models.Show{ExternalID: 1, Name: "No TMDB"})
	db.UpsertShow(models.Show{ExternalID: 2, Name: "Has TMDB", TMDBID: 99})

	shows, err := db.GetShowsWithoutTMDB()
	if err != nil {
		t.Fatalf("GetShowsWithoutTMDB: %v", err)
	}
	if len(shows) != 1 {
		t.Errorf("expected 1 show without TMDB, got %d", len(shows))
	}
}

func TestAddShowFromTMDB(t *testing.T) {
	db := newTestDB(t)
	id, err := db.AddShowFromTMDB(999, "New Show", "/p.jpg", "/b.jpg", "desc", "Drama", "Ended", 3)
	if err != nil {
		t.Fatalf("AddShowFromTMDB: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestInsertEpisode(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S"})

	err := db.InsertEpisode(models.Episode{
		UserID: uid, ExternalID: 100, ShowID: sid,
		SeasonNumber: 1, EpisodeNumber: 1,
		Watched: true, WatchedAt: time.Now(), Runtime: 45,
	})
	if err != nil {
		t.Fatalf("InsertEpisode: %v", err)
	}
}

func TestGetFollowedShowsWithTMDB(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S", TMDBID: 100})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	shows, err := db.GetFollowedShowsWithTMDB(uid)
	if err != nil {
		t.Fatalf("GetFollowedShowsWithTMDB: %v", err)
	}
	if len(shows) != 1 {
		t.Errorf("expected 1, got %d", len(shows))
	}
}

func TestUpdateMovieTMDB(t *testing.T) {
	db := newTestDB(t)
	mid, _ := db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "Movie"})

	err := db.UpdateMovieTMDB(mid, 555, "/p.jpg", "overview", "Action", 120)
	if err != nil {
		t.Fatalf("UpdateMovieTMDB: %v", err)
	}
}

func TestGetMoviesWithoutTMDB(t *testing.T) {
	db := newTestDB(t)
	db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "No TMDB"})
	db.UpsertMovie(models.Movie{ExternalID: "m2", Name: "Has TMDB", TMDBID: 99})

	movies, err := db.GetMoviesWithoutTMDB()
	if err != nil {
		t.Fatalf("GetMoviesWithoutTMDB: %v", err)
	}
	if len(movies) != 1 {
		t.Errorf("expected 1 movie without TMDB, got %d", len(movies))
	}
}

func TestAddMovieFromTMDB(t *testing.T) {
	db := newTestDB(t)
	id, err := db.AddMovieFromTMDB(777, "New Movie", "/p.jpg", "overview", "Sci-Fi", 150)
	if err != nil {
		t.Fatalf("AddMovieFromTMDB: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestUpdateList(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	lid, _ := db.CreateList(uid, "Original", false)

	err := db.UpdateList(lid, "Updated", true)
	if err != nil {
		t.Fatalf("UpdateList: %v", err)
	}
	list, _ := db.GetListWithItems(lid)
	if list.Name != "Updated" {
		t.Errorf("list name = %q, want Updated", list.Name)
	}
}

func TestAddAndRemoveListItem(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	lid, _ := db.CreateList(uid, "L", false)

	err := db.AddListItem(models.ListItem{ListID: lid, EntityType: "series", EntityID: 1})
	if err != nil {
		t.Fatalf("AddListItem: %v", err)
	}
	list, _ := db.GetListWithItems(lid)
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list.Items))
	}

	err = db.RemoveListItem(lid, list.Items[0].ID)
	if err != nil {
		t.Fatalf("RemoveListItem: %v", err)
	}
	list, _ = db.GetListWithItems(lid)
	if len(list.Items) != 0 {
		t.Errorf("expected 0 items after remove, got %d", len(list.Items))
	}
}

// TestRemoveListItem_ScopedByList ensures an item can only be removed via its
// own list, preventing cross-list deletion (IDOR).
func TestRemoveListItem_ScopedByList(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	victimList, _ := db.CreateList(uid, "victim", false)
	attackerList, _ := db.CreateList(uid, "attacker", false)

	if err := db.AddListItem(models.ListItem{ListID: victimList, EntityType: "series", EntityID: 1}); err != nil {
		t.Fatalf("AddListItem: %v", err)
	}
	vl, _ := db.GetListWithItems(victimList)
	if len(vl.Items) != 1 {
		t.Fatalf("expected 1 item in victim list, got %d", len(vl.Items))
	}
	itemID := vl.Items[0].ID

	// Attempt to remove the victim's item through a different list — must be a no-op.
	if err := db.RemoveListItem(attackerList, itemID); err != nil {
		t.Fatalf("RemoveListItem (cross-list): %v", err)
	}
	vl, _ = db.GetListWithItems(victimList)
	if len(vl.Items) != 1 {
		t.Errorf("cross-list deletion succeeded: item removed via wrong list_id")
	}

	// Removal through the correct list still works.
	if err := db.RemoveListItem(victimList, itemID); err != nil {
		t.Fatalf("RemoveListItem (correct list): %v", err)
	}
	vl, _ = db.GetListWithItems(victimList)
	if len(vl.Items) != 0 {
		t.Errorf("expected 0 items after correct remove, got %d", len(vl.Items))
	}
}

func TestAddMovieToList(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	lid, _ := db.CreateList(uid, "L", false)
	mid, _ := db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "M"})

	err := db.AddMovieToList(lid, mid)
	if err != nil {
		t.Fatalf("AddMovieToList: %v", err)
	}
	list, _ := db.GetListWithItems(lid)
	if len(list.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(list.Items))
	}
}

func TestGetUserShowsSorted_AllSorts(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	s1, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "Alpha"})
	s2, _ := db.UpsertShow(models.Show{ExternalID: 2, Name: "Beta"})
	db.UpsertUserShow(uid, s1, true, false, false, 5, time.Now().Add(-time.Hour))
	db.UpsertUserShow(uid, s2, true, false, false, 20, time.Now())

	for _, sort := range []string{"name", "recent", "episodes", "followed"} {
		shows, err := db.GetUserShowsSorted(uid, sort)
		if err != nil {
			t.Errorf("sort=%q: %v", sort, err)
		}
		if len(shows) != 2 {
			t.Errorf("sort=%q: expected 2 shows, got %d", sort, len(shows))
		}
	}
}

func TestGetUserMoviesSorted_AllSorts(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	m1, _ := db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "Alpha"})
	m2, _ := db.UpsertMovie(models.Movie{ExternalID: "m2", Name: "Beta"})
	db.MarkMovieWatched(uid, m1, time.Now().Add(-time.Hour))
	db.MarkMovieWatched(uid, m2, time.Now())

	for _, sort := range []string{"name", "recent"} {
		movies, err := db.GetUserMoviesSorted(uid, sort)
		if err != nil {
			t.Errorf("sort=%q: %v", sort, err)
		}
		if len(movies) != 2 {
			t.Errorf("sort=%q: expected 2 movies, got %d", sort, len(movies))
		}
	}
}

func TestUpsertUpcomingCache(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")
	sid, _ := db.UpsertShow(models.Show{ExternalID: 1, Name: "S", TMDBID: 1})
	db.UpsertUserShow(uid, sid, true, false, false, 0, time.Now())

	err := db.UpsertUpcomingCache(sid, "S", "/p.jpg", "Ep Name", 2, 1, "2026-07-10", "overview")
	if err != nil {
		t.Fatalf("UpsertUpcomingCache: %v", err)
	}

	cached, err := db.GetUpcomingCacheForUser(uid)
	if err != nil {
		t.Fatalf("GetUpcomingCacheForUser: %v", err)
	}
	if len(cached) != 1 {
		t.Errorf("expected 1 cached upcoming, got %d", len(cached))
	}

	err = db.DeleteUpcomingCache(sid)
	if err != nil {
		t.Fatalf("DeleteUpcomingCache: %v", err)
	}
}

func TestGetActiveShowsWithTMDB(t *testing.T) {
	db := newTestDB(t)
	db.UpsertShow(models.Show{ExternalID: 1, Name: "Active", TMDBID: 100, Status: "Returning Series"})
	db.UpsertShow(models.Show{ExternalID: 2, Name: "Ended", TMDBID: 200, Status: "Ended"})
	db.UpsertShow(models.Show{ExternalID: 3, Name: "No TMDB"})

	shows, err := db.GetActiveShowsWithTMDB()
	if err != nil {
		t.Fatalf("GetActiveShowsWithTMDB: %v", err)
	}
	// Should include shows with TMDB that are not "Ended"
	if len(shows) < 1 {
		t.Errorf("expected at least 1 active show, got %d", len(shows))
	}
}

func TestRecalcWatchStats_WithMovies(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")

	// Add a movie with watched_at
	mid, _ := db.UpsertMovie(models.Movie{ExternalID: "m1", Name: "M", Runtime: 120})
	db.MarkMovieWatched(uid, mid, time.Now())

	err := db.RecalcWatchStats(uid)
	if err != nil {
		t.Fatalf("RecalcWatchStats: %v", err)
	}

	stats, _ := db.GetUserWatchStats(uid)
	if len(stats) == 0 {
		t.Fatal("expected at least 1 stat after recalc with movies")
	}
}

// --- Sessions ---

func TestCreateAndGetSession(t *testing.T) {
	db := newTestDB(t)
	db.CreateUser("u", "h")

	err := db.CreateSession("tok123", 1, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	userID, ok := db.GetSession("tok123")
	if !ok {
		t.Fatal("GetSession returned not ok for valid token")
	}
	if userID != 1 {
		t.Errorf("userID = %d, want 1", userID)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	db := newTestDB(t)

	_, ok := db.GetSession("nonexistent")
	if ok {
		t.Error("GetSession should return false for nonexistent token")
	}
}

func TestGetSession_Expired(t *testing.T) {
	db := newTestDB(t)
	db.CreateUser("u", "h")

	db.CreateSession("expired", 1, time.Now().Add(-time.Hour))

	_, ok := db.GetSession("expired")
	if ok {
		t.Error("GetSession should return false for expired session")
	}
}

func TestDeleteSession(t *testing.T) {
	db := newTestDB(t)
	db.CreateUser("u", "h")

	db.CreateSession("todelete", 1, time.Now().Add(time.Hour))
	err := db.DeleteSession("todelete")
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, ok := db.GetSession("todelete")
	if ok {
		t.Error("GetSession should return false after delete")
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	db := newTestDB(t)
	db.CreateUser("u", "h")

	db.CreateSession("valid", 1, time.Now().Add(time.Hour))
	db.CreateSession("expired1", 1, time.Now().Add(-time.Hour))
	db.CreateSession("expired2", 1, time.Now().Add(-2*time.Hour))

	err := db.CleanExpiredSessions()
	if err != nil {
		t.Fatalf("CleanExpiredSessions: %v", err)
	}

	_, ok := db.GetSession("valid")
	if !ok {
		t.Error("valid session should still exist")
	}
	_, ok = db.GetSession("expired1")
	if ok {
		t.Error("expired1 should be cleaned")
	}
	_, ok = db.GetSession("expired2")
	if ok {
		t.Error("expired2 should be cleaned")
	}
}
