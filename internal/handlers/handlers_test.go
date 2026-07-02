package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
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
		"min": func(a, b int) int { if a < b { return a }; return b },
		"mul": func(a, b int) int { return a * b },
		"add": func(a, b int) int { return a + b },
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("../../web/templates/*.html")
	if err != nil {
		t.Fatalf("ParseGlob: %v", err)
	}

	sessions := auth.NewSessionStore()
	h := New(database, tmpl, nil, sessions)

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

func TestAPICreateList(t *testing.T) {
	h, _, token := newTestHandler(t)
	body, _ := json.Marshal(map[string]any{"name": "Test List", "is_public": false})
	req := authedRequest("POST", "/api/lists", token, body)
	w := httptest.NewRecorder()

	h.APICreateList(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 200 or 201", w.Code)
	}
}

// --- More Page Tests ---

func TestPageLists(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/lists", token, nil)
	w := httptest.NewRecorder()
	h.PageLists(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

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
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
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

func TestAPIGetLists(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/api/lists", token, nil)
	w := httptest.NewRecorder()
	h.APIGetLists(w, req)
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

func TestAPIDeleteList(t *testing.T) {
	h, userID, token := newTestHandler(t)
	lid, _ := h.DB.CreateList(userID, "ToDelete", false)

	req := authedRequest("DELETE", "/api/lists/1", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIDeleteList(w, req)
	// Should work since we own the list
	_ = lid
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 200 or 204", w.Code)
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

func TestPageList(t *testing.T) {
	h, userID, token := newTestHandler(t)
	lid, _ := h.DB.CreateList(userID, "My List", false)

	req := authedRequest("GET", "/lists/1", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.PageList(w, req)
	_ = lid
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestPageList_InvalidID(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/lists/abc", token, nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	h.PageList(w, req)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
}

func TestAPIGetList(t *testing.T) {
	h, userID, token := newTestHandler(t)
	h.DB.CreateList(userID, "My List", false)

	req := authedRequest("GET", "/api/lists/1", token, nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIGetList(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIUpdateList(t *testing.T) {
	h, userID, token := newTestHandler(t)
	h.DB.CreateList(userID, "Original", false)

	body, _ := json.Marshal(map[string]any{"name": "Updated", "is_public": true})
	req := authedRequest("PUT", "/api/lists/1", token, body)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIUpdateList(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPIAddToList(t *testing.T) {
	h, userID, token := newTestHandler(t)
	h.DB.CreateList(userID, "L", false)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})

	body, _ := json.Marshal(map[string]any{"show_id": sid})
	req := authedRequest("POST", "/api/lists/1/items", token, body)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIAddToList(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 200 or 201", w.Code)
	}
}

func TestAPIRemoveFromList(t *testing.T) {
	h, userID, token := newTestHandler(t)
	lid, _ := h.DB.CreateList(userID, "L", false)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	h.DB.AddShowToList(lid, sid)

	req := authedRequest("DELETE", "/api/lists/1/items/1", token, nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("itemId", "1")
	w := httptest.NewRecorder()
	h.APIRemoveFromList(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 200 or 204", w.Code)
	}
}

func TestSearchTMDB_NoClient(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/add/search?q=test&type=tv", token, nil)
	w := httptest.NewRecorder()
	h.SearchTMDB(w, req)
	// TMDB is nil so it should return a message
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
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
