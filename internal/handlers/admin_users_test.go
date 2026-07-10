package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

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

// --- Invitations ---

func TestAdminInviteUser_Success(t *testing.T) {
	h, _, token := newTestHandler(t) // first user (id 1) = admin

	form := url.Values{"email": {"newbie@example.com"}}
	req := formRequest("/admin/invites", token, form)
	w := httptest.NewRecorder()

	h.AdminInviteUser(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/invite?token=") {
		t.Error("response should show the invite link")
	}
	invs, _ := h.DB.ListPendingInvitations()
	if len(invs) != 1 || invs[0].Email != "newbie@example.com" {
		t.Errorf("expected one pending invitation for newbie@example.com, got %+v", invs)
	}
}

func TestAdminInviteUser_InvalidEmail(t *testing.T) {
	h, _, token := newTestHandler(t)

	req := formRequest("/admin/invites", token, url.Values{"email": {"notanemail"}})
	w := httptest.NewRecorder()
	h.AdminInviteUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if invs, _ := h.DB.ListPendingInvitations(); len(invs) != 0 {
		t.Error("no invitation should be created for an invalid email")
	}
}

func TestAdminInviteUser_ExistingEmail(t *testing.T) {
	h, _, token := newTestHandler(t)
	// Give the admin an email, then try to invite it.
	h.DB.UpdateUserEmail(1, "taken@example.com")

	req := formRequest("/admin/invites", token, url.Values{"email": {"taken@example.com"}})
	w := httptest.NewRecorder()
	h.AdminInviteUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for existing email", w.Code)
	}
}

func TestAdminInviteUser_NonAdminForbidden(t *testing.T) {
	h, _, _ := newTestHandler(t)
	hash, _ := auth.HashPassword("password123")
	uid2, _ := h.DB.CreateUser("charlie", hash)
	token2 := h.Sessions.Create(uid2)

	req := formRequest("/admin/invites", token2, url.Values{"email": {"x@example.com"}})
	w := httptest.NewRecorder()
	h.AdminInviteUser(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect for non-admin", w.Code)
	}
	if invs, _ := h.DB.ListPendingInvitations(); len(invs) != 0 {
		t.Error("non-admin must not create invitations")
	}
}

func TestAdminRevokeInvite(t *testing.T) {
	h, _, token := newTestHandler(t)
	id, _ := h.DB.CreateInvitation("revoke@example.com", "tok-revoke", time.Now().Add(time.Hour))

	req := formRequest("/admin/invites/"+strconv.FormatInt(id, 10)+"/revoke", token, url.Values{})
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	h.AdminRevokeInvite(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if invs, _ := h.DB.ListPendingInvitations(); len(invs) != 0 {
		t.Error("invitation should have been revoked")
	}
}

func TestPageAcceptInvite_ValidToken(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.CreateInvitation("guest@example.com", "tok-valid", time.Now().Add(time.Hour))

	req := httptest.NewRequest("GET", "/invite?token=tok-valid", nil)
	w := httptest.NewRecorder()
	h.PageAcceptInvite(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "guest@example.com") {
		t.Error("invite page should show the invited email")
	}
	if !strings.Contains(body, `name="username"`) {
		t.Error("invite page should have a username field")
	}
}

func TestPageAcceptInvite_InvalidToken(t *testing.T) {
	h, _, _ := newTestHandler(t)
	req := httptest.NewRequest("GET", "/invite?token=nope", nil)
	w := httptest.NewRecorder()
	h.PageAcceptInvite(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), `name="username"`) {
		t.Error("invalid invite should not render the account form")
	}
}

func TestHandleAcceptInvite_WithPassword(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.CreateInvitation("withpass@example.com", "tok-wp", time.Now().Add(time.Hour))

	form := url.Values{"token": {"tok-wp"}, "username": {"withpass"}, "password": {"password123"}}
	req := httptest.NewRequest("POST", "/invite", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleAcceptInvite(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	user, err := h.DB.GetUserByUsername("withpass")
	if err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.Email != "withpass@example.com" {
		t.Errorf("email = %q, want withpass@example.com", user.Email)
	}
	if !auth.CheckPassword(user.PasswordHash, "password123") {
		t.Error("password should authenticate")
	}
	if _, ok := h.DB.GetInvitation("tok-wp"); ok {
		t.Error("invitation should be marked accepted")
	}
}

func TestHandleAcceptInvite_NoPassword(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.CreateInvitation("nopass@example.com", "tok-np", time.Now().Add(time.Hour))

	form := url.Values{"token": {"tok-np"}, "username": {"nopass"}}
	req := httptest.NewRequest("POST", "/invite", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleAcceptInvite(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	user, err := h.DB.GetUserByUsername("nopass")
	if err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.PasswordHash != "" {
		t.Error("password hash should be empty when no password is set")
	}
	if auth.CheckPassword(user.PasswordHash, "anything") {
		t.Error("no password must not authenticate any password")
	}
}

func TestHandleAcceptInvite_ShortPassword(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.DB.CreateInvitation("short@example.com", "tok-short", time.Now().Add(time.Hour))

	form := url.Values{"token": {"tok-short"}, "username": {"shorty"}, "password": {"abc"}}
	req := httptest.NewRequest("POST", "/invite", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleAcceptInvite(w, req)

	if _, ok := findUser(t, h, "shorty"); ok {
		t.Error("user should not be created with a short password")
	}
}

func TestHandleAcceptInvite_DuplicateUsername(t *testing.T) {
	h, _, _ := newTestHandler(t)
	// "testuser" already exists (id 1).
	h.DB.CreateInvitation("dup@example.com", "tok-dup", time.Now().Add(time.Hour))

	form := url.Values{"token": {"tok-dup"}, "username": {"testuser"}, "password": {"password123"}}
	req := httptest.NewRequest("POST", "/invite", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleAcceptInvite(w, req)

	// Re-renders the form (200), does not redirect, invitation stays valid.
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (re-render on duplicate username)", w.Code)
	}
	if _, ok := h.DB.GetInvitation("tok-dup"); !ok {
		t.Error("invitation should remain valid after a failed acceptance")
	}
}

func TestHandleAcceptInvite_InvalidToken(t *testing.T) {
	h, _, _ := newTestHandler(t)
	form := url.Values{"token": {"bad"}, "username": {"whoever"}, "password": {"password123"}}
	req := httptest.NewRequest("POST", "/invite", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleAcceptInvite(w, req)

	if _, ok := findUser(t, h, "whoever"); ok {
		t.Error("must not create a user from an invalid invitation")
	}
}

// --- Block / delete / listing (existing users) ---

func TestAdminToggleUserBlock(t *testing.T) {
	h, _, token := newTestHandler(t)
	hash, _ := auth.HashPassword("password123")
	uid, _ := h.DB.CreateUser("dave", hash)

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
	if !strings.Contains(w.Body.String(), "frank") {
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
