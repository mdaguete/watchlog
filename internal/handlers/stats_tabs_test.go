package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatsPage_HasHistoryTabs(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := authedRequest("GET", "/stats", token, nil)
	rec := httptest.NewRecorder()
	h.PageStats(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	// Tabs are now History + Stats.
	for _, want := range []string{`href="/timeline"`, `href="/stats"`} {
		if !strings.Contains(body, want) {
			t.Errorf("stats page missing tab link %s", want)
		}
	}
	// The activity heatmap links each month to its calendar.
	if !strings.Contains(body, `/calendar?month=`) {
		t.Errorf("stats heatmap missing per-month calendar links")
	}
}
