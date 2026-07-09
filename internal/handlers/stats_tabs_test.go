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
	for _, want := range []string{`href="/timeline"`, `href="/calendar"`, `href="/stats"`} {
		if !strings.Contains(body, want) {
			t.Errorf("stats page missing tab link %s", want)
		}
	}
}
