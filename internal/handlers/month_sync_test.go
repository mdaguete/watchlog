package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mdaguete/watchlog/internal/auth"
)

func TestCalendar_HistoryTabCarriesMonth(t *testing.T) {
	h, _, token := newTestHandler(t)
	req := httptest.NewRequest("GET", "/calendar?month=2024-03", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.PageCalendar(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `/timeline?month=2024-03`) {
		t.Error("calendar History tab should link back to the same month")
	}
}
