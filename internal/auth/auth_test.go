package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("mypassword")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty string")
	}
	if hash == "mypassword" {
		t.Fatal("HashPassword returned plaintext")
	}
}

func TestCheckPassword(t *testing.T) {
	hash, _ := HashPassword("correct")

	if !CheckPassword(hash, "correct") {
		t.Error("CheckPassword should return true for correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("CheckPassword should return false for wrong password")
	}
}

func TestSessionStore_CreateAndGet(t *testing.T) {
	store := NewSessionStore()

	token := store.Create(42)
	if token == "" {
		t.Fatal("Create returned empty token")
	}

	userID, ok := store.Get(token)
	if !ok {
		t.Fatal("Get returned not ok for valid token")
	}
	if userID != 42 {
		t.Errorf("Get returned userID %d, want 42", userID)
	}
}

func TestSessionStore_GetInvalidToken(t *testing.T) {
	store := NewSessionStore()

	_, ok := store.Get("nonexistent")
	if ok {
		t.Error("Get should return false for nonexistent token")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewSessionStore()

	token := store.Create(1)
	store.Delete(token)

	_, ok := store.Get(token)
	if ok {
		t.Error("Get should return false after Delete")
	}
}

func TestSessionStore_ExpiredSession(t *testing.T) {
	store := NewSessionStore()

	token := "expired-token"
	store.mu.Lock()
	store.sessions[token] = Session{
		UserID:    99,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	store.mu.Unlock()

	_, ok := store.Get(token)
	if ok {
		t.Error("Get should return false for expired session")
	}

	// Should also delete the expired session
	store.mu.RLock()
	_, exists := store.sessions[token]
	store.mu.RUnlock()
	if exists {
		t.Error("Expired session should be deleted after Get")
	}
}

func TestSetSessionCookie(t *testing.T) {
	w := httptest.NewRecorder()
	SetSessionCookie(w, "test-token")

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("No cookie set")
	}
	found := false
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			found = true
			if c.Value != "test-token" {
				t.Errorf("Cookie value = %q, want %q", c.Value, "test-token")
			}
			if !c.HttpOnly {
				t.Error("Cookie should be HttpOnly")
			}
		}
	}
	if !found {
		t.Errorf("Cookie %q not found", SessionCookieName)
	}
}

func TestClearSessionCookie(t *testing.T) {
	w := httptest.NewRecorder()
	ClearSessionCookie(w)

	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("Cookie MaxAge = %d, want -1", c.MaxAge)
			}
		}
	}
	if !found {
		t.Errorf("Cookie %q not found", SessionCookieName)
	}
}

func TestGetSessionToken(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "my-token"})

	got := GetSessionToken(req)
	if got != "my-token" {
		t.Errorf("GetSessionToken = %q, want %q", got, "my-token")
	}
}

func TestGetSessionToken_NoCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	got := GetSessionToken(req)
	if got != "" {
		t.Errorf("GetSessionToken = %q, want empty", got)
	}
}
