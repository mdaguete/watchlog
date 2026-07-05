package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mdaguete/watchlog/internal/auth"
)

// formRequest builds an authenticated form POST request.
func formRequest(path, token string, form url.Values) *http.Request {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	return req
}

func findUser(t *testing.T, h *Handler, username string) (int64, bool) {
	t.Helper()
	users, err := h.DB.ListAllUsers()
	if err != nil {
		t.Fatalf("ListAllUsers: %v", err)
	}
	for _, u := range users {
		if u.Username == username {
			return u.ID, true
		}
	}
	return 0, false
}

func TestAdminCreateUser_Success(t *testing.T) {
	h, _, token := newTestHandler(t) // first user (id 1) = admin

	form := url.Values{"username": {"alice"}, "email": {"alice@example.com"}, "password": {"password123"}}
	req := formRequest("/admin/users", token, form)
	w := httptest.NewRecorder()

	h.AdminCreateUser(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if _, ok := findUser(t, h, "alice"); !ok {
		t.Error("user alice was not created")
	}
}

func TestAdminCreateUser_ShortPassword(t *testing.T) {
	h, _, token := newTestHandler(t)

	form := url.Values{"username": {"bob"}, "password": {"short"}}
	req := formRequest("/admin/users", token, form)
	w := httptest.NewRecorder()

	h.AdminCreateUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if _, ok := findUser(t, h, "bob"); ok {
		t.Error("user bob should not have been created with a short password")
	}
}

func TestAdminCreateUser_DuplicateUsername(t *testing.T) {
	h, _, token := newTestHandler(t)

	// "testuser" already exists (created by newTestHandler as id 1)
	form := url.Values{"username": {"testuser"}, "password": {"password123"}}
	req := formRequest("/admin/users", token, form)
	w := httptest.NewRecorder()

	h.AdminCreateUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for duplicate username", w.Code)
	}
}

func TestAdminCreateUser_NonAdminForbidden(t *testing.T) {
	h, _, _ := newTestHandler(t)

	// Create a second, non-admin user and a session for them.
	hash, _ := auth.HashPassword("password123")
	uid2, _ := h.DB.CreateUser("charlie", hash)
	token2 := h.Sessions.Create(uid2)

	form := url.Values{"username": {"mallory"}, "password": {"password123"}}
	req := formRequest("/admin/users", token2, form)
	w := httptest.NewRecorder()

	h.AdminCreateUser(w, req)

	// Non-admin web request is redirected, not processed.
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect for non-admin", w.Code)
	}
	if _, ok := findUser(t, h, "mallory"); ok {
		t.Error("non-admin must not be able to create users")
	}
}

func TestAdminToggleUserBlock(t *testing.T) {
	h, _, token := newTestHandler(t)
	hash, _ := auth.HashPassword("password123")
	uid, _ := h.DB.CreateUser("dave", hash)

	// Block
	req := formRequest("/admin/users/"+strconv.FormatInt(uid, 10)+"/block", token, url.Values{})
	req.SetPathValue("id", strconv.FormatInt(uid, 10))
	w := httptest.NewRecorder()
	h.AdminToggleUserBlock(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("block status = %d, want 302", w.Code)
	}
	if !h.DB.IsUserBlocked(uid) {
		t.Error("user should be blocked")
	}

	// Unblock
	req2 := formRequest("/admin/users/"+strconv.FormatInt(uid, 10)+"/block", token, url.Values{})
	req2.SetPathValue("id", strconv.FormatInt(uid, 10))
	w2 := httptest.NewRecorder()
	h.AdminToggleUserBlock(w2, req2)
	if h.DB.IsUserBlocked(uid) {
		t.Error("user should be unblocked after second toggle")
	}
}

func TestAdminToggleUserBlock_AdminProtected(t *testing.T) {
	h, _, token := newTestHandler(t)

	req := formRequest("/admin/users/1/block", token, url.Values{})
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.AdminToggleUserBlock(w, req)

	if h.DB.IsUserBlocked(1) {
		t.Error("admin (id 1) must never be blockable")
	}
}

func TestAdminDeleteUser(t *testing.T) {
	h, _, token := newTestHandler(t)
	hash, _ := auth.HashPassword("password123")
	uid, _ := h.DB.CreateUser("erin", hash)

	req := formRequest("/admin/users/"+strconv.FormatInt(uid, 10)+"/delete", token, url.Values{})
	req.SetPathValue("id", strconv.FormatInt(uid, 10))
	w := httptest.NewRecorder()
	h.AdminDeleteUser(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("delete status = %d, want 302", w.Code)
	}
	if _, ok := findUser(t, h, "erin"); ok {
		t.Error("user erin should have been deleted")
	}
}

func TestAdminDeleteUser_AdminProtected(t *testing.T) {
	h, _, token := newTestHandler(t)

	req := formRequest("/admin/users/1/delete", token, url.Values{})
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.AdminDeleteUser(w, req)

	if _, ok := findUser(t, h, "testuser"); !ok {
		t.Error("admin (id 1) must never be deletable")
	}
}

func TestPageAdmin_ListsUsersForAdmin(t *testing.T) {
	h, _, token := newTestHandler(t)
	hash, _ := auth.HashPassword("password123")
	h.DB.CreateUser("frank", hash)

	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.PageAdmin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "frank") {
		t.Error("admin page should list the created user 'frank'")
	}
}

func TestPageAdmin_NonAdminRedirected(t *testing.T) {
	h, _, _ := newTestHandler(t)
	hash, _ := auth.HashPassword("password123")
	uid2, _ := h.DB.CreateUser("grace", hash)
	token2 := h.Sessions.Create(uid2)

	req := httptest.NewRequest("GET", "/admin", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token2})
	w := httptest.NewRecorder()
	h.PageAdmin(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect for non-admin", w.Code)
	}
}
