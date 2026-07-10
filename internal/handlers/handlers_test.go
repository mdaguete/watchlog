package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/i18n"
	"github.com/mdaguete/watchlog/internal/models"
)

func newTestHandler(t *testing.T) (*Handler, int64, string) {
	t.Helper()
	f, err := os.CreateTemp("", "watchlog-handler-test-*.db")
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

	funcMap := template.FuncMap{
		"T":   i18n.T,
		"dict": func(values ...any) map[string]any {
			m := make(map[string]any, len(values)/2)
			for i := 0; i+1 < len(values); i += 2 {
				key, _ := values[i].(string)
				m[key] = values[i+1]
			}
			return m
		},
		"Loc": func(lang, es, en string) string { if lang == "en" && en != "" { return en }; if es != "" { return es }; return en },
		"LocGenres": func(lang, es, en string) string { if lang == "en" && en != "" { return en }; return i18n.TranslateGenres(lang, es) },
		"LocName": func(lang, name, nameES, nameEN string) string { if lang == "en" { if nameEN != "" { return nameEN }; return name }; if nameES != "" { return nameES }; return name },
		"min": func(a, b int) int { if a < b { return a }; return b },
		"mul": func(a, b int) int { return a * b },
		"add": func(a, b int) int { return a + b },
		"mod": func(a, b int) int { return a % b },
		"dtLocal": func(t time.Time) string { if t.IsZero() { return "" }; return t.Format("2006-01-02T15:04") },
		"dt": func(t time.Time) string { if t.IsZero() { return "" }; return t.Format("2006-01-02 15:04") },
		"ImgURL": func(url string) string { return url },
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("../../web/templates/*.html")
	if err != nil {
		t.Fatalf("ParseGlob: %v", err)
	}

	sessions := auth.NewSessionStore(database)
	h := New(database, tmpl, nil, sessions, nil, os.TempDir())

	// Create test user
	hash, _ := auth.HashPassword("test123")
	userID, _ := database.CreateUser("testuser", hash)

	// Create session
	token := sessions.Create(userID)

	return h, userID, token
}

func authedRequest(method, path, token string, body []byte) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	return req
}

// --- Auth Pages ---

func TestPageLogin_Unauthenticated(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	h.PageLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageLogin_AlreadyAuthenticated(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/login", token, nil)
	w := httptest.NewRecorder()

	h.PageLogin(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
}

func TestHandleLogin_Success(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/login", nil)
	req.Form = map[string][]string{
		"username": {"testuser"},
		"password": {"test123"},
	}
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/login", nil)
	req.Form = map[string][]string{
		"username": {"testuser"},
		"password": {"wrong"},
	}
	w := httptest.NewRecorder()

	h.HandleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (re-render login)", w.Code)
	}
}

func TestHandleRegister_Success(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/register", nil)
	req.Form = map[string][]string{
		"username": {"newuser"},
		"password": {"pass1234"},
	}
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}
}

func TestHandleRegister_EmptyFields(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/register", nil)
	req.Form = map[string][]string{
		"username": {""},
		"password": {""},
	}
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleRegister_ShortPassword(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/register", nil)
	req.Form = map[string][]string{
		"username": {"x"},
		"password": {"ab"},
	}
	w := httptest.NewRecorder()

	h.HandleRegister(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleLogout(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("POST", "/logout", token, nil)
	w := httptest.NewRecorder()

	h.HandleLogout(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
}

// --- Protected Pages ---

func TestPageDashboard_Unauthenticated(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	h.PageDashboard(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect to login", w.Code)
	}
}

func TestPageDashboard_Authenticated(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/", token, nil)
	w := httptest.NewRecorder()

	h.PageDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageShows(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/shows", token, nil)
	w := httptest.NewRecorder()

	h.PageShows(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageMovies(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/movies", token, nil)
	w := httptest.NewRecorder()

	h.PageMovies(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageStats(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/stats", token, nil)
	w := httptest.NewRecorder()

	h.PageStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageSettings(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/settings", token, nil)
	w := httptest.NewRecorder()

	h.PageSettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestSaveSettings(t *testing.T) {
	h, userID, token := newTestHandler(t)
	req := authedRequest("POST", "/settings", token, nil)
	req.Form = map[string][]string{"lang": {"en"}}
	w := httptest.NewRecorder()

	h.SaveSettings(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}

	lang := h.DB.GetUserLang(userID)
	if lang != "en" {
		t.Errorf("lang = %q, want %q", lang, "en")
	}
}

func TestSaveSettings_InvalidLang(t *testing.T) {
	h, userID, token := newTestHandler(t)
	req := authedRequest("POST", "/settings", token, nil)
	req.Form = map[string][]string{"lang": {"xx"}}
	w := httptest.NewRecorder()

	h.SaveSettings(w, req)

	lang := h.DB.GetUserLang(userID)
	if lang != "es" {
		t.Errorf("invalid lang should default to es, got %q", lang)
	}
}

// --- API Handlers ---

func TestAPIGetStats(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/api/stats", token, nil)
	w := httptest.NewRecorder()

	h.APIGetStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestAPIMarkEpisodeWatched(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "Test Show"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	body, _ := json.Marshal(map[string]int{"season": 1, "episode": 1})
	req := authedRequest("POST", "/api/shows/1/episodes/watched", token, body)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.APIMarkEpisodeWatched(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIUnmarkEpisodeWatched(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "Test Show"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())
	h.DB.MarkEpisodeWatched(userID, sid, 1, 1)

	body, _ := json.Marshal(map[string]int{"season": 1, "episode": 1})
	req := authedRequest("DELETE", "/api/shows/1/episodes/watched", token, body)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.APIUnmarkEpisodeWatched(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIGetShows_Unauthenticated(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/api/shows", nil)
	w := httptest.NewRecorder()

	h.APIGetShows(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// --- More Page Tests ---

func TestPageSearch(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/search", token, nil)
	w := httptest.NewRecorder()
	h.PageSearch(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageAddShow(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/add", token, nil)
	w := httptest.NewRecorder()
	h.PageAddShow(w, req)
	// /add now redirects to the unified /search page.
	if w.Code != http.StatusMovedPermanently && w.Code != http.StatusFound {
		t.Errorf("status = %d, want redirect", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/search" {
		t.Errorf("redirect Location = %q, want /search", loc)
	}
}

func TestPageUpcoming(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/upcoming", token, nil)
	w := httptest.NewRecorder()
	h.PageUpcoming(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageRegister(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/register", nil)
	w := httptest.NewRecorder()
	h.PageRegister(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- More API Tests ---

func TestAPIGetShows(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/api/shows", token, nil)
	w := httptest.NewRecorder()
	h.APIGetShows(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIGetMovies(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/api/movies", token, nil)
	w := httptest.NewRecorder()
	h.APIGetMovies(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIGetWatchStats(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/api/stats/history", token, nil)
	w := httptest.NewRecorder()
	h.APIGetWatchStats(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPISearch(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/api/search?q=test", token, nil)
	w := httptest.NewRecorder()
	h.APISearch(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIToggleFollow(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	req := authedRequest("POST", "/api/shows/1/follow", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIToggleFollow(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIToggleFavorite(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	req := authedRequest("POST", "/api/shows/1/favorite", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIToggleFavorite(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIToggleArchive(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	req := authedRequest("POST", "/api/shows/1/archive", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIToggleArchive(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIGetEpisodes(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())
	h.DB.MarkEpisodeWatched(userID, sid, 1, 1)

	req := authedRequest("GET", "/api/shows/1/episodes", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIGetEpisodes(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIMarkEpisodeWatched_InvalidBody(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("POST", "/api/shows/1/episodes/watched", token, []byte("invalid"))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIMarkEpisodeWatched(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAPIUnmarkEpisodeWatched_InvalidBody(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("DELETE", "/api/shows/1/episodes/watched", token, []byte("invalid"))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIUnmarkEpisodeWatched(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAPIMarkSeasonWatched_InvalidBody(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("POST", "/api/shows/1/season/watched", token, []byte("bad"))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIMarkSeasonWatched(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAPIUnmarkSeasonWatched(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())
	h.DB.MarkSeasonWatched(userID, sid, 1, 5)

	body, _ := json.Marshal(map[string]int{"season": 1})
	req := authedRequest("DELETE", "/api/shows/1/season/watched", token, body)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIUnmarkSeasonWatched(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIGetShow(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "Show"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	req := authedRequest("GET", "/api/shows/1", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIGetShow(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestSearchResults(t *testing.T) {
	h, _, token := newTestHandler(t)
	h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "Breaking Bad"})

	req := authedRequest("GET", "/search/results?q=break", token, nil)
	w := httptest.NewRecorder()
	h.SearchResults(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestSearchResults_Empty(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/search/results?q=", token, nil)
	w := httptest.NewRecorder()
	h.SearchResults(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIMarkEpisodeWatched_HXRequest(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	body, _ := json.Marshal(map[string]int{"season": 1, "episode": 1})
	req := authedRequest("POST", "/api/shows/1/episodes/watched", token, body)
	req.SetPathValue("id", "1")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.APIMarkEpisodeWatched(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Content-Type = %q, want text/html for HX-Request", ct)
	}
}

func TestPageShow(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "Show"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())
	h.DB.MarkEpisodeWatched(userID, sid, 1, 1)

	req := authedRequest("GET", "/shows/1", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.PageShow(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageShow_InvalidID(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/shows/abc", token, nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	h.PageShow(w, req)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}
}

func TestHandleRegister_DuplicateUser(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/register", nil)
	req.Form = map[string][]string{
		"username": {"testuser"},
		"password": {"pass1234"},
	}
	w := httptest.NewRecorder()
	h.HandleRegister(w, req)
	// testuser already exists from newTestHandler
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (re-render with error)", w.Code)
	}
}

func TestAPIFetchTMDB_NoClient(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	req := authedRequest("POST", "/api/shows/1/fetch-tmdb", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIFetchTMDB(w, req)
	// With nil TMDB client, should error or handle gracefully
	if w.Code == 0 {
		t.Error("expected some response")
	}
}

func TestAPIFetchAllTMDB_NoClient(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("POST", "/api/tmdb/fetch-all", token, nil)
	w := httptest.NewRecorder()
	h.APIFetchAllTMDB(w, req)
	if w.Code == 0 {
		t.Error("expected some response")
	}
}

func TestAPIAddShowFromTMDB_NoClient(t *testing.T) {
	h, _, token := newTestHandler(t)
	body, _ := json.Marshal(map[string]int{"tmdb_id": 123})
	req := authedRequest("POST", "/api/tmdb/add-show", token, body)
	w := httptest.NewRecorder()
	h.APIAddShowFromTMDB(w, req)
	if w.Code == 0 {
		t.Error("expected some response")
	}
}

func TestAPIAddMovieFromTMDB_NoClient(t *testing.T) {
	h, _, token := newTestHandler(t)
	body, _ := json.Marshal(map[string]int{"tmdb_id": 456})
	req := authedRequest("POST", "/api/tmdb/add-movie", token, body)
	w := httptest.NewRecorder()
	h.APIAddMovieFromTMDB(w, req)
	if w.Code == 0 {
		t.Error("expected some response")
	}
}

func TestAPIMarkSeasonWatched_WithEpisodes(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	body, _ := json.Marshal(map[string]int{"season": 1, "episodes": 10})
	req := authedRequest("POST", "/api/shows/1/season/watched", token, body)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIMarkSeasonWatched(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIMarkSeasonWatched_HXRequest(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	body, _ := json.Marshal(map[string]int{"season": 1, "episodes": 5})
	req := authedRequest("POST", "/api/shows/1/season/watched", token, body)
	req.SetPathValue("id", "1")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.APIMarkSeasonWatched(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestAPIMarkSeasonWatched_NoEpisodesNoTMDB(t *testing.T) {
	h, userID, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.UpsertUserShow(userID, sid, true, false, false, 0, time.Now())

	// episodes=0 and no TMDB → should return error
	body, _ := json.Marshal(map[string]int{"season": 1, "episodes": 0})
	req := authedRequest("POST", "/api/shows/1/season/watched", token, body)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIMarkSeasonWatched(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAPIUnmarkSeasonWatched_InvalidBody(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("DELETE", "/api/shows/1/season/watched", token, []byte("bad"))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIUnmarkSeasonWatched(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPageShow_CatalogOnly(t *testing.T) {
	h, _, token := newTestHandler(t)
	// Show exists in catalog but user hasn't followed
	h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "Catalog Show"})

	req := authedRequest("GET", "/shows/1", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.PageShow(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageShow_NotFound(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/shows/999", token, nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	h.PageShow(w, req)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
}

func TestPageForgotPassword(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/forgot-password", nil)
	w := httptest.NewRecorder()
	h.PageForgotPassword(w, req)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestHandleForgotPassword_NoUser(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/forgot-password", bytes.NewBufferString("identifier=nonexistent"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleForgotPassword(w, req)
	// Should still return 200 (don't reveal if user exists)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestHandleForgotPassword_NoSMTP(t *testing.T) {
	h, _, _ := newTestHandler(t)
	// User exists but SMTP not configured
	h.DB.UpdateUserEmail(1, "test@example.com")
	req := httptest.NewRequest("POST", "/forgot-password", bytes.NewBufferString("identifier=testuser"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleForgotPassword(w, req)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestPageMagicLogin(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/magic-login", nil)
	w := httptest.NewRecorder()
	h.PageMagicLogin(w, req)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestHandleMagicLogin_NoEmail(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/magic-login", bytes.NewBufferString("email=nobody@example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleMagicLogin(w, req)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestHandleMagicAuth_InvalidToken(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/auth/magic?token=invalidtoken", nil)
	w := httptest.NewRecorder()
	h.HandleMagicAuth(w, req)
	// Shows error page on invalid token
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestHandleMagicAuth_ValidToken(t *testing.T) {
	h, userID, _ := newTestHandler(t)
	token := auth.GenerateToken()
	h.DB.CreateMagicLink(token, userID, "login", time.Now().Add(time.Hour))
	req := httptest.NewRequest("GET", "/auth/magic?token="+token, nil)
	w := httptest.NewRecorder()
	h.HandleMagicAuth(w, req)
	if w.Code != 302 { t.Errorf("expected 302 redirect, got %d", w.Code) }
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}
}

func TestPageResetPassword_InvalidToken(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/reset-password?token=badtoken", nil)
	w := httptest.NewRecorder()
	h.PageResetPassword(w, req)
	// Shows error page on invalid token
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestPageResetPassword_ValidToken(t *testing.T) {
	h, userID, _ := newTestHandler(t)
	token := auth.GenerateToken()
	h.DB.CreateMagicLink(token, userID, "reset", time.Now().Add(time.Hour))
	req := httptest.NewRequest("GET", "/reset-password?token="+token, nil)
	w := httptest.NewRecorder()
	h.PageResetPassword(w, req)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestHandleResetPassword_Success(t *testing.T) {
	h, userID, _ := newTestHandler(t)
	token := auth.GenerateToken()
	h.DB.CreateMagicLink(token, userID, "reset", time.Now().Add(time.Hour))
	body := "token=" + token + "&password=newpassword123"
	req := httptest.NewRequest("POST", "/reset-password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleResetPassword(w, req)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
	// Verify new password works
	user, _ := h.DB.GetUserByID(userID)
	if !auth.CheckPassword(user.PasswordHash, "newpassword123") {
		t.Error("password was not updated")
	}
	// Verify token is consumed (single use)
	_, _, ok := h.DB.GetMagicLink(token)
	if ok {
		t.Error("magic link should have been deleted after use")
	}
}

func TestHandleResetPassword_ShortPassword(t *testing.T) {
	h, userID, _ := newTestHandler(t)
	token := auth.GenerateToken()
	h.DB.CreateMagicLink(token, userID, "reset", time.Now().Add(time.Hour))
	body := "token=" + token + "&password=short"
	req := httptest.NewRequest("POST", "/reset-password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleResetPassword(w, req)
	if w.Code != 200 { t.Errorf("expected 200 (show form again), got %d", w.Code) }
}

// --- Auth Improvements Tests ---

func TestHandleLogin_RateLimited(t *testing.T) {
	h, _, _ := newTestHandler(t)

	// Trigger 5 failed login attempts
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.Form = map[string][]string{
			"username": {"testuser"},
			"password": {"wrongpassword"},
		}
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		h.HandleLogin(w, req)
	}

	// 6th attempt should be rate limited
	req := httptest.NewRequest("POST", "/login", nil)
	req.Form = map[string][]string{
		"username": {"testuser"},
		"password": {"test123"},
	}
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	h.HandleLogin(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", w.Code)
	}
}

func TestHandleLogin_PasswordDisabled(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.SetSetting("auth_password", "disabled")

	req := httptest.NewRequest("POST", "/login", nil)
	req.Form = map[string][]string{
		"username": {"testuser"},
		"password": {"test123"},
	}
	w := httptest.NewRecorder()
	h.HandleLogin(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestPageRegister_Disabled(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.SetSetting("auth_registration", "disabled")

	req := httptest.NewRequest("GET", "/register", nil)
	w := httptest.NewRecorder()
	h.PageRegister(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestHandleRegister_Disabled(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.SetSetting("auth_registration", "disabled")

	req := httptest.NewRequest("POST", "/register", nil)
	req.Form = map[string][]string{
		"username": {"newuser"},
		"password": {"pass1234"},
	}
	w := httptest.NewRecorder()
	h.HandleRegister(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestPageAdmin_NonAdmin(t *testing.T) {
	h, _, _ := newTestHandler(t)

	// Create a second user (id=2, not admin)
	hash, _ := auth.HashPassword("pass1234")
	userID2, _ := h.DB.CreateUser("user2", hash)
	token2 := h.Sessions.Create(userID2)

	req := authedRequest("GET", "/admin", token2, nil)
	w := httptest.NewRecorder()
	h.PageAdmin(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}
}

func TestPageAdmin_Admin(t *testing.T) {
	h, _, token := newTestHandler(t)

	req := authedRequest("GET", "/admin", token, nil)
	w := httptest.NewRecorder()
	h.PageAdmin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestSaveAdmin_Settings(t *testing.T) {
	h, _, token := newTestHandler(t)

	req := authedRequest("POST", "/admin", token, nil)
	req.Form = map[string][]string{
		"watchlog_url":       {"https://watch.example.com"},
		"auth_registration":  {"on"},
		"auth_password":      {"on"},
		"auth_magic_link":    {"on"},
		"auth_default_login": {"password"},
	}
	w := httptest.NewRecorder()
	h.SaveAdmin(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}

	// Verify settings persisted
	if v := h.DB.GetSetting("watchlog_url"); v != "https://watch.example.com" {
		t.Errorf("watchlog_url = %q, want %q", v, "https://watch.example.com")
	}
	if v := h.DB.GetSetting("auth_default_login"); v != "password" {
		t.Errorf("auth_default_login = %q, want %q", v, "password")
	}
	if v := h.DB.GetSetting("auth_registration"); v != "enabled" {
		t.Errorf("auth_registration = %q, want %q", v, "enabled")
	}
}

func TestPageForgotPassword_SMTPConfigured(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.SetSetting("smtp_url", "smtp://user:pass@localhost:2525/test@example.com")

	req := httptest.NewRequest("GET", "/forgot-password", nil)
	w := httptest.NewRecorder()
	h.PageForgotPassword(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	// Body should contain the form (SMTP is configured)
	body := w.Body.String()
	if !strings.Contains(body, "username_or_email") && !strings.Contains(body, "form") {
		t.Error("expected form to be shown when SMTP is configured")
	}
}

func TestHandleForgotPassword_UserWithEmail(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.SetSetting("smtp_url", "smtp://user:pass@localhost:2525/test@example.com")
	h.DB.SetSetting("watchlog_url", "http://localhost:8080")
	h.DB.UpdateUserEmail(1, "test@example.com")

	req := httptest.NewRequest("POST", "/forgot-password", bytes.NewBufferString("username_or_email=testuser"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleForgotPassword(w, req)

	// Should return 200 (success message shown regardless of email send result)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleMagicLogin_UserWithEmail(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.SetSetting("smtp_url", "smtp://user:pass@localhost:2525/test@example.com")
	h.DB.SetSetting("watchlog_url", "http://localhost:8080")
	h.DB.UpdateUserEmail(1, "test@example.com")

	req := httptest.NewRequest("POST", "/magic-login", bytes.NewBufferString("username=testuser"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleMagicLogin(w, req)

	// Should return 200 with success message
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleMagicLogin_UserNoEmail(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.SetSetting("smtp_url", "smtp://user:pass@localhost:2525/test@example.com")

	// User exists but has no email set
	req := httptest.NewRequest("POST", "/magic-login", bytes.NewBufferString("username=testuser"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleMagicLogin(w, req)

	// Should still return 200 (success shown to prevent user enumeration)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageLogin_DefaultMagic(t *testing.T) {
	h, _, _ := newTestHandler(t)
	// Default is already "magic" when no setting is set

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	h.PageLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageLogin_DefaultPassword(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.SetSetting("auth_default_login", "password")

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	h.PageLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestSaveAdmin_AuthOptions(t *testing.T) {
	h, _, token := newTestHandler(t)
	body := "auth_registration=on&auth_magic_link=on&auth_default_login=password"
	req := httptest.NewRequest("POST", "/admin", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.SaveAdmin(w, req)
	if w.Code != 302 { t.Errorf("expected 302, got %d", w.Code) }
	if h.DB.GetSetting("auth_registration") != "enabled" { t.Error("registration not enabled") }
	if h.DB.GetSetting("auth_password") != "disabled" { t.Error("password should be disabled") }
	if h.DB.GetSetting("auth_magic_link") != "enabled" { t.Error("magic link not enabled") }
	if h.DB.GetSetting("auth_default_login") != "password" { t.Error("default login not saved") }
}

func TestSaveSettings_Email(t *testing.T) {
	h, _, token := newTestHandler(t)
	body := "lang=en&email=user@example.com"
	req := httptest.NewRequest("POST", "/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.SaveSettings(w, req)
	if w.Code != 302 { t.Errorf("expected 302, got %d", w.Code) }
	user, _ := h.DB.GetUserByID(1)
	if user.Email != "user@example.com" { t.Errorf("email not saved, got %q", user.Email) }
}

func TestSetupWizard_Step1_EmptyUsername(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-setup-*.db")
	f.Close()
	defer os.Remove(f.Name())
	database, _ := db.New(f.Name())
	defer database.Close()
	funcMap := template.FuncMap{
		"T": i18n.T, "dict": func(values ...any) map[string]any { m := make(map[string]any); for i := 0; i+1 < len(values); i += 2 { k, _ := values[i].(string); m[k] = values[i+1] }; return m },
		"Loc": func(l, e, n string) string { return e },
		"LocGenres": func(l, e, n string) string { return e },
		"LocName": func(l, n, e, en string) string { return n },
		"min": func(a, b int) int { if a < b { return a }; return b },
		"mul": func(a, b int) int { return a * b }, "add": func(a, b int) int { return a + b }, "mod": func(a, b int) int { return a % b },
		"dtLocal": func(t time.Time) string { if t.IsZero() { return "" }; return t.Format("2006-01-02T15:04") },
		"dt": func(t time.Time) string { if t.IsZero() { return "" }; return t.Format("2006-01-02 15:04") },
		"ImgURL": func(url string) string { return url },
	}
	tmpl, _ := template.New("").Funcs(funcMap).ParseGlob("../../web/templates/*.html")
	sessions := auth.NewSessionStore(database)
	h := New(database, tmpl, nil, sessions, nil, os.TempDir())

	body := "step=1&username=&email=test@x.com&password=12345678&password_confirm=12345678"
	req := httptest.NewRequest("POST", "/setup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetup(w, req)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestSetupWizard_Step1_PasswordMismatch(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-setup-*.db")
	f.Close()
	defer os.Remove(f.Name())
	database, _ := db.New(f.Name())
	defer database.Close()
	funcMap := template.FuncMap{
		"T": i18n.T, "dict": func(values ...any) map[string]any { m := make(map[string]any); for i := 0; i+1 < len(values); i += 2 { k, _ := values[i].(string); m[k] = values[i+1] }; return m },
		"Loc": func(l, e, n string) string { return e },
		"LocGenres": func(l, e, n string) string { return e },
		"LocName": func(l, n, e, en string) string { return n },
		"min": func(a, b int) int { if a < b { return a }; return b },
		"mul": func(a, b int) int { return a * b }, "add": func(a, b int) int { return a + b }, "mod": func(a, b int) int { return a % b },
		"dtLocal": func(t time.Time) string { if t.IsZero() { return "" }; return t.Format("2006-01-02T15:04") },
		"dt": func(t time.Time) string { if t.IsZero() { return "" }; return t.Format("2006-01-02 15:04") },
		"ImgURL": func(url string) string { return url },
	}
	tmpl, _ := template.New("").Funcs(funcMap).ParseGlob("../../web/templates/*.html")
	sessions := auth.NewSessionStore(database)
	h := New(database, tmpl, nil, sessions, nil, os.TempDir())

	body := "step=1&username=admin&email=a@b.com&password=12345678&password_confirm=different"
	req := httptest.NewRequest("POST", "/setup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetup(w, req)
	if w.Code != 200 { t.Errorf("expected 200, got %d", w.Code) }
}

func TestSetupWizard_Step1_Success(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-setup-*.db")
	f.Close()
	defer os.Remove(f.Name())
	database, _ := db.New(f.Name())
	defer database.Close()
	funcMap := template.FuncMap{
		"T": i18n.T, "dict": func(values ...any) map[string]any { m := make(map[string]any); for i := 0; i+1 < len(values); i += 2 { k, _ := values[i].(string); m[k] = values[i+1] }; return m },
		"Loc": func(l, e, n string) string { return e },
		"LocGenres": func(l, e, n string) string { return e },
		"LocName": func(l, n, e, en string) string { return n },
		"min": func(a, b int) int { if a < b { return a }; return b },
		"mul": func(a, b int) int { return a * b }, "add": func(a, b int) int { return a + b }, "mod": func(a, b int) int { return a % b },
		"dtLocal": func(t time.Time) string { if t.IsZero() { return "" }; return t.Format("2006-01-02T15:04") },
		"dt": func(t time.Time) string { if t.IsZero() { return "" }; return t.Format("2006-01-02 15:04") },
		"ImgURL": func(url string) string { return url },
	}
	tmpl, _ := template.New("").Funcs(funcMap).ParseGlob("../../web/templates/*.html")
	sessions := auth.NewSessionStore(database)
	h := New(database, tmpl, nil, sessions, nil, os.TempDir())

	body := "step=1&username=admin&email=admin@example.com&password=securepass&password_confirm=securepass"
	req := httptest.NewRequest("POST", "/setup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetup(w, req)
	if w.Code != 302 { t.Errorf("expected 302, got %d", w.Code) }
	if loc := w.Header().Get("Location"); loc != "/setup?step=2" {
		t.Errorf("expected redirect to /setup?step=2, got %q", loc)
	}
	// Verify user was created
	if !database.HasUsers() { t.Error("user was not created") }
	user, _ := database.GetUserByUsername("admin")
	if user.Email != "admin@example.com" { t.Errorf("email = %q", user.Email) }
}
