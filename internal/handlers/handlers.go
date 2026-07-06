package handlers

import (
	"archive/zip"
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/cache"
	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/i18n"
	"github.com/mdaguete/watchlog/internal/importer"
	"github.com/mdaguete/watchlog/internal/mail"
	"github.com/mdaguete/watchlog/internal/ratelimit"
	"github.com/mdaguete/watchlog/internal/tmdb"
	"github.com/mdaguete/watchlog/internal/worker"
)

type Handler struct {
	DB           *db.DB
	Templates    *template.Template
	TMDB         *tmdb.Client
	Sessions     *auth.SessionStore
	LoginLimiter *ratelimit.Limiter
	ImageCache   *cache.ImageCache
	DataDir      string
}

func New(database *db.DB, tmpl *template.Template, tmdbClient *tmdb.Client, sessions *auth.SessionStore, imgCache *cache.ImageCache, dataDir string) *Handler {
	return &Handler{
		DB:           database,
		Templates:    tmpl,
		TMDB:         tmdbClient,
		Sessions:     sessions,
		LoginLimiter: ratelimit.New(5, 15*time.Minute),
		ImageCache:   imgCache,
		DataDir:      dataDir,
	}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) currentUser(r *http.Request) int64 {
	token := auth.GetSessionToken(r)
	if token == "" {
		return 0
	}
	userID, ok := h.Sessions.Get(token)
	if !ok {
		return 0
	}
	return userID
}

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) int64 {
	userID := h.currentUser(r)
	if userID == 0 {
		if r.Header.Get("HX-Request") == "true" || strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusUnauthorized, "not authenticated")
		} else {
			http.Redirect(w, r, "/login", http.StatusFound)
		}
		return 0
	}
	return userID
}

// parsePathID extracts and validates a numeric path parameter. Returns 0 and writes a 400 error on failure.
func (h *Handler) parsePathID(w http.ResponseWriter, r *http.Request, param string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(param), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+param)
		return 0, false
	}
	return id, true
}

// clientIP extracts the client IP from the request. The X-Forwarded-For header
// is client-controllable, so it is only trusted when the "trust_proxy" setting
// is enabled (i.e. the app runs behind a reverse proxy that sets it). Otherwise
// the connection's RemoteAddr is used, preventing rate-limit evasion via a
// spoofed X-Forwarded-For header.
func (h *Handler) clientIP(r *http.Request) string {
	if h.DB.GetSetting("trust_proxy") == "true" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the first IP in the chain
			if idx := strings.Index(xff, ","); idx != -1 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
	}
	// Fall back to RemoteAddr (ip:port)
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

// getLang returns the user's language preference, falling back to Accept-Language header.
func (h *Handler) getLang(r *http.Request, userID int64) string {
	if userID > 0 {
		lang := h.DB.GetUserLang(userID)
		if lang != "" {
			return lang
		}
	}
	return i18n.DetectLang(r.Header.Get("Accept-Language"))
}

// --- Auth ---

func (h *Handler) PageLogin(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) != 0 {
		if h.DB.GetSetting("setup_complete") != "true" {
			http.Redirect(w, r, "/setup?step=2", http.StatusFound)
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
		}
		return
	}
	lang := h.getLang(r, 0)
	h.Templates.ExecuteTemplate(w, "login.html", map[string]any{
		"Lang":             lang,
		"PasswordEnabled":  h.DB.GetSetting("auth_password") != "disabled",
		"MagicLinkEnabled": h.DB.GetSetting("auth_magic_link") != "disabled",
		"DefaultLogin":     h.getDefaultLogin(),
		"RegistrationEnabled": h.DB.GetSetting("auth_registration") != "disabled",
	})
}

func (h *Handler) PageRegister(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) != 0 {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if h.DB.GetSetting("auth_registration") == "disabled" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	lang := h.getLang(r, 0)
	h.Templates.ExecuteTemplate(w, "register.html", map[string]any{"Lang": lang})
}

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if h.DB.GetSetting("auth_password") == "disabled" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	ip := h.clientIP(r)
	username := r.FormValue("username")
	password := r.FormValue("password")

	// Rate limit by IP and by target username. The username key caps brute-force
	// attempts against a single account even if the attacker rotates source IPs.
	userKey := "user:" + strings.ToLower(strings.TrimSpace(username))
	if !h.LoginLimiter.Allow(ip) || !h.LoginLimiter.Allow(userKey) {
		lang := h.getLang(r, 0)
		w.WriteHeader(http.StatusTooManyRequests)
		h.Templates.ExecuteTemplate(w, "login.html", map[string]any{"Error": i18n.T(lang, "login.rate_limited"), "Lang": lang})
		return
	}

	user, err := h.DB.GetUserByUsername(username)
	if err != nil || !auth.CheckPassword(user.PasswordHash, password) {
		h.LoginLimiter.Record(ip)
		h.LoginLimiter.Record(userKey)
		lang := h.getLang(r, 0)
		h.Templates.ExecuteTemplate(w, "login.html", map[string]any{"Error": i18n.T(lang, "login.error"), "Lang": lang})
		return
	}

	if h.DB.IsUserBlocked(user.ID) {
		lang := h.getLang(r, 0)
		h.Templates.ExecuteTemplate(w, "login.html", map[string]any{"Error": i18n.T(lang, "login.blocked"), "Lang": lang})
		return
	}

	h.LoginLimiter.Reset(ip)
	h.LoginLimiter.Reset(userKey)
	token := h.Sessions.Create(user.ID)
	auth.SetSessionCookie(w, token)
	// Set theme cookie for client-side dark mode
	theme := h.DB.GetUserTheme(user.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     "theme",
		Value:    theme,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	log.Printf("ACTION: login user=%q", username)
	// If setup incomplete, redirect to continue setup
	if h.DB.GetSetting("setup_complete") != "true" {
		http.Redirect(w, r, "/setup?step=2", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if h.DB.GetSetting("auth_registration") == "disabled" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	email := strings.TrimSpace(r.FormValue("email"))

	if username == "" || password == "" {
		lang := h.getLang(r, 0)
		h.Templates.ExecuteTemplate(w, "register.html", map[string]any{"Error": i18n.T(lang, "register.error.username_required"), "Lang": lang})
		return
	}
	if len(password) < 8 {
		lang := h.getLang(r, 0)
		h.Templates.ExecuteTemplate(w, "register.html", map[string]any{"Error": i18n.T(lang, "register.error.password_min"), "Lang": lang})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		lang := h.getLang(r, 0)
		h.Templates.ExecuteTemplate(w, "register.html", map[string]any{"Error": i18n.T(lang, "register.error.internal"), "Lang": lang})
		return
	}

	userID, err := h.DB.CreateUser(username, hash)
	if err != nil {
		lang := h.getLang(r, 0)
		h.Templates.ExecuteTemplate(w, "register.html", map[string]any{"Error": i18n.T(lang, "register.error.username_taken"), "Lang": lang})
		return
	}

	if email != "" {
		h.DB.UpdateUserEmail(userID, email)
	}

	token := h.Sessions.Create(userID)
	auth.SetSessionCookie(w, token)
	log.Printf("ACTION: register user=%q id=%d", username, userID)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	token := auth.GetSessionToken(r)
	if token != "" {
		h.Sessions.Delete(token)
	}
	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// --- SMTP Config Helper ---

func (h *Handler) getSMTPConfig() mail.Config {
	smtpURL := h.DB.GetSetting("smtp_url")
	if smtpURL == "" {
		return mail.Config{}
	}
	cfg, err := mail.ParseURL(smtpURL)
	if err != nil {
		log.Printf("SMTP: invalid URL: %v", err)
		return mail.Config{}
	}
	return cfg
}

func (h *Handler) getWatchLogURL() string {
	return h.DB.GetSetting("watchlog_url")
}

func (h *Handler) getDefaultLogin() string {
	d := h.DB.GetSetting("auth_default_login")
	if d == "" {
		return "magic"
	}
	return d
}

// --- Forgot Password ---

func (h *Handler) PageForgotPassword(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	smtpCfg := h.getSMTPConfig()
	h.Templates.ExecuteTemplate(w, "forgot_password.html", map[string]any{
		"Lang":          lang,
		"SMTPConfigured": smtpCfg.Configured(),
	})
}

func (h *Handler) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	smtpCfg := h.getSMTPConfig()

	if !smtpCfg.Configured() {
		h.Templates.ExecuteTemplate(w, "forgot_password.html", map[string]any{
			"Lang":  lang,
			"Error": i18n.T(lang, "forgot.smtp_disabled"),
		})
		return
	}

	input := strings.TrimSpace(r.FormValue("username_or_email"))
	// Always show success to prevent user enumeration
	successData := map[string]any{
		"Lang":    lang,
		"Success": i18n.T(lang, "forgot.success"),
		"SMTPConfigured": true,
	}

	// Try to find user by username or email
	user, err := h.DB.GetUserByUsername(input)
	if err != nil {
		user, err = h.DB.GetUserByEmail(input)
	}
	if err != nil || user.Email == "" {
		h.Templates.ExecuteTemplate(w, "forgot_password.html", successData)
		return
	}

	token := auth.GenerateToken()
	expiresAt := time.Now().Add(1 * time.Hour)
	if err := h.DB.CreateMagicLink(token, user.ID, "reset", expiresAt); err != nil {
		log.Printf("failed to create magic link: %v", err)
		h.Templates.ExecuteTemplate(w, "forgot_password.html", successData)
		return
	}

	baseURL := h.getWatchLogURL()
	resetURL := baseURL + "/reset-password?token=" + token
	body := fmt.Sprintf(`<p>%s</p><p><a href="%s">%s</a></p>`,
		html.EscapeString(i18n.T(lang, "forgot.title")),
		html.EscapeString(resetURL),
		html.EscapeString(resetURL))

	if err := mail.Send(smtpCfg, user.Email, i18n.T(lang, "email.reset_subject"), body); err != nil {
		log.Printf("failed to send reset email to %s: %v", user.Email, err)
	} else {
		log.Printf("ACTION: password reset email sent to user=%q", user.Username)
	}

	h.Templates.ExecuteTemplate(w, "forgot_password.html", successData)
}

// --- Reset Password ---

func (h *Handler) PageResetPassword(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	token := r.URL.Query().Get("token")
	_, purpose, ok := h.DB.GetMagicLink(token)
	if !ok || purpose != "reset" {
		h.Templates.ExecuteTemplate(w, "reset_password.html", map[string]any{
			"Lang":  lang,
			"Error": i18n.T(lang, "reset.invalid_token"),
		})
		return
	}
	h.Templates.ExecuteTemplate(w, "reset_password.html", map[string]any{
		"Lang":  lang,
		"Token": token,
	})
}

func (h *Handler) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	token := r.FormValue("token")
	password := r.FormValue("password")

	userID, purpose, ok := h.DB.GetMagicLink(token)
	if !ok || purpose != "reset" {
		h.Templates.ExecuteTemplate(w, "reset_password.html", map[string]any{
			"Lang":  lang,
			"Error": i18n.T(lang, "reset.invalid_token"),
		})
		return
	}

	if len(password) < 8 {
		h.Templates.ExecuteTemplate(w, "reset_password.html", map[string]any{
			"Lang":  lang,
			"Token": token,
			"Error": i18n.T(lang, "reset.password_min"),
		})
		return
	}

	hash, _ := auth.HashPassword(password)
	h.DB.UpdateUserPassword(userID, hash)
	h.DB.DeleteMagicLink(token)
	log.Printf("ACTION: password reset for user_id=%d", userID)

	h.Templates.ExecuteTemplate(w, "login.html", map[string]any{
		"Lang":    lang,
		"Success": i18n.T(lang, "reset.success"),
	})
}

// --- Magic Login ---

func (h *Handler) PageMagicLogin(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	smtpCfg := h.getSMTPConfig()
	h.Templates.ExecuteTemplate(w, "magic_login.html", map[string]any{
		"Lang":          lang,
		"SMTPConfigured": smtpCfg.Configured(),
	})
}

func (h *Handler) HandleMagicLogin(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	smtpCfg := h.getSMTPConfig()

	if !smtpCfg.Configured() {
		h.Templates.ExecuteTemplate(w, "magic_login.html", map[string]any{
			"Lang":  lang,
			"Error": i18n.T(lang, "magic.smtp_disabled"),
		})
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	// Always show success to prevent user enumeration
	successData := map[string]any{
		"Lang":           lang,
		"Success":        i18n.T(lang, "magic.success"),
		"SMTPConfigured": true,
	}

	if username == "" {
		h.Templates.ExecuteTemplate(w, "magic_login.html", successData)
		return
	}

	user, err := h.DB.GetUserByUsername(username)
	if err != nil || user.Email == "" {
		// User not found or has no email — show success anyway (don't reveal)
		h.Templates.ExecuteTemplate(w, "magic_login.html", successData)
		return
	}

	token := auth.GenerateToken()
	expiresAt := time.Now().Add(1 * time.Hour)
	if err := h.DB.CreateMagicLink(token, user.ID, "login", expiresAt); err != nil {
		log.Printf("failed to create magic link: %v", err)
		h.Templates.ExecuteTemplate(w, "magic_login.html", successData)
		return
	}

	baseURL := h.getWatchLogURL()
	magicURL := baseURL + "/auth/magic?token=" + token
	body := fmt.Sprintf(`<p>%s</p><p><a href="%s">%s</a></p>`,
		html.EscapeString(i18n.T(lang, "magic.title")),
		html.EscapeString(magicURL),
		html.EscapeString(magicURL))

	if err := mail.Send(smtpCfg, user.Email, i18n.T(lang, "email.magic_subject"), body); err != nil {
		log.Printf("failed to send magic link email to %s: %v", user.Email, err)
	} else {
		log.Printf("ACTION: magic login email sent to user=%q", user.Username)
	}

	h.Templates.ExecuteTemplate(w, "magic_login.html", successData)
}

func (h *Handler) HandleMagicAuth(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	token := r.URL.Query().Get("token")

	userID, purpose, ok := h.DB.GetMagicLink(token)
	if !ok || purpose != "login" {
		h.Templates.ExecuteTemplate(w, "magic_login.html", map[string]any{
			"Lang":  lang,
			"Error": i18n.T(lang, "magic.invalid_token"),
			"SMTPConfigured": h.getSMTPConfig().Configured(),
		})
		return
	}

	h.DB.DeleteMagicLink(token)
	sessionToken := h.Sessions.Create(userID)
	auth.SetSessionCookie(w, sessionToken)
	log.Printf("ACTION: magic login user_id=%d", userID)
	http.Redirect(w, r, "/", http.StatusFound)
}

// --- Pages ---

func (h *Handler) PageDashboard(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	stats, _ := h.DB.GetDashboardStats(userID)
	continueWatching, _ := h.DB.GetContinueWatching(userID, 5)
	newSeasons, _ := h.DB.GetNewSeasons(userID)
	h.Templates.ExecuteTemplate(w, "dashboard.html", map[string]any{
		"Lang":             lang,
		"Stats":            stats,
		"Runtime":          importer.FormatRuntime(stats.TotalRuntime),
		"ContinueWatching": continueWatching,
		"NewSeasons":       newSeasons,
		"Page":             1,
		"HasMore":          len(continueWatching) == 5,
	})
}

func (h *Handler) APIContinueWatching(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 { page = 1 }
	offset := (page - 1) * 5
	items, _ := h.DB.GetContinueWatching(userID, 5, offset)
	h.Templates.ExecuteTemplate(w, "continue_watching_items.html", map[string]any{
		"Lang":             lang,
		"ContinueWatching": items,
		"Page":             page,
		"HasMore":          len(items) == 5,
	})
}

func (h *Handler) PageShows(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	sort := r.URL.Query().Get("sort")
	if sort == "" { sort = "recent" }
	filter := r.URL.Query().Get("filter")
	shows, _ := h.DB.GetUserShowsFiltered(userID, sort, filter)
	h.Templates.ExecuteTemplate(w, "shows.html", map[string]any{
		"Lang":   lang,
		"Shows":  shows,
		"Sort":   sort,
		"Filter": filter,
	})
}

func (h *Handler) PageShow(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil { http.Redirect(w, r, "/shows", http.StatusFound); return }

	show, err := h.DB.GetUserShow(userID, id)
	if err != nil {
		// Show exists in catalog but user hasn't followed — show catalog info
		catalogShow, err2 := h.DB.GetShow(id)
		if err2 != nil { http.Redirect(w, r, "/shows", http.StatusFound); return }
		show.Show = catalogShow
	}

	episodes, _ := h.DB.GetEpisodesByShow(userID, id)
	progress, _ := h.DB.GetShowProgress(userID, id)

	// Build seasons map: read from cached season_episodes table first
	seasons, _ := h.DB.GetSeasonEpisodes(id)
	if len(seasons) == 0 && show.TMDBID > 0 && h.TMDB != nil && h.TMDB.Enabled() {
		// Fallback: fetch from TMDB and cache for next time
		tvShow, err := h.TMDB.GetTVShow(show.TMDBID)
		if err == nil {
			for _, s := range tvShow.Seasons {
				if s.SeasonNumber > 0 {
					seasons[s.SeasonNumber] = s.EpisodeCount
					h.DB.UpsertSeasonEpisodes(id, s.SeasonNumber, s.EpisodeCount)
				}
			}
		}
	}
	// Fill in from watched data if nothing else available
	for _, ep := range episodes {
		if _, ok := seasons[ep.SeasonNumber]; !ok {
			seasons[ep.SeasonNumber] = ep.EpisodeNumber
		} else if ep.EpisodeNumber > seasons[ep.SeasonNumber] {
			seasons[ep.SeasonNumber] = ep.EpisodeNumber
		}
	}

	episodeDetails, _ := h.DB.GetEpisodeDetails(id)
	h.Templates.ExecuteTemplate(w, "show.html", map[string]any{
		"Lang":           h.getLang(r, userID),
		"Show":           show,
		"Episodes":       episodes,
		"Progress":       progress,
		"Seasons":        seasons,
		"EpisodeDetails": episodeDetails,
	})
}

func (h *Handler) PageMovies(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	sort := r.URL.Query().Get("sort")
	if sort == "" { sort = "recent" }
	movies, _ := h.DB.GetUserMoviesSorted(userID, sort)
	unwatched, _ := h.DB.GetUserMoviesUnwatched(userID)
	stats, _ := h.DB.GetMovieStats(userID)
	h.Templates.ExecuteTemplate(w, "movies.html", map[string]any{
		"Lang":      lang,
		"Movies":    movies,
		"Unwatched": unwatched,
		"Sort":      sort,
		"Stats":     stats,
		"Runtime":   importer.FormatRuntime(stats.TotalRuntime),
	})
}

func (h *Handler) PageMovie(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil { http.Redirect(w, r, "/movies", http.StatusFound); return }
	movie, err := h.DB.GetMovie(id)
	if err != nil { http.Redirect(w, r, "/movies", http.StatusFound); return }
	lang := h.getLang(r, userID)
	watched := h.DB.IsMovieWatched(userID, id)
	data := map[string]any{
		"Lang":    lang,
		"Movie":   movie,
		"Watched": watched,
	}
	if watched {
		if at, ok := h.DB.GetMovieWatchedAt(userID, id); ok && !at.IsZero() {
			data["WatchedInput"] = at.Format("2006-01-02T15:04")
			data["WatchedDisplay"] = at.Format("2006-01-02 15:04")
		}
	}
	h.Templates.ExecuteTemplate(w, "movie.html", data)
}

func (h *Handler) PageLists(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	lists, _ := h.DB.GetUserLists(userID)
	h.Templates.ExecuteTemplate(w, "lists.html", map[string]any{"Lang": lang, "Lists": lists})
}

func (h *Handler) PageList(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil { http.Redirect(w, r, "/lists", http.StatusFound); return }
	list, err := h.DB.GetListWithItems(id)
	if err != nil || list.UserID != userID { http.Redirect(w, r, "/lists", http.StatusFound); return }
	h.Templates.ExecuteTemplate(w, "list.html", map[string]any{"Lang": lang, "List": list})
}

func (h *Handler) PageStats(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	stats, _ := h.DB.GetUserWatchStats(userID)
	dashboard, _ := h.DB.GetDashboardStats(userID)
	h.Templates.ExecuteTemplate(w, "stats.html", map[string]any{
		"Lang":       lang,
		"WatchStats": stats,
		"Dashboard":  dashboard,
		"Runtime":    importer.FormatRuntime(dashboard.TotalRuntime),
	})
}

func (h *Handler) PageTimeline(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	items, _ := h.DB.GetTimelineItems(userID, "", 100)
	days := groupByDay(items)
	if len(days) > 10 {
		days = days[:10]
	}
	lastDate := ""
	if len(days) > 0 {
		lastDate = days[len(days)-1].Date
	}
	years, _ := h.DB.GetTimelineYears(userID)
	h.Templates.ExecuteTemplate(w, "timeline.html", map[string]any{
		"Lang":     lang,
		"Days":     days,
		"LastDate": lastDate,
		"HasMore":  len(days) == 10,
		"Years":    years,
	})
}

type timelineDay struct {
	Date  string
	Items []db.TimelineItem
}

func groupByDay(items []db.TimelineItem) []timelineDay {
	var days []timelineDay
	for _, item := range items {
		if len(days) == 0 || days[len(days)-1].Date != item.Date {
			days = append(days, timelineDay{Date: item.Date})
		}
		days[len(days)-1].Items = append(days[len(days)-1].Items, item)
	}
	return days
}

func (h *Handler) APITimelineItems(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	before := r.URL.Query().Get("before")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	var items []db.TimelineItem
	if from != "" && to != "" {
		items, _ = h.DB.GetTimelineItemsForRange(userID, from, to)
	} else if before != "" {
		items, _ = h.DB.GetTimelineItems(userID, before, 100)
	}

	days := groupByDay(items)
	if from == "" && len(days) > 10 {
		days = days[:10]
	}
	lastDate := ""
	if len(days) > 0 {
		lastDate = days[len(days)-1].Date
	}
	h.Templates.ExecuteTemplate(w, "timeline_items.html", map[string]any{
		"Lang":     lang,
		"Days":     days,
		"LastDate": lastDate,
		"HasMore":  from == "" && len(days) == 10,
	})
}

func (h *Handler) PageSearch(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	h.Templates.ExecuteTemplate(w, "search.html", map[string]any{"Lang": lang})
}

func (h *Handler) SearchResults(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	query := r.URL.Query().Get("q")
	if query == "" { w.Write([]byte("")); return }
	shows, _ := h.DB.SearchShows(query)
	movies, _ := h.DB.SearchMovies(query)
	h.Templates.ExecuteTemplate(w, "search_results.html", map[string]any{
		"Lang": lang, "Query": query, "Shows": shows, "Movies": movies,
	})
}

func (h *Handler) PageAddShow(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	h.Templates.ExecuteTemplate(w, "add.html", map[string]any{"Lang": lang})
}

func (h *Handler) SearchTMDB(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	if h.TMDB == nil || !h.TMDB.Enabled() {
		w.Write([]byte(`<p class="text-sm text-wl-gray">` + i18n.T(lang, "tmdb.not_configured") + `</p>`))
		return
	}
	query := r.URL.Query().Get("q")
	mediaType := r.URL.Query().Get("type")
	if query == "" { w.Write([]byte("")); return }

	if mediaType == "movie" {
		results, _ := h.TMDB.SearchMovie(query)
		h.Templates.ExecuteTemplate(w, "tmdb_movie_results.html", map[string]any{"Lang": lang, "Results": results})
	} else {
		results, _ := h.TMDB.SearchTV(query)
		h.Templates.ExecuteTemplate(w, "tmdb_show_results.html", map[string]any{"Lang": lang, "Results": results})
	}
}

// PageCalendar renders a monthly calendar of watched episodes and movies.
func (h *Handler) PageCalendar(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	lang := h.getLang(r, userID)

	cur, err := time.ParseInLocation("2006-01", r.URL.Query().Get("month"), time.Local)
	if err != nil {
		now := time.Now()
		cur = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	}
	firstOfMonth := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, time.Local)
	nextMonth := firstOfMonth.AddDate(0, 1, 0)
	lastOfMonth := nextMonth.AddDate(0, 0, -1)

	items, _ := h.DB.GetWatchedItemsInRange(userID, firstOfMonth.Format("2006-01-02"), lastOfMonth.Format("2006-01-02"))
	byDay := map[string][]db.CalendarItem{}
	for _, it := range items {
		byDay[it.Date] = append(byDay[it.Date], it)
	}

	type Day struct {
		Date    string
		Day     int
		InMonth bool
		Items   []db.CalendarItem
	}
	// Monday-first grid: convert Sunday=0..Saturday=6 to Monday=0.
	offset := (int(firstOfMonth.Weekday()) + 6) % 7
	var days []Day
	for i := offset; i > 0; i-- {
		d := firstOfMonth.AddDate(0, 0, -i)
		days = append(days, Day{Date: d.Format("2006-01-02"), Day: d.Day()})
	}
	for d := firstOfMonth; d.Before(nextMonth); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		days = append(days, Day{Date: ds, Day: d.Day(), InMonth: true, Items: byDay[ds]})
	}
	for len(days)%7 != 0 {
		lt, _ := time.ParseInLocation("2006-01-02", days[len(days)-1].Date, time.Local)
		nd := lt.AddDate(0, 0, 1)
		days = append(days, Day{Date: nd.Format("2006-01-02"), Day: nd.Day()})
	}
	var weeks [][]Day
	for i := 0; i < len(days); i += 7 {
		weeks = append(weeks, days[i:i+7])
	}

	monthsES := []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	monthsEN := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	monthName := monthsEN[int(firstOfMonth.Month())-1]
	if lang == "es" {
		monthName = monthsES[int(firstOfMonth.Month())-1]
	}

	h.Templates.ExecuteTemplate(w, "calendar.html", map[string]any{
		"Lang":       lang,
		"Weeks":      weeks,
		"MonthLabel": fmt.Sprintf("%s %d", monthName, firstOfMonth.Year()),
		"PrevMonth":  firstOfMonth.AddDate(0, -1, 0).Format("2006-01"),
		"NextMonth":  nextMonth.Format("2006-01"),
		"Today":      time.Now().Format("2006-01-02"),
	})
}

func (h *Handler) PageUpcoming(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	cached, _ := h.DB.GetUpcomingCacheForUser(userID)
	h.Templates.ExecuteTemplate(w, "upcoming.html", map[string]any{
		"Lang":     lang,
		"Upcoming": cached,
	})
}

// --- API Handlers ---

func (h *Handler) APIGetShows(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	shows, _ := h.DB.GetUserShowsSorted(userID, "name")
	writeJSON(w, http.StatusOK, shows)
}

func (h *Handler) APIGetShow(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	show, err := h.DB.GetUserShow(userID, id)
	if err != nil { writeError(w, http.StatusNotFound, "not found"); return }
	writeJSON(w, http.StatusOK, show)
}

func (h *Handler) APIToggleFollow(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	h.DB.ToggleUserShowFollow(userID, id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIToggleFavorite(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	h.DB.ToggleUserShowFavorite(userID, id)
	var isFav bool
	h.DB.GetUserShowField(userID, id, "is_favorited", &isFav)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		star := "☆"
		cls := "px-3 py-1.5 text-xs uppercase tracking-widest border bg-white text-black border-wl-border hover:border-black transition-colors"
		if isFav {
			star = "★"
			cls = "px-3 py-1.5 text-xs uppercase tracking-widest border bg-black text-white border-black transition-colors"
		}
		fmt.Fprintf(w, `<button hx-post="/api/shows/%d/favorite" hx-swap="outerHTML" class="%s">%s</button>`, id, cls, star)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "is_favorited": isFav})
}

func (h *Handler) APIToggleArchive(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	h.DB.ToggleUserShowArchive(userID, id)
	var isArchived bool
	h.DB.GetUserShowField(userID, id, "is_archived", &isArchived)
	lang := h.getLang(r, userID)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		label := i18n.T(lang, "show.archive")
		cls := "px-3 py-1.5 text-xs uppercase tracking-widest border bg-white text-black border-wl-border hover:border-black transition-colors"
		if isArchived {
			cls = "px-3 py-1.5 text-xs uppercase tracking-widest border bg-black text-white border-black transition-colors"
		}
		fmt.Fprintf(w, `<button hx-post="/api/shows/%d/archive" hx-swap="outerHTML" class="%s">%s</button>`, id, cls, label)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "is_archived": isArchived})
}

func (h *Handler) APISnoozeShow(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	// Snooze indefinitely — only unsnoozes when user marks an episode
	until := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	h.DB.SnoozeShow(userID, id, until)
	log.Printf("ACTION: user=%d snooze show=%d", userID, id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIGetEpisodes(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	showID, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	episodes, _ := h.DB.GetEpisodesByShow(userID, showID)
	writeJSON(w, http.StatusOK, episodes)
}

func (h *Handler) APIMarkEpisodeWatched(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	showID, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	var req struct { Season int `json:"season"`; Episode int `json:"episode"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body"); return
	}
	h.DB.MarkEpisodeWatched(userID, showID, req.Season, req.Episode)
	h.DB.IncrementWatchStats(userID, 1)
	h.DB.UnsnoozeShow(userID, showID)
	archived := h.DB.AutoArchiveIfComplete(userID, showID)
	log.Printf("ACTION: user=%d mark watched show=%d S%02dE%02d", userID, showID, req.Season, req.Episode)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-xs text-wl-gray">✓ S%02dE%02d</span>`, req.Season, req.Episode)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "archived": archived})
}

func (h *Handler) APIUnmarkEpisodeWatched(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	showID, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	var req struct { Season int `json:"season"`; Episode int `json:"episode"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body"); return
	}
	h.DB.UnmarkEpisodeWatched(userID, showID, req.Season, req.Episode)
	unarchived := h.DB.AutoUnarchiveIfIncomplete(userID, showID)
	log.Printf("ACTION: user=%d unmark watched show=%d S%02dE%02d", userID, showID, req.Season, req.Episode)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "unarchived": unarchived})
}

func (h *Handler) APIMarkSeasonWatched(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	showID, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	var req struct { Season int `json:"season"`; Episodes int `json:"episodes"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body"); return
	}
	if req.Episodes <= 0 {
		show, _ := h.DB.GetShow(showID)
		if show.TMDBID > 0 && h.TMDB != nil && h.TMDB.Enabled() {
			season, err := h.TMDB.GetSeason(show.TMDBID, req.Season)
			if err == nil { req.Episodes = len(season.Episodes) }
		}
		if req.Episodes <= 0 { writeError(w, http.StatusBadRequest, "specify episodes"); return }
	}
	marked, _ := h.DB.MarkSeasonWatched(userID, showID, req.Season, req.Episodes)
	if marked > 0 {
		h.DB.IncrementWatchStats(userID, marked)
	}
	archived := h.DB.AutoArchiveIfComplete(userID, showID)
	log.Printf("ACTION: user=%d mark season show=%d S%02d (%d eps)", userID, showID, req.Season, marked)
	if r.Header.Get("HX-Request") == "true" {
		lang := h.getLang(r, userID)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-xs text-wl-gray">✓ `+i18n.T(lang, "tmdb.season_marked")+`</span>`, req.Season, marked)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "marked": marked, "archived": archived})
}

func (h *Handler) APIUnmarkSeasonWatched(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	showID, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	var req struct { Season int `json:"season"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body"); return
	}
	removed, _ := h.DB.UnmarkSeasonWatched(userID, showID, req.Season)
	unarchived := h.DB.AutoUnarchiveIfIncomplete(userID, showID)
	log.Printf("ACTION: user=%d unmark season show=%d S%02d (%d eps)", userID, showID, req.Season, removed)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "removed": removed, "unarchived": unarchived})
}

func (h *Handler) APIGetMovies(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	movies, _ := h.DB.GetUserMoviesSorted(userID, "name")
	writeJSON(w, http.StatusOK, movies)
}

func (h *Handler) APIMarkMovieWatched(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	h.DB.MarkMovieWatched(userID, id, time.Now())
	log.Printf("ACTION: user=%d mark movie watched id=%d", userID, id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// parseWatchedInput parses a datetime from an <input type="datetime-local">
// value (local wall clock), tolerating with/without seconds.
func parseWatchedInput(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// APISetEpisodeDate updates the watched date/time of a watched episode.
func (h *Handler) APISetEpisodeDate(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	showID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Season   int    `json:"season"`
		Episode  int    `json:"episode"`
		Datetime string `json:"datetime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	at, ok := parseWatchedInput(req.Datetime)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid datetime")
		return
	}
	n, err := h.DB.UpdateEpisodeWatchedAt(userID, showID, req.Season, req.Episode, at)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "episode not watched")
		return
	}
	log.Printf("ACTION: user=%d set date show=%d S%02dE%02d", userID, showID, req.Season, req.Episode)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// APISetMovieDate updates the watched date/time of a watched movie.
func (h *Handler) APISetMovieDate(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	id, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Datetime string `json:"datetime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	at, ok := parseWatchedInput(req.Datetime)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid datetime")
		return
	}
	if !h.DB.IsMovieWatched(userID, id) {
		writeError(w, http.StatusNotFound, "movie not watched")
		return
	}
	h.DB.MarkMovieWatched(userID, id, at)
	log.Printf("ACTION: user=%d set movie date id=%d", userID, id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIUnmarkMovieWatched(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	h.DB.UnmarkMovieWatched(userID, id)
	log.Printf("ACTION: user=%d unmark movie watched id=%d", userID, id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIGetLists(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lists, _ := h.DB.GetUserLists(userID)
	writeJSON(w, http.StatusOK, lists)
}

func (h *Handler) APIGetList(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	list, err := h.DB.GetListWithItems(id)
	if err != nil || list.UserID != userID { writeError(w, http.StatusNotFound, "not found"); return }
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) APICreateList(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	var req struct { Name string `json:"name"`; IsPublic bool `json:"is_public"` }
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" { writeError(w, http.StatusBadRequest, "name required"); return }
	id, _ := h.DB.CreateList(userID, req.Name, req.IsPublic)
	log.Printf("ACTION: user=%d create list %q id=%d", userID, req.Name, id)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", fmt.Sprintf("/lists/%d", id))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) APIUpdateList(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	list, _ := h.DB.GetListWithItems(id)
	if list.UserID != userID { writeError(w, http.StatusForbidden, "forbidden"); return }
	var req struct { Name string `json:"name"`; IsPublic bool `json:"is_public"` }
	json.NewDecoder(r.Body).Decode(&req)
	h.DB.UpdateList(id, req.Name, req.IsPublic)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIDeleteList(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	list, _ := h.DB.GetListWithItems(id)
	if list.UserID != userID { writeError(w, http.StatusForbidden, "forbidden"); return }
	h.DB.DeleteList(id)
	log.Printf("ACTION: user=%d delete list id=%d", userID, id)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/lists")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIAddToList(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	listID, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	list, _ := h.DB.GetListWithItems(listID)
	if list.UserID != userID { writeError(w, http.StatusForbidden, "forbidden"); return }
	var req struct { ShowID int64 `json:"show_id"`; MovieID int64 `json:"movie_id"` }
	json.NewDecoder(r.Body).Decode(&req)
	if req.ShowID > 0 { h.DB.AddShowToList(listID, req.ShowID) }
	if req.MovieID > 0 { h.DB.AddMovieToList(listID, req.MovieID) }
	if r.Header.Get("HX-Request") == "true" {
		lang := h.getLang(r, userID)
		w.Write([]byte(`<span class="text-xs text-wl-gray">✓ ` + i18n.T(lang, "tmdb.list_added") + `</span>`))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIRemoveFromList(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	listID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil { writeError(w, http.StatusBadRequest, "invalid list id"); return }
	list, err := h.DB.GetListWithItems(listID)
	if err != nil || list.UserID != userID { writeError(w, http.StatusForbidden, "forbidden"); return }
	itemID, err := strconv.ParseInt(r.PathValue("itemId"), 10, 64)
	if err != nil { writeError(w, http.StatusBadRequest, "invalid item id"); return }
	h.DB.RemoveListItem(listID, itemID)
	if r.Header.Get("HX-Request") == "true" { w.Write([]byte("")); return }
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIGetStats(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	stats, _ := h.DB.GetDashboardStats(userID)
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) APIGetWatchStats(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	stats, _ := h.DB.GetUserWatchStats(userID)
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) APISearch(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	query := r.URL.Query().Get("q")
	if query == "" { writeJSON(w, http.StatusOK, map[string]any{"shows": []any{}, "movies": []any{}}); return }
	shows, _ := h.DB.SearchShows(query)
	movies, _ := h.DB.SearchMovies(query)
	writeJSON(w, http.StatusOK, map[string]any{"shows": shows, "movies": movies})
}

// --- TMDB API ---

func (h *Handler) APIFetchTMDB(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	if h.TMDB == nil || !h.TMDB.Enabled() { writeError(w, http.StatusServiceUnavailable, "TMDB not configured"); return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	show, err := h.DB.GetShow(id)
	if err != nil { writeError(w, http.StatusNotFound, "show not found"); return }
	// If the show is already linked to a TMDB entry (including a manual re-link),
	// refresh by that id instead of re-matching by name, so a correct match is
	// never silently overwritten by a title search.
	if show.TMDBID > 0 {
		if err := worker.RefreshShowByTMDB(h.DB, h.TMDB, id, show.TMDBID); err != nil {
			writeError(w, http.StatusBadGateway, "refresh failed"); return
		}
		log.Printf("TMDB: refreshed show=%d by existing tmdb_id=%d", id, show.TMDBID)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "tmdb_id": show.TMDBID})
		return
	}
	result, err := h.TMDB.FindTVByName(show.Name)
	if err != nil { writeError(w, http.StatusNotFound, "not found on TMDB"); return }
	genres := extractGenreNames(result.Genres)
	posterURL := tmdb.PosterURL(result.PosterPath, "w342")
	backdropURL := tmdb.BackdropURL(result.BackdropPath, "w780")
	h.DB.UpdateShowTMDB(id, result.ID, posterURL, backdropURL, result.Overview, genres, result.Status, len(result.Seasons))
	log.Printf("TMDB: fetched %q tmdb_id=%d", show.Name, result.ID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "tmdb_id": result.ID})
}

// APIRematchSearch searches TMDB for the correct show so the user can re-link a
// mismatched entry. Returns an HTMX fragment with a "use this" action per result.
func (h *Handler) APIRematchSearch(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	lang := h.getLang(r, userID)
	id, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if h.TMDB == nil || !h.TMDB.Enabled() {
		w.Write([]byte(`<p class="text-sm text-wl-gray">` + i18n.T(lang, "tmdb.not_configured") + `</p>`))
		return
	}
	query := r.URL.Query().Get("q")
	if strings.TrimSpace(query) == "" {
		w.Write([]byte(""))
		return
	}
	results, _ := h.TMDB.SearchTV(query)
	h.Templates.ExecuteTemplate(w, "tmdb_rematch_results.html", map[string]any{
		"Lang":    lang,
		"Results": results,
		"ShowID":  id,
	})
}

// APIRelinkTMDB re-links a show to a specific TMDB id and refetches its metadata.
// Watch history is preserved (episodes are keyed by show id, not tmdb id).
func (h *Handler) APIRelinkTMDB(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	if h.TMDB == nil || !h.TMDB.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "TMDB not configured")
		return
	}
	id, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.DB.GetShow(id); err != nil {
		writeError(w, http.StatusNotFound, "show not found")
		return
	}
	var req struct {
		TMDBID int `json:"tmdb_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.TMDBID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid tmdb_id")
		return
	}
	// Validate the target exists on TMDB before re-linking.
	if _, err := h.TMDB.GetTVShow(req.TMDBID); err != nil {
		writeError(w, http.StatusNotFound, "tmdb show not found")
		return
	}
	if err := worker.RefreshShowByTMDB(h.DB, h.TMDB, id, req.TMDBID); err != nil {
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	log.Printf("ACTION: user=%d relinked show=%d to tmdb_id=%d", userID, id, req.TMDBID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "tmdb_id": req.TMDBID})
}

func (h *Handler) APIFetchAllTMDB(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	if h.TMDB == nil || !h.TMDB.Enabled() {
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<span class="text-xs text-red-600">` + i18n.T(h.getLang(r, userID), "tmdb.not_configured") + `</span>`))
			return
		}
		writeError(w, http.StatusServiceUnavailable, "TMDB not configured"); return
	}
	shows, _ := h.DB.GetShowsWithoutTMDB()
	log.Printf("TMDB FETCH: starting — %d shows", len(shows))
	fetched := 0
	for i, show := range shows {
		result, err := h.TMDB.FindTVByName(show.Name)
		if err != nil { log.Printf("TMDB [%d/%d] ✗ %q: %v", i+1, len(shows), show.Name, err); continue }
		genres := extractGenreNames(result.Genres)
		h.DB.UpdateShowTMDB(show.ID, result.ID, tmdb.PosterURL(result.PosterPath, "w342"), tmdb.BackdropURL(result.BackdropPath, "w780"), result.Overview, genres, result.Status, len(result.Seasons))
		if resultEN, err := h.TMDB.GetTVShowLang(result.ID, "en-US"); err == nil {
			h.DB.UpdateShowTMDBEN(show.ID, resultEN.Overview, extractGenreNames(resultEN.Genres))
			h.DB.UpdateShowTMDBNames(show.ID, result.Name, resultEN.Name)
		}
		// Cache season episode counts
		for _, s := range result.Seasons {
			if s.SeasonNumber > 0 {
				h.DB.UpsertSeasonEpisodes(show.ID, s.SeasonNumber, s.EpisodeCount)
			}
		}
		fetched++
		log.Printf("TMDB [%d/%d] ✓ %q → tmdb_id=%d", i+1, len(shows), show.Name, result.ID)
	}
	movies, _ := h.DB.GetMoviesWithoutTMDB()
	log.Printf("TMDB FETCH: shows done (%d/%d). Starting %d movies...", fetched, len(shows), len(movies))
	moviesFetched := 0
	for i, movie := range movies {
		results, err := h.TMDB.SearchMovie(movie.Name)
		if err != nil || len(results) == 0 { log.Printf("TMDB [%d/%d] ✗ movie %q", i+1, len(movies), movie.Name); continue }
		detail, err := h.TMDB.GetMovie(results[0].ID)
		if err != nil { continue }
		h.DB.UpdateMovieTMDB(movie.ID, detail.ID, tmdb.PosterURL(detail.PosterPath, "w342"), detail.Overview, extractGenreNames(detail.Genres), detail.Runtime)
		if detailEN, err := h.TMDB.GetMovieLang(detail.ID, "en-US"); err == nil {
			h.DB.UpdateMovieTMDBEN(movie.ID, detailEN.Overview, extractGenreNames(detailEN.Genres))
			h.DB.UpdateMovieTMDBNames(movie.ID, detail.Title, detailEN.Title)
		}
		moviesFetched++
		log.Printf("TMDB [%d/%d] ✓ movie %q → tmdb_id=%d", i+1, len(movies), movie.Name, detail.ID)
	}
	log.Printf("TMDB FETCH: complete — shows %d/%d, movies %d/%d", fetched, len(shows), moviesFetched, len(movies))
	if r.Header.Get("HX-Request") == "true" {
		lang := h.getLang(r, userID)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-xs text-wl-gray">✓ `+i18n.T(lang, "tmdb.fetch_result")+`</span>`, fetched, len(shows), moviesFetched, len(movies))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shows_fetched": fetched, "movies_fetched": moviesFetched})
}

func (h *Handler) APIAddShowFromTMDB(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	if h.TMDB == nil || !h.TMDB.Enabled() { writeError(w, http.StatusServiceUnavailable, "TMDB not configured"); return }
	var req struct { TMDBID int `json:"tmdb_id"` }
	json.NewDecoder(r.Body).Decode(&req)
	show, err := h.TMDB.GetTVShow(req.TMDBID)
	if err != nil { writeError(w, http.StatusNotFound, "not found"); return }
	genres := extractGenreNames(show.Genres)
	id, _ := h.DB.AddShowFromTMDB(show.ID, show.Name, tmdb.PosterURL(show.PosterPath, "w342"), tmdb.BackdropURL(show.BackdropPath, "w780"), show.Overview, genres, show.Status, len(show.Seasons))
	h.DB.FollowShow(userID, id)
	log.Printf("ACTION: user=%d add show %q tmdb_id=%d", userID, show.Name, show.ID)
	if r.Header.Get("HX-Request") == "true" {
		lang := h.getLang(r, userID)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-xs text-wl-gray">✓ "%s" `+i18n.T(lang, "tmdb.added")+`</span>`, html.EscapeString(show.Name))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "id": id})
}

func (h *Handler) APIAddMovieFromTMDB(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	if h.TMDB == nil || !h.TMDB.Enabled() { writeError(w, http.StatusServiceUnavailable, "TMDB not configured"); return }
	var req struct { TMDBID int `json:"tmdb_id"` }
	json.NewDecoder(r.Body).Decode(&req)
	movie, err := h.TMDB.GetMovie(req.TMDBID)
	if err != nil { writeError(w, http.StatusNotFound, "not found"); return }
	genres := extractGenreNames(movie.Genres)
	id, _ := h.DB.AddMovieFromTMDB(movie.ID, movie.Title, tmdb.PosterURL(movie.PosterPath, "w342"), movie.Overview, genres, movie.Runtime)
	h.DB.AddMovieToLibrary(userID, id)
	log.Printf("ACTION: user=%d add movie %q tmdb_id=%d", userID, movie.Title, movie.ID)
	if r.Header.Get("HX-Request") == "true" {
		lang := h.getLang(r, userID)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-xs text-wl-gray">✓ "%s" `+i18n.T(lang, "tmdb.added")+`</span>`, html.EscapeString(movie.Title))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIRefreshUpcoming(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	if h.TMDB == nil || !h.TMDB.Enabled() {
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<span class="text-xs text-red-600">` + i18n.T(h.getLang(r, userID), "tmdb.not_configured") + `</span>`))
			return
		}
		writeError(w, http.StatusServiceUnavailable, "TMDB not configured"); return
	}
	worker.RefreshUpcomingCache(h.DB, h.TMDB)
	if r.Header.Get("HX-Request") == "true" {
		lang := h.getLang(r, userID)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<span class="text-xs text-wl-gray">✓ ` + i18n.T(lang, "tmdb.upcoming_refreshed") + `</span>`))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIRefreshAllTMDB(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	if h.TMDB == nil || !h.TMDB.Enabled() {
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<span class="text-xs text-red-600">` + i18n.T(h.getLang(r, userID), "tmdb.not_configured") + `</span>`))
			return
		}
		writeError(w, http.StatusServiceUnavailable, "TMDB not configured"); return
	}

	shows, _ := h.DB.GetAllShowsWithTMDB()
	log.Printf("TMDB REFRESH: updating %d shows (es+en)...", len(shows))
	updated := 0
	for i, show := range shows {
		// Fetch in Spanish (primary)
		result, err := h.TMDB.GetTVShowLang(show.TMDBID, "es-ES")
		if err != nil { log.Printf("TMDB REFRESH [%d/%d] ✗ %q: %v", i+1, len(shows), show.Name, err); continue }
		genres := extractGenreNames(result.Genres)
		h.DB.UpdateShowTMDB(show.ID, result.ID, tmdb.PosterURL(result.PosterPath, "w342"), tmdb.BackdropURL(result.BackdropPath, "w780"), result.Overview, genres, result.Status, len(result.Seasons))
		// Fetch in English
		resultEN, err := h.TMDB.GetTVShowLang(show.TMDBID, "en-US")
		if err == nil {
			h.DB.UpdateShowTMDBEN(show.ID, resultEN.Overview, extractGenreNames(resultEN.Genres))
			h.DB.UpdateShowTMDBNames(show.ID, result.Name, resultEN.Name)
		}
		// Cache season episode counts
		newSeasonCount := 0
		for _, s := range result.Seasons {
			if s.SeasonNumber > 0 {
				newSeasonCount++
			}
		}
		h.DB.UnarchiveForNewSeason(show.ID, newSeasonCount)
		for _, s := range result.Seasons {
			if s.SeasonNumber > 0 {
				h.DB.UpsertSeasonEpisodes(show.ID, s.SeasonNumber, s.EpisodeCount)
			}
		}
		// Fetch episode details per season
		for _, s := range result.Seasons {
			if s.SeasonNumber == 0 {
				continue
			}
			seasonES, err := h.TMDB.GetSeasonLang(show.TMDBID, s.SeasonNumber, "es-ES")
			if err != nil {
				continue
			}
			seasonEN, _ := h.TMDB.GetSeasonLang(show.TMDBID, s.SeasonNumber, "en-US")
			for _, ep := range seasonES.Episodes {
				d := db.EpisodeDetail{
					ShowID:        show.ID,
					SeasonNumber:  ep.SeasonNumber,
					EpisodeNumber: ep.EpisodeNumber,
					Name:          ep.Name,
					Overview:      ep.Overview,
					AirDate:       ep.AirDate,
					Runtime:       ep.Runtime,
					StillURL:      tmdb.BackdropURL(ep.StillPath, "w300"),
				}
				// Fill English data if available
				if seasonEN != nil {
					for _, epEN := range seasonEN.Episodes {
						if epEN.EpisodeNumber == ep.EpisodeNumber {
							d.NameEN = epEN.Name
							d.OverviewEN = epEN.Overview
							break
						}
					}
				}
				h.DB.UpsertEpisodeDetail(d)
			}
		}
		updated++
	}

	movies, _ := h.DB.GetAllMoviesWithTMDB()
	log.Printf("TMDB REFRESH: updating %d movies (es+en)...", len(movies))
	moviesUpdated := 0
	for i, movie := range movies {
		// Fetch in Spanish (primary)
		detail, err := h.TMDB.GetMovieLang(movie.TMDBID, "es-ES")
		if err != nil { log.Printf("TMDB REFRESH [%d/%d] ✗ movie %q: %v", i+1, len(movies), movie.Name, err); continue }
		genres := extractGenreNames(detail.Genres)
		h.DB.UpdateMovieTMDB(movie.ID, detail.ID, tmdb.PosterURL(detail.PosterPath, "w342"), detail.Overview, genres, detail.Runtime)
		// Fetch in English
		detailEN, err := h.TMDB.GetMovieLang(movie.TMDBID, "en-US")
		if err == nil {
			h.DB.UpdateMovieTMDBEN(movie.ID, detailEN.Overview, extractGenreNames(detailEN.Genres))
			h.DB.UpdateMovieTMDBNames(movie.ID, detail.Title, detailEN.Title)
		}
		moviesUpdated++
	}

	log.Printf("TMDB REFRESH: complete — shows %d/%d, movies %d/%d", updated, len(shows), moviesUpdated, len(movies))
	if r.Header.Get("HX-Request") == "true" {
		lang := h.getLang(r, userID)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-xs text-wl-gray">✓ `+i18n.T(lang, "tmdb.refresh_result")+`</span>`, updated, moviesUpdated)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shows_updated": updated, "movies_updated": moviesUpdated})
}

func extractGenreNames(genres []tmdb.Genre) string {
	names := make([]string, len(genres))
	for i, g := range genres { names[i] = g.Name }
	return strings.Join(names, ", ")
}

// --- Settings ---

func (h *Handler) PageSettings(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	user, _ := h.DB.GetUserByID(userID)
	theme := h.DB.GetUserTheme(userID)
	apiKeys, _ := h.DB.GetUserAPIKeys(userID)
	h.Templates.ExecuteTemplate(w, "settings.html", map[string]any{
		"Lang":    lang,
		"Theme":   theme,
		"IsAdmin": userID == 1,
		"Email":   user.Email,
		"APIKeys": apiKeys,
	})
}

func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	r.ParseForm()
	lang := r.FormValue("lang")
	if lang != "es" && lang != "en" {
		lang = "es"
	}
	h.DB.SetUserLang(userID, lang)
	theme := r.FormValue("theme")
	if theme != "light" && theme != "dark" && theme != "system" {
		theme = "system"
	}
	h.DB.SetUserTheme(userID, theme)
	http.SetCookie(w, &http.Cookie{
		Name:     "theme",
		Value:    theme,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: false, // JS needs to read it
		SameSite: http.SameSiteLaxMode,
	})
	email := strings.TrimSpace(r.FormValue("email"))
	h.DB.UpdateUserEmail(userID, email)
	http.Redirect(w, r, "/settings", http.StatusFound)
}

// requireAdmin ensures the request comes from the admin user (id 1). It writes
// the appropriate response (403 for API/HTMX, redirect otherwise) and returns 0
// when the caller is not the admin.
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) int64 {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return 0
	}
	if userID != 1 {
		if r.Header.Get("HX-Request") == "true" || strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusForbidden, "forbidden")
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
		}
		return 0
	}
	return userID
}

// adminData builds the template data for the admin page.
func (h *Handler) adminData(r *http.Request, userID int64) map[string]any {
	lang := h.getLang(r, userID)
	data := map[string]any{
		"Lang":        lang,
		"WatchLogURL": h.DB.GetSetting("watchlog_url"),
		// Auth options
		"RegistrationEnabled": h.DB.GetSetting("auth_registration") != "disabled",
		"PasswordEnabled":     h.DB.GetSetting("auth_password") != "disabled",
		"MagicLinkEnabled":    h.DB.GetSetting("auth_magic_link") != "disabled",
		"DefaultLogin":        h.DB.GetSetting("auth_default_login"),
	}
	if data["DefaultLogin"] == "" {
		data["DefaultLogin"] = "magic"
	}
	tmdbKey := h.DB.GetSetting("tmdb_api_key")
	if tmdbKey != "" {
		data["TMDBKeySet"] = true
		data["TMDBKeyHint"] = "••••" + tmdbKey[len(tmdbKey)-4:]
	}
	smtpURL := h.DB.GetSetting("smtp_url")
	if smtpURL != "" {
		if cfg, err := mail.ParseURL(smtpURL); err == nil {
			data["SMTPConfigured"] = true
			data["SMTPDisplay"] = mail.FormatURL(cfg)
		}
	}
	users, _ := h.DB.ListAllUsers()
	data["Users"] = users
	invites, _ := h.DB.ListPendingInvitations()
	data["PendingInvites"] = invites
	return data
}

func (h *Handler) PageAdmin(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAdmin(w, r)
	if userID == 0 {
		return
	}
	h.Templates.ExecuteTemplate(w, "admin.html", h.adminData(r, userID))
}

func (h *Handler) SaveAdmin(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAdmin(w, r)
	if userID == 0 {
		return
	}
	r.ParseForm()

	// TMDB key
	if tmdbKey := r.FormValue("tmdb_key"); tmdbKey != "" {
		h.DB.SetSetting("tmdb_api_key", tmdbKey)
		h.TMDB = tmdb.NewClient(tmdbKey)
	}
	// SMTP URL
	if smtpURL := strings.TrimSpace(r.FormValue("smtp_url")); smtpURL != "" {
		if _, err := mail.ParseURL(smtpURL); err == nil {
			h.DB.SetSetting("smtp_url", smtpURL)
		}
	}
	// Public URL
	if v := strings.TrimSpace(r.FormValue("watchlog_url")); v != "" {
		h.DB.SetSetting("watchlog_url", strings.TrimRight(v, "/"))
	}

	// Auth options
	if r.FormValue("auth_registration") == "on" {
		h.DB.SetSetting("auth_registration", "enabled")
	} else {
		h.DB.SetSetting("auth_registration", "disabled")
	}
	if r.FormValue("auth_password") == "on" {
		h.DB.SetSetting("auth_password", "enabled")
	} else {
		h.DB.SetSetting("auth_password", "disabled")
	}
	if r.FormValue("auth_magic_link") == "on" {
		h.DB.SetSetting("auth_magic_link", "enabled")
	} else {
		h.DB.SetSetting("auth_magic_link", "disabled")
	}
	if defaultLogin := r.FormValue("auth_default_login"); defaultLogin == "password" || defaultLogin == "magic" {
		h.DB.SetSetting("auth_default_login", defaultLogin)
	}

	http.Redirect(w, r, "/admin", http.StatusFound)
}

// --- Admin: user management (web, admin only) ---

// AdminInviteUser creates an email invitation. The invite link is emailed (when
// SMTP is configured) and always shown to the admin so it can be shared manually.
func (h *Handler) AdminInviteUser(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAdmin(w, r)
	if userID == 0 {
		return
	}
	r.ParseForm()
	lang := h.getLang(r, userID)
	email := strings.TrimSpace(r.FormValue("email"))

	render := func(extra map[string]any, status int) {
		data := h.adminData(r, userID)
		for k, v := range extra {
			data[k] = v
		}
		w.WriteHeader(status)
		h.Templates.ExecuteTemplate(w, "admin.html", data)
	}

	// Basic email validation.
	if email == "" || !strings.Contains(email, "@") || strings.Contains(email, " ") {
		render(map[string]any{"UserError": i18n.T(lang, "admin.users_err_email")}, http.StatusBadRequest)
		return
	}
	// Reject if a user already has that email.
	if _, err := h.DB.GetUserByEmail(email); err == nil {
		render(map[string]any{"UserError": i18n.T(lang, "admin.users_err_email_exists")}, http.StatusBadRequest)
		return
	}

	token := auth.GenerateToken()
	expiresAt := time.Now().Add(24 * time.Hour)
	if _, err := h.DB.CreateInvitation(email, token, expiresAt); err != nil {
		render(map[string]any{"UserError": i18n.T(lang, "admin.users_err_email")}, http.StatusInternalServerError)
		return
	}

	inviteURL := h.getWatchLogURL() + "/invite?token=" + token

	// Send the invitation email if SMTP is configured.
	smtpCfg := h.getSMTPConfig()
	if smtpCfg.Configured() {
		body := fmt.Sprintf(`<p>%s</p><p><a href="%s">%s</a></p>`,
			html.EscapeString(i18n.T(lang, "invite.email_body")),
			html.EscapeString(inviteURL),
			html.EscapeString(inviteURL))
		if err := mail.Send(smtpCfg, email, i18n.T(lang, "email.invite_subject"), body); err != nil {
			log.Printf("failed to send invitation email to %s: %v", email, err)
		}
	}
	log.Printf("ACTION: admin=%d invited email=%q", userID, email)
	render(map[string]any{
		"InviteLink": inviteURL,
		"UserOK":     i18n.T(lang, "admin.users_invite_sent"),
	}, http.StatusOK)
}

// AdminRevokeInvite deletes a pending invitation.
func (h *Handler) AdminRevokeInvite(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAdmin(w, r)
	if userID == 0 {
		return
	}
	id, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	h.DB.RevokeInvitation(id)
	log.Printf("ACTION: admin=%d revoked invitation id=%d", userID, id)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

// PageAcceptInvite shows the invitation acceptance form (public).
func (h *Handler) PageAcceptInvite(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	token := r.URL.Query().Get("token")
	email, ok := h.DB.GetInvitation(token)
	if !ok {
		h.Templates.ExecuteTemplate(w, "invite.html", map[string]any{
			"Lang":  lang,
			"Error": i18n.T(lang, "invite.invalid"),
		})
		return
	}
	h.Templates.ExecuteTemplate(w, "invite.html", map[string]any{
		"Lang":  lang,
		"Token": token,
		"Email": email,
	})
}

// HandleAcceptInvite creates the account from an invitation (public). Username is
// required; password is optional (empty means the user signs in via magic link).
func (h *Handler) HandleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	lang := h.getLang(r, 0)
	r.ParseForm()
	token := r.FormValue("token")
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	email, ok := h.DB.GetInvitation(token)
	if !ok {
		h.Templates.ExecuteTemplate(w, "invite.html", map[string]any{
			"Lang":  lang,
			"Error": i18n.T(lang, "invite.invalid"),
		})
		return
	}

	renderErr := func(key string) {
		h.Templates.ExecuteTemplate(w, "invite.html", map[string]any{
			"Lang":  lang,
			"Token": token,
			"Email": email,
			"Error": i18n.T(lang, key),
		})
	}

	if username == "" {
		renderErr("invite.err_username")
		return
	}
	// Password is optional; if provided it must meet the minimum length.
	passwordHash := ""
	if password != "" {
		if len(password) < 8 {
			renderErr("invite.err_password")
			return
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			renderErr("invite.err_password")
			return
		}
		passwordHash = hash
	}

	newID, err := h.DB.CreateUser(username, passwordHash)
	if err != nil {
		renderErr("invite.err_username_taken")
		return
	}
	h.DB.UpdateUserEmail(newID, email)
	h.DB.AcceptInvitation(token)

	sessionToken := h.Sessions.Create(newID)
	auth.SetSessionCookie(w, sessionToken)
	log.Printf("ACTION: invitation accepted user=%q id=%d", username, newID)
	http.Redirect(w, r, "/", http.StatusFound)
}

// AdminToggleUserBlock blocks or unblocks a user. The admin (id 1) cannot be blocked.
func (h *Handler) AdminToggleUserBlock(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAdmin(w, r)
	if userID == 0 {
		return
	}
	targetID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if targetID == 1 {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	blocked := h.DB.IsUserBlocked(targetID)
	h.DB.SetUserBlocked(targetID, !blocked)
	log.Printf("ACTION: admin=%d set user=%d blocked=%v", userID, targetID, !blocked)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

// AdminDeleteUser deletes a user and all their data. The admin (id 1) cannot be deleted.
func (h *Handler) AdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAdmin(w, r)
	if userID == 0 {
		return
	}
	targetID, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	if targetID == 1 {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}
	h.DB.DeleteUser(targetID)
	log.Printf("ACTION: admin=%d deleted user=%d", userID, targetID)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

// --- Setup (first run) ---

func (h *Handler) PageSetup(w http.ResponseWriter, r *http.Request) {
	// Block access if setup is fully complete (except not possible to re-enter)
	if h.DB.GetSetting("setup_complete") == "true" {
		step := r.URL.Query().Get("step")
		// Allow step 3 (import) even after setup_complete if admin just finished step 2
		if step != "3" || h.currentUser(r) != 1 {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}
	// Step 1 needs no users, steps 2-3 need admin logged in
	if h.DB.HasUsers() && h.currentUser(r) != 1 {
		// Setup incomplete: admin needs to log in to continue
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	lang := i18n.DetectLang(r.Header.Get("Accept-Language"))
	if uid := h.currentUser(r); uid > 0 {
		lang = h.getLang(r, uid)
	}
	step := 1
	if s := r.URL.Query().Get("step"); s == "2" {
		step = 2
	} else if s == "3" {
		step = 3
	}
	// If user already exists, skip step 1
	if step == 1 && h.DB.HasUsers() {
		step = 2
	}
	// Steps 2 and 3 require the user to be created already
	if step > 1 && !h.DB.HasUsers() {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Step": step})
}

func (h *Handler) HandleSetup(w http.ResponseWriter, r *http.Request) {
	if h.DB.HasUsers() && h.currentUser(r) != 1 {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	lang := i18n.DetectLang(r.Header.Get("Accept-Language"))
	step := r.FormValue("step")

	switch step {
	case "1":
		// Create admin account
		username := strings.TrimSpace(r.FormValue("username"))
		email := strings.TrimSpace(r.FormValue("email"))
		password := r.FormValue("password")
		passwordConfirm := r.FormValue("password_confirm")

		if username == "" {
			h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Step": 1, "Error": i18n.T(lang, "setup.error.username_required"), "Email": email})
			return
		}
		if email == "" {
			h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Step": 1, "Error": i18n.T(lang, "setup.error.email_required"), "Username": username})
			return
		}
		if len(password) < 8 {
			h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Step": 1, "Error": i18n.T(lang, "setup.error.password_min"), "Username": username, "Email": email})
			return
		}
		if password != passwordConfirm {
			h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Step": 1, "Error": i18n.T(lang, "setup.error.password_mismatch"), "Username": username, "Email": email})
			return
		}

		hash, _ := auth.HashPassword(password)
		userID, err := h.DB.CreateUser(username, hash)
		if err != nil {
			h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Step": 1, "Error": i18n.T(lang, "setup.error.create_failed")})
			return
		}
		h.DB.UpdateUserEmail(userID, email)

		// Create session immediately
		sessionToken := h.Sessions.Create(userID)
		auth.SetSessionCookie(w, sessionToken)
		log.Printf("ACTION: setup admin user=%q id=%d email=%q", username, userID, email)

		// Go to step 2
		http.Redirect(w, r, "/setup?step=2", http.StatusFound)

	case "2":
		// Server configuration
		if tmdbKey := strings.TrimSpace(r.FormValue("tmdb_key")); tmdbKey != "" {
			h.DB.SetSetting("tmdb_api_key", tmdbKey)
			h.TMDB = tmdb.NewClient(tmdbKey)
			log.Printf("ACTION: setup TMDB key configured")
		}
		if smtpURL := strings.TrimSpace(r.FormValue("smtp_url")); smtpURL != "" {
			if _, err := mail.ParseURL(smtpURL); err == nil {
				h.DB.SetSetting("smtp_url", smtpURL)
				log.Printf("ACTION: setup SMTP configured")
			}
		}
		if v := strings.TrimSpace(r.FormValue("watchlog_url")); v != "" {
			h.DB.SetSetting("watchlog_url", strings.TrimRight(v, "/"))
		}
		// Auth options
		if r.FormValue("auth_registration") == "on" {
			h.DB.SetSetting("auth_registration", "enabled")
		} else {
			h.DB.SetSetting("auth_registration", "disabled")
		}
		if defaultLogin := r.FormValue("auth_default_login"); defaultLogin == "password" || defaultLogin == "magic" {
			h.DB.SetSetting("auth_default_login", defaultLogin)
		}

		log.Printf("ACTION: setup server configuration saved")
		// Mark setup as complete
		h.DB.SetSetting("setup_complete", "true")
		// Go to step 3
		http.Redirect(w, r, "/setup?step=3", http.StatusFound)

	default:
		http.Redirect(w, r, "/setup", http.StatusFound)
	}
}

// --- Import ---

func (h *Handler) PageImport(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	h.Templates.ExecuteTemplate(w, "import.html", map[string]any{"Lang": lang})
}

func (h *Handler) HandleImport(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }

	// Parse multipart form (max 500MB)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		log.Printf("import: parse form error: %v", err)
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("zipfile")
	if err != nil {
		log.Printf("import: form file error: %v", err)
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()
	log.Printf("import: received file %q (%d bytes)", header.Filename, header.Size)

	// Save to temp file
	tmpBase := filepath.Join(h.DataDir, "tmp")
	os.MkdirAll(tmpBase, 0755)
	tmpFile, err := os.CreateTemp(tmpBase, "watchlog-import-*.zip")
	if err != nil {
		log.Printf("import: create temp file error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())
	io.Copy(tmpFile, file)
	tmpFile.Close()
	log.Printf("import: saved to %s", tmpFile.Name())

	// Extract zip to temp dir
	tmpDir, err := os.MkdirTemp(tmpBase, "watchlog-import-data-")
	if err != nil {
		log.Printf("import: create temp dir error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := extractZip(tmpFile.Name(), tmpDir); err != nil {
		log.Printf("import: extract zip error: %v", err)
		http.Error(w, "Failed to extract zip: "+err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("import: extracted to %s", tmpDir)

	// SSE streaming response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	sendSSE := func(msg string) {
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	}

	lang := h.getLang(r, userID)
	sendSSE(i18n.T(lang, "import.starting"))

	imp := importer.New(h.DB, tmpDir, userID)
	imp.LogFunc = sendSSE

	if err := imp.ImportAll(); err != nil {
		sendSSE("ERROR: " + err.Error())
	} else {
		sendSSE(i18n.T(lang, "import.complete"))
	}

	// Fetch TMDB metadata
	if h.TMDB != nil && h.TMDB.Enabled() {
		sendSSE("═══════════════════════════════════════")
		sendSSE("  " + i18n.T(lang, "import.fetching_tmdb"))
		sendSSE("═══════════════════════════════════════")

		shows, _ := h.DB.GetShowsWithoutTMDB()
		if len(shows) > 0 {
			sendSSE(fmt.Sprintf("  %d shows to enrich...", len(shows)))
			fetched := 0
			for i, show := range shows {
				result, err := h.TMDB.FindTVByName(show.Name)
				if err != nil {
					sendSSE(fmt.Sprintf("  [%d/%d] ✗ %q: %v", i+1, len(shows), show.Name, err))
					continue
				}
				genres := extractGenreNames(result.Genres)
				h.DB.UpdateShowTMDB(show.ID, result.ID, tmdb.PosterURL(result.PosterPath, "w342"), tmdb.BackdropURL(result.BackdropPath, "w780"), result.Overview, genres, result.Status, len(result.Seasons))
				// Fetch English version
				if resultEN, err := h.TMDB.GetTVShowLang(result.ID, "en-US"); err == nil {
					h.DB.UpdateShowTMDBEN(show.ID, resultEN.Overview, extractGenreNames(resultEN.Genres))
					h.DB.UpdateShowTMDBNames(show.ID, result.Name, resultEN.Name)
				}
				fetched++
				sendSSE(fmt.Sprintf("  [%d/%d] ✓ %q", i+1, len(shows), show.Name))
			}
			sendSSE(fmt.Sprintf("  Shows: %d/%d enriched", fetched, len(shows)))
		}

		movies, _ := h.DB.GetMoviesWithoutTMDB()
		if len(movies) > 0 {
			sendSSE(fmt.Sprintf("  %d movies to enrich...", len(movies)))
			fetched := 0
			for i, movie := range movies {
				results, err := h.TMDB.SearchMovie(movie.Name)
				if err != nil || len(results) == 0 {
					sendSSE(fmt.Sprintf("  [%d/%d] ✗ %q", i+1, len(movies), movie.Name))
					continue
				}
				detail, err := h.TMDB.GetMovie(results[0].ID)
				if err != nil {
					continue
				}
				h.DB.UpdateMovieTMDB(movie.ID, detail.ID, tmdb.PosterURL(detail.PosterPath, "w342"), detail.Overview, extractGenreNames(detail.Genres), detail.Runtime)
				// Fetch English version
				if detailEN, err := h.TMDB.GetMovieLang(detail.ID, "en-US"); err == nil {
					h.DB.UpdateMovieTMDBEN(movie.ID, detailEN.Overview, extractGenreNames(detailEN.Genres))
					h.DB.UpdateMovieTMDBNames(movie.ID, detail.Title, detailEN.Title)
				}
				fetched++
				sendSSE(fmt.Sprintf("  [%d/%d] ✓ %q", i+1, len(movies), movie.Name))
			}
			sendSSE(fmt.Sprintf("  Movies: %d/%d enriched", fetched, len(movies)))
		}

		sendSSE(i18n.T(lang, "import.tmdb_complete"))
	}

	// Refresh upcoming episodes cache after import
	if h.TMDB != nil && h.TMDB.Enabled() {
		sendSSE(i18n.T(lang, "import.refreshing_upcoming"))
		worker.RefreshUpcomingCache(h.DB, h.TMDB)
		sendSSE(i18n.T(lang, "import.upcoming_complete"))
	}

	sendSSE("[DONE]")
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	const (
		maxFileSize  = 100 << 20 // 100MB per file
		maxTotalSize = 2 << 30   // 2GB total uncompressed (zip-bomb guard)
		maxEntries   = 5000      // cap number of extracted files
	)

	var totalWritten int64
	extracted := 0

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Flatten: extract all files to destDir regardless of zip structure
		name := filepath.Base(f.Name)
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		if extracted >= maxEntries {
			return fmt.Errorf("zip has too many entries (max %d)", maxEntries)
		}
		destPath := filepath.Join(destDir, name)

		outFile, err := os.Create(destPath)
		if err != nil {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			continue
		}
		// Cap per-file, and stop if we would exceed the total budget.
		remaining := maxTotalSize - totalWritten
		if remaining <= 0 {
			rc.Close()
			outFile.Close()
			return fmt.Errorf("zip exceeds maximum total size (%d bytes)", maxTotalSize)
		}
		limit := int64(maxFileSize)
		if remaining < limit {
			limit = remaining
		}
		n, _ := io.Copy(outFile, io.LimitReader(rc, limit))
		rc.Close()
		outFile.Close()
		totalWritten += n
		extracted++
		if totalWritten >= maxTotalSize {
			return fmt.Errorf("zip exceeds maximum total size (%d bytes)", maxTotalSize)
		}
	}
	return nil
}

// --- API Keys ---

func (h *Handler) APICreateKey(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("key_name"))
	if name == "" {
		name = "default"
	}
	scopes := strings.Join(r.Form["scopes"], ",")
	if scopes == "" {
		scopes = "read"
	}

	// Generate random API key
	keyBytes := make([]byte, 32)
	crypto_rand.Read(keyBytes)
	key := fmt.Sprintf("wl_%x", keyBytes)

	// Store only the SHA-256 hash of the key; the raw key is shown once below.
	h.DB.CreateAPIKey(userID, key, name, scopes)

	// Return the key once (won't be shown again)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<div class="border border-wl-border p-4 my-4"><p class="text-xs uppercase tracking-widest text-wl-gray mb-2">API Key (copy now, won't be shown again):</p><div class="flex items-center gap-2"><code class="text-sm break-all select-all flex-1" id="api-key-value">%s</code><button onclick="navigator.clipboard.writeText(document.getElementById('api-key-value').textContent).then(()=>{this.textContent='✓'});setTimeout(()=>{this.textContent='Copy'},1500)" class="px-3 py-1 text-xs uppercase tracking-widest border border-wl-border hover:bg-black hover:text-white transition-colors shrink-0">Copy</button></div></div>`, html.EscapeString(key))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key})
}

func (h *Handler) APIDeleteKey(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 {
		return
	}
	id, ok := h.parsePathID(w, r, "id")
	if !ok {
		return
	}
	h.DB.DeleteAPIKey(userID, id)
	if r.Header.Get("HX-Request") == "true" {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
