package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/models"
)

func TestPageCalendar_Authenticated(t *testing.T) {
	h, uid, token := newTestHandler(t)
	sid, _ := h.DB.UpsertShow(models.Show{ExternalID: 1, Name: "CalShow"})
	h.DB.MarkEpisodeWatched(uid, sid, 1, 1)
	// Move it to a known month.
	h.DB.UpdateEpisodeWatchedAt(uid, sid, 1, 1, time.Date(2024, 3, 15, 20, 0, 0, 0, time.UTC))

	req := httptest.NewRequest("GET", "/calendar?month=2024-03", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.PageCalendar(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "CalShow") {
		t.Error("calendar should show the watched show in its month")
	}
	if !strings.Contains(body, "calEdit") {
		t.Error("calendar items should be editable (calEdit handler)")
	}
}

func TestPageCalendar_DefaultsToCurrentMonth(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := httptest.NewRequest("GET", "/calendar", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.PageCalendar(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestPageCalendar_Unauthenticated(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/calendar", nil)
	w := httptest.NewRecorder()
	h.PageCalendar(w, req)
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect", w.Code)
	}
}
