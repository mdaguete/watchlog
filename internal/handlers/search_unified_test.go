package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageSearch_HasAddSupport(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/search", token, nil)
	rec := httptest.NewRecorder()
	h.PageSearch(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	// The unified search page can add TMDB results directly.
	if !strings.Contains(body, "function addShow(") || !strings.Contains(body, "function addMovie(") {
		t.Errorf("search page missing add-from-TMDB JS")
	}
	if !strings.Contains(body, `hx-get="/search/results"`) {
		t.Errorf("search page missing results endpoint")
	}
}
