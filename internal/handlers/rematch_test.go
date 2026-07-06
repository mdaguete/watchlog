package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mdaguete/watchlog/internal/auth"
)

func TestAPIRelinkTMDB_Unauthenticated(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("POST", "/api/shows/1/relink-tmdb", strings.NewReader(`{"tmdb_id":203667}`))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIRelinkTMDB(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for unauthenticated", w.Code)
	}
}

func TestAPIRelinkTMDB_TMDBDisabled(t *testing.T) {
	h, _, token := newTestHandler(t) // TMDB client is nil in tests
	req := httptest.NewRequest("POST", "/api/shows/1/relink-tmdb", strings.NewReader(`{"tmdb_id":203667}`))
	req.SetPathValue("id", "1")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.APIRelinkTMDB(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when TMDB is not configured", w.Code)
	}
}

func TestAPIRematchSearch_TMDBDisabled(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := httptest.NewRequest("GET", "/api/shows/1/rematch?q=reina+roja", nil)
	req.SetPathValue("id", "1")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.APIRematchSearch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "TMDB") {
		t.Error("expected a 'TMDB not configured' style message")
	}
}

func TestAPIRematchSearch_Unauthenticated(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/api/shows/1/rematch?q=x", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.APIRematchSearch(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for unauthenticated", w.Code)
	}
}
