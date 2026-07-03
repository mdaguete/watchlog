package handlers

import (
	"archive/zip"
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
	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/i18n"
	"github.com/mdaguete/watchlog/internal/importer"
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
}

func New(database *db.DB, tmpl *template.Template, tmdbClient *tmdb.Client, sessions *auth.SessionStore) *Handler {
	return &Handler{
		DB:           database,
		Templates:    tmpl,
		TMDB:         tmdbClient,
		Sessions:     sessions,
		LoginLimiter: ratelimit.New(5, 15*time.Minute),
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

// clientIP extracts the client IP from the request, checking X-Forwarded-For first.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
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
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	lang := h.getLang(r, 0)
	h.Templates.ExecuteTemplate(w, "login.html", map[string]any{"Lang": lang})
}

func (h *Handler) PageRegister(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) != 0 {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	lang := h.getLang(r, 0)
	h.Templates.ExecuteTemplate(w, "register.html", map[string]any{"Lang": lang})
}

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !h.LoginLimiter.Allow(ip) {
		lang := h.getLang(r, 0)
		w.WriteHeader(http.StatusTooManyRequests)
		h.Templates.ExecuteTemplate(w, "login.html", map[string]any{"Error": i18n.T(lang, "login.rate_limited"), "Lang": lang})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := h.DB.GetUserByUsername(username)
	if err != nil || !auth.CheckPassword(user.PasswordHash, password) {
		h.LoginLimiter.Record(ip)
		lang := h.getLang(r, 0)
		h.Templates.ExecuteTemplate(w, "login.html", map[string]any{"Error": i18n.T(lang, "login.error"), "Lang": lang})
		return
	}

	h.LoginLimiter.Reset(ip)
	token := h.Sessions.Create(user.ID)
	auth.SetSessionCookie(w, token)
	log.Printf("ACTION: login user=%q", username)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

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

// --- Pages ---

func (h *Handler) PageDashboard(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	stats, _ := h.DB.GetDashboardStats(userID)
	h.Templates.ExecuteTemplate(w, "dashboard.html", map[string]any{
		"Lang":    lang,
		"Stats":   stats,
		"Runtime": importer.FormatRuntime(stats.TotalRuntime),
	})
}

func (h *Handler) PageShows(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	sort := r.URL.Query().Get("sort")
	if sort == "" { sort = "recent" }
	shows, _ := h.DB.GetUserShowsSorted(userID, sort)
	h.Templates.ExecuteTemplate(w, "shows.html", map[string]any{
		"Lang":  lang,
		"Shows": shows,
		"Sort":  sort,
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

	// Build seasons map with total episode count per season
	// Try TMDB first for accurate totals, fall back to watched episode data
	seasons := make(map[int]int)
	if show.TMDBID > 0 && h.TMDB != nil && h.TMDB.Enabled() {
		tvShow, err := h.TMDB.GetTVShow(show.TMDBID)
		if err == nil {
			for _, s := range tvShow.Seasons {
				if s.SeasonNumber > 0 { // skip specials (season 0)
					seasons[s.SeasonNumber] = s.EpisodeCount
				}
			}
		}
	}
	// Fill in from watched data if TMDB didn't provide info
	for _, ep := range episodes {
		if _, ok := seasons[ep.SeasonNumber]; !ok {
			seasons[ep.SeasonNumber] = ep.EpisodeNumber
		} else if ep.EpisodeNumber > seasons[ep.SeasonNumber] {
			seasons[ep.SeasonNumber] = ep.EpisodeNumber
		}
	}

	h.Templates.ExecuteTemplate(w, "show.html", map[string]any{
		"Lang":     h.getLang(r, userID),
		"Show":     show,
		"Episodes": episodes,
		"Progress": progress,
		"Seasons":  seasons,
	})
}

func (h *Handler) PageMovies(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	lang := h.getLang(r, userID)
	sort := r.URL.Query().Get("sort")
	if sort == "" { sort = "recent" }
	movies, _ := h.DB.GetUserMoviesSorted(userID, sort)
	h.Templates.ExecuteTemplate(w, "movies.html", map[string]any{
		"Lang":   lang,
		"Movies": movies,
		"Sort":   sort,
	})
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) APIToggleArchive(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	id, ok := h.parsePathID(w, r, "id")
	if !ok { return }
	h.DB.ToggleUserShowArchive(userID, id)
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
	log.Printf("ACTION: user=%d mark watched show=%d S%02dE%02d", userID, showID, req.Season, req.Episode)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-xs text-wl-gray">✓ S%02dE%02d</span>`, req.Season, req.Episode)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	log.Printf("ACTION: user=%d unmark watched show=%d S%02dE%02d", userID, showID, req.Season, req.Episode)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	log.Printf("ACTION: user=%d mark season show=%d S%02d (%d eps)", userID, showID, req.Season, marked)
	if r.Header.Get("HX-Request") == "true" {
		lang := h.getLang(r, userID)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-xs text-wl-gray">✓ `+i18n.T(lang, "tmdb.season_marked")+`</span>`, req.Season, marked)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "marked": marked})
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
	log.Printf("ACTION: user=%d unmark season show=%d S%02d (%d eps)", userID, showID, req.Season, removed)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "removed": removed})
}

func (h *Handler) APIGetMovies(w http.ResponseWriter, r *http.Request) {
	userID := h.requireAuth(w, r)
	if userID == 0 { return }
	movies, _ := h.DB.GetUserMoviesSorted(userID, "name")
	writeJSON(w, http.StatusOK, movies)
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
	h.DB.RemoveListItem(itemID)
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
	result, err := h.TMDB.FindTVByName(show.Name)
	if err != nil { writeError(w, http.StatusNotFound, "not found on TMDB"); return }
	genres := extractGenreNames(result.Genres)
	posterURL := tmdb.PosterURL(result.PosterPath, "w342")
	backdropURL := tmdb.BackdropURL(result.BackdropPath, "w780")
	h.DB.UpdateShowTMDB(id, result.ID, posterURL, backdropURL, result.Overview, genres, result.Status, len(result.Seasons))
	log.Printf("TMDB: fetched %q tmdb_id=%d", show.Name, result.ID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "tmdb_id": result.ID})
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
	h.DB.MarkMovieWatched(userID, id, time.Now())
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
	data := map[string]any{
		"Lang":    lang,
		"IsAdmin": userID == 1,
	}
	if userID == 1 {
		tmdbKey := h.DB.GetSetting("tmdb_api_key")
		if tmdbKey != "" {
			// Mask the key, show only last 4 chars
			data["TMDBKeySet"] = true
			data["TMDBKeyHint"] = "••••" + tmdbKey[len(tmdbKey)-4:]
		}
	}
	h.Templates.ExecuteTemplate(w, "settings.html", data)
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

	// Only admin (user id 1) can update TMDB key
	if userID == 1 {
		tmdbKey := r.FormValue("tmdb_key")
		if tmdbKey != "" {
			h.DB.SetSetting("tmdb_api_key", tmdbKey)
			// Update the TMDB client in-place
			h.TMDB = tmdb.NewClient(tmdbKey)
		}
	}

	http.Redirect(w, r, "/settings", http.StatusFound)
}

// --- Setup (first run) ---

func (h *Handler) PageSetup(w http.ResponseWriter, r *http.Request) {
	if h.DB.HasUsers() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	lang := i18n.DetectLang(r.Header.Get("Accept-Language"))
	h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang})
}

func (h *Handler) HandleSetup(w http.ResponseWriter, r *http.Request) {
	if h.DB.HasUsers() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	lang := i18n.DetectLang(r.Header.Get("Accept-Language"))
	username := r.FormValue("username")
	password := r.FormValue("password")
	if username == "" {
		h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Error": i18n.T(lang, "setup.error.username_required")})
		return
	}
	if len(password) < 8 {
		h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Error": i18n.T(lang, "setup.error.password_min")})
		return
	}
	hash, _ := auth.HashPassword(password)
	userID, err := h.DB.CreateUser(username, hash)
	if err != nil {
		h.Templates.ExecuteTemplate(w, "setup.html", map[string]any{"Lang": lang, "Error": i18n.T(lang, "setup.error.create_failed")})
		return
	}
	token := h.Sessions.Create(userID)
	auth.SetSessionCookie(w, token)
	log.Printf("ACTION: setup user=%q id=%d", username, userID)
	http.Redirect(w, r, "/import", http.StatusFound)
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
	tmpFile, err := os.CreateTemp("", "watchlog-import-*.zip")
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
	tmpDir, err := os.MkdirTemp("", "watchlog-import-data-")
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

	const maxFileSize = 100 << 20 // 100MB per file

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Flatten: extract all files to destDir regardless of zip structure
		name := filepath.Base(f.Name)
		if name == "" || strings.HasPrefix(name, ".") {
			continue
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
		io.Copy(outFile, io.LimitReader(rc, maxFileSize))
		rc.Close()
		outFile.Close()
	}
	return nil
}
