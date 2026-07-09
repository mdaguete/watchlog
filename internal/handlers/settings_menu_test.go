package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsMenu_AdminHintAndDropdown(t *testing.T) {
	h, userID, token := newTestHandler(t)
	if userID != 1 {
		t.Skipf("test user is %d, admin-hint assertion assumes id 1", userID)
	}
	req := authedRequest("GET", "/settings", token, nil)
	rec := httptest.NewRecorder()
	h.PageSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	// Admin hint cookie set to 1 for user 1.
	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "wl_admin=1") {
		t.Errorf("expected wl_admin=1 cookie, got %q", setCookie)
	}
	body := rec.Body.String()
	// Dropdown and admin entry present in the nav.
	if !strings.Contains(body, `id="settings-menu"`) {
		t.Errorf("settings dropdown menu not rendered")
	}
	if !strings.Contains(body, `id="settings-admin-link"`) || !strings.Contains(body, `href="/admin"`) {
		t.Errorf("admin menu entry not rendered")
	}
	// The old inline admin link block in settings body must be gone.
	if strings.Contains(body, "settings.admin_link") {
		t.Errorf("old admin link key should not appear")
	}
}
