package auth

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	SessionCookieName = "watchlog_session"
	SessionDuration   = 30 * 24 * time.Hour // 30 days
)

// SessionDB defines the persistence interface for session storage.
// Implemented by *db.DB to avoid circular imports.
type SessionDB interface {
	CreateSession(token string, userID int64, expiresAt time.Time) error
	GetSession(token string) (int64, bool)
	DeleteSession(token string) error
	CleanExpiredSessions() error
}

type SessionStore struct {
	db SessionDB
}

func NewSessionStore(db SessionDB) *SessionStore {
	store := &SessionStore{db: db}
	// Periodically clean expired sessions
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := store.db.CleanExpiredSessions(); err != nil {
				log.Printf("session cleanup error: %v", err)
			}
		}
	}()
	return store
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *SessionStore) Create(userID int64) string {
	token := generateToken()
	expiresAt := time.Now().Add(SessionDuration)
	if err := s.db.CreateSession(token, userID, expiresAt); err != nil {
		log.Printf("session create error: %v", err)
		return ""
	}
	return token
}

func (s *SessionStore) Get(token string) (int64, bool) {
	return s.db.GetSession(token)
}

func (s *SessionStore) Delete(token string) {
	s.db.DeleteSession(token)
}

func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionDuration.Seconds()),
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   SessionCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func GetSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateToken() string {
	return GenerateToken()
}
