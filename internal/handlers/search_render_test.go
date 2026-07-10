package handlers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mdaguete/watchlog/internal/models"
)

// TestSearchResults_RendersAll guards against the regression where the search
// results template errored on a per-user field, aborting render after one card.
func TestSearchResults_RendersAll(t *testing.T) {
	h, _, _ := newTestHandler(t)
	shows := []models.Show{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}
	movies := []models.Movie{{ID: 10, Name: "M1"}, {ID: 11, Name: "M2"}}
	var buf bytes.Buffer
	if err := h.Templates.ExecuteTemplate(&buf, "search_results.html", map[string]any{"Lang": "es", "Query": "x", "Shows": shows, "Movies": movies}); err != nil {
		t.Fatalf("render error: %v", err)
	}
	body := buf.String()
	if n := strings.Count(body, `href="/shows/`); n != 3 {
		t.Errorf("show cards = %d, want 3", n)
	}
}
