package main

import (
	"context"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/cache"
	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/handlers"
	"github.com/mdaguete/watchlog/internal/i18n"
	mcpserver "github.com/mdaguete/watchlog/internal/mcp"
	"github.com/mdaguete/watchlog/internal/tmdb"
	"github.com/mdaguete/watchlog/internal/worker"
)

// loggingMiddleware logs every HTTP request with method, path, status and duration.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(lw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, lw.status, time.Since(start).Round(time.Millisecond))
	})
}

// securityHeadersMiddleware adds standard security headers to all responses.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// setupMiddleware redirects to /setup if setup is not complete.
func setupMiddleware(next http.Handler, database *db.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setupDone := database.HasUsers() && database.GetSetting("setup_complete") == "true"
		if !setupDone && r.URL.Path != "/setup" && r.URL.Path != "/import" && r.URL.Path != "/login" && r.URL.Path != "/logout" && !strings.HasPrefix(r.URL.Path, "/static/") {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.status = code
	lw.ResponseWriter.WriteHeader(code)
}

func (lw *loggingResponseWriter) Flush() {
	if f, ok := lw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dataDir := flag.String("datadir", ".", "Data directory (database, cache, tmp)")
	flag.Parse()

	// Load .env if present
	godotenv.Load()

	log.Printf("WatchLog starting on %s", *addr)
	log.Printf("Data directory: %s", *dataDir)

	// Ensure data directory exists
	os.MkdirAll(*dataDir, 0755)

	dbFile := filepath.Join(*dataDir, "watchlog.db")
	database, err := db.New(dbFile)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// TMDB client: env var takes priority, then DB setting
	tmdbKey := os.Getenv("TMDB_API_KEY")
	if tmdbKey != "" {
		// Persist to DB (or update if different)
		stored := database.GetSetting("tmdb_api_key")
		if stored != tmdbKey {
			database.SetSetting("tmdb_api_key", tmdbKey)
			if stored == "" {
				log.Println("TMDB: API key saved to database")
			} else {
				log.Println("TMDB: API key updated in database")
			}
		}
	} else {
		// Try to read from DB
		tmdbKey = database.GetSetting("tmdb_api_key")
		if tmdbKey != "" {
			log.Println("TMDB: using API key from database")
		}
	}
	tmdbClient := tmdb.NewClient(tmdbKey)
	if tmdbKey != "" {
		log.Println("TMDB integration enabled")
	} else {
		log.Println("TMDB integration disabled (set TMDB_API_KEY to enable)")
	}

	// SMTP: env var SMTP_URL takes priority, then DB setting
	// Format: smtps://user:password@host:port/from@example.com
	if smtpURL := os.Getenv("SMTP_URL"); smtpURL != "" {
		stored := database.GetSetting("smtp_url")
		if stored != smtpURL {
			database.SetSetting("smtp_url", smtpURL)
			log.Println("SMTP: URL saved to database")
		}
	}
	if database.GetSetting("smtp_url") != "" {
		log.Println("SMTP: configured")
	}

	// Public URL for magic links
	if watchlogURL := os.Getenv("WATCHLOG_URL"); watchlogURL != "" {
		stored := database.GetSetting("watchlog_url")
		if stored != watchlogURL {
			database.SetSetting("watchlog_url", watchlogURL)
			log.Println("WATCHLOG_URL saved to database")
		}
	}

	// Trust X-Forwarded-For only when explicitly enabled (app behind a trusted
	// reverse proxy). Otherwise the header is ignored to prevent rate-limit
	// evasion via a spoofed X-Forwarded-For.
	if tp := os.Getenv("TRUST_PROXY"); tp != "" {
		if tp == "true" || tp == "1" {
			database.SetSetting("trust_proxy", "true")
			log.Println("TRUST_PROXY enabled: X-Forwarded-For will be trusted")
		} else {
			database.SetSetting("trust_proxy", "false")
		}
	}

	// Start background worker for upcoming episodes cache
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.StartUpcomingRefresher(ctx, database, tmdbClient)

	// Run pending TMDB refresh if a migration flagged it
	if database.TMDBRefreshPending() && tmdbClient.Enabled() {
		go func() {
			log.Println("TMDB: running post-migration refresh...")
			worker.RunTMDBRefresh(database, tmdbClient)
			database.ClearTMDBRefreshPending()
			log.Println("TMDB: post-migration refresh complete")
		}()
	}

	// Image cache directory (relative to DB path)
	cacheDir := filepath.Join(*dataDir, "cache", "images")
	imgCache, err := cache.NewImageCache(cacheDir)
	if err != nil {
		log.Printf("Warning: image cache disabled: %v", err)
	}

	// Parse templates
	funcMap := template.FuncMap{
		"T": i18n.T,
		"dict": func(values ...any) map[string]any {
			m := make(map[string]any, len(values)/2)
			for i := 0; i+1 < len(values); i += 2 {
				key, _ := values[i].(string)
				m[key] = values[i+1]
			}
			return m
		},
		"Loc": func(lang, es, en string) string {
			if lang == "en" && en != "" {
				return en
			}
			if es != "" {
				return es
			}
			return en
		},
		"LocGenres": func(lang, es, en string) string {
			if lang == "en" && en != "" {
				return en
			}
			return i18n.TranslateGenres(lang, es)
		},
		"LocName": func(lang, name, nameES, nameEN string) string {
			if lang == "en" {
				if nameEN != "" { return nameEN }
				return name
			}
			if nameES != "" { return nameES }
			return name
		},
		"ImgURL": func(url string) string {
			if imgCache == nil || url == "" { return url }
			local, err := imgCache.Ensure(url)
			if err != nil { return url }
			return local
		},
		"min": func(a, b int) int {
			if a < b {
				return a
			}
			return b
		},
		"mul": func(a, b int) int {
			return a * b
		},
		"add": func(a, b int) int {
			return a + b
		},
		"mod": func(a, b int) int {
			return a % b
		},
		"dtLocal": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02T15:04")
		},
		"dt": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02 15:04")
		},
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("web/templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	h := handlers.New(database, tmpl, tmdbClient, auth.NewSessionStore(database), imgCache, *dataDir)

	mux := http.NewServeMux()

	// Setup (first run)
	mux.HandleFunc("GET /setup", h.PageSetup)
	mux.HandleFunc("POST /setup", h.HandleSetup)

	// Import
	mux.HandleFunc("GET /import", h.PageImport)
	mux.HandleFunc("POST /import", h.HandleImport)
	mux.HandleFunc("GET /import/history", h.PageHistoryImport)
	mux.HandleFunc("POST /import/history", h.HandleHistoryAnalyze)
	mux.HandleFunc("GET /import/history/{id}", h.PageHistoryBatch)
	mux.HandleFunc("POST /import/history/{id}/change/{cid}/toggle", h.HandleHistoryToggle)
	mux.HandleFunc("POST /import/history/{id}/change/{cid}/date", h.HandleHistoryEditDate)
	mux.HandleFunc("POST /import/history/{id}/apply", h.HandleHistoryApply)
	mux.HandleFunc("POST /import/history/{id}/delete", h.HandleHistoryDelete)
	mux.HandleFunc("GET /import/history/{id}/tmdb", h.HandleHistoryTMDBSearch)
	mux.HandleFunc("POST /import/history/{id}/resolve", h.HandleHistoryResolve)

	// Auth
	mux.HandleFunc("GET /login", h.PageLogin)
	mux.HandleFunc("POST /login", h.HandleLogin)
	mux.HandleFunc("GET /register", h.PageRegister)
	mux.HandleFunc("POST /register", h.HandleRegister)
	mux.HandleFunc("POST /logout", h.HandleLogout)
	mux.HandleFunc("GET /forgot-password", h.PageForgotPassword)
	mux.HandleFunc("POST /forgot-password", h.HandleForgotPassword)
	mux.HandleFunc("GET /reset-password", h.PageResetPassword)
	mux.HandleFunc("POST /reset-password", h.HandleResetPassword)
	mux.HandleFunc("GET /magic-login", h.PageMagicLogin)
	mux.HandleFunc("POST /magic-login", h.HandleMagicLogin)
	mux.HandleFunc("GET /auth/magic", h.HandleMagicAuth)
	mux.HandleFunc("GET /invite", h.PageAcceptInvite)
	mux.HandleFunc("POST /invite", h.HandleAcceptInvite)

	// Web pages
	mux.HandleFunc("GET /", h.PageDashboard)
	mux.HandleFunc("GET /api/continue-watching", h.APIContinueWatching)
	mux.HandleFunc("GET /shows", h.PageShows)
	mux.HandleFunc("GET /shows/{id}", h.PageShow)
	mux.HandleFunc("GET /movies", h.PageMovies)
	mux.HandleFunc("GET /movies/{id}", h.PageMovie)
	mux.HandleFunc("GET /stats", h.PageStats)
	mux.HandleFunc("GET /timeline", h.PageTimeline)
	mux.HandleFunc("GET /calendar", h.PageCalendar)
	mux.HandleFunc("GET /api/timeline", h.APITimelineItems)
	mux.HandleFunc("GET /search", h.PageSearch)
	mux.HandleFunc("GET /search/results", h.SearchResults)
	mux.HandleFunc("GET /add", h.PageAddShow)
	mux.HandleFunc("GET /add/search", h.SearchTMDB)
	mux.HandleFunc("GET /upcoming", h.PageUpcoming)
	mux.HandleFunc("GET /settings", h.PageSettings)
	mux.HandleFunc("POST /settings", h.SaveSettings)
	mux.HandleFunc("GET /admin", h.PageAdmin)
	mux.HandleFunc("POST /admin", h.SaveAdmin)
	mux.HandleFunc("POST /admin/invites", h.AdminInviteUser)
	mux.HandleFunc("POST /admin/invites/{id}/revoke", h.AdminRevokeInvite)
	mux.HandleFunc("POST /admin/users/{id}/block", h.AdminToggleUserBlock)
	mux.HandleFunc("POST /admin/users/{id}/delete", h.AdminDeleteUser)

	// API: Shows
	mux.HandleFunc("GET /api/shows", h.APIGetShows)
	mux.HandleFunc("GET /api/shows/{id}", h.APIGetShow)
	mux.HandleFunc("POST /api/shows/{id}/follow", h.APIToggleFollow)
	mux.HandleFunc("POST /api/shows/{id}/favorite", h.APIToggleFavorite)
	mux.HandleFunc("POST /api/shows/{id}/archive", h.APIToggleArchive)
	mux.HandleFunc("POST /api/shows/{id}/snooze", h.APISnoozeShow)

	// API: Episodes
	mux.HandleFunc("GET /api/shows/{id}/episodes", h.APIGetEpisodes)
	mux.HandleFunc("POST /api/shows/{id}/episodes/watched", h.APIMarkEpisodeWatched)
	mux.HandleFunc("DELETE /api/shows/{id}/episodes/watched", h.APIUnmarkEpisodeWatched)
	mux.HandleFunc("POST /api/shows/{id}/season/watched", h.APIMarkSeasonWatched)
	mux.HandleFunc("DELETE /api/shows/{id}/season/watched", h.APIUnmarkSeasonWatched)
	mux.HandleFunc("POST /api/shows/{id}/episodes/date", h.APISetEpisodeDate)

	// API: Movies
	mux.HandleFunc("GET /api/movies", h.APIGetMovies)
	mux.HandleFunc("POST /api/movies/{id}/watched", h.APIMarkMovieWatched)
	mux.HandleFunc("DELETE /api/movies/{id}/watched", h.APIUnmarkMovieWatched)
	mux.HandleFunc("POST /api/movies/{id}/date", h.APISetMovieDate)

	// API: Stats
	mux.HandleFunc("GET /api/stats", h.APIGetStats)
	mux.HandleFunc("GET /api/stats/history", h.APIGetWatchStats)

	// API: Search
	mux.HandleFunc("GET /api/search", h.APISearch)

	// API: TMDB
	mux.HandleFunc("POST /api/shows/{id}/fetch-tmdb", h.APIFetchTMDB)
	mux.HandleFunc("GET /api/shows/{id}/rematch", h.APIRematchSearch)
	mux.HandleFunc("POST /api/shows/{id}/relink-tmdb", h.APIRelinkTMDB)
	mux.HandleFunc("POST /api/tmdb/fetch-all", h.APIFetchAllTMDB)
	mux.HandleFunc("POST /api/tmdb/add-show", h.APIAddShowFromTMDB)
	mux.HandleFunc("POST /api/tmdb/add-movie", h.APIAddMovieFromTMDB)
	mux.HandleFunc("POST /api/tmdb/refresh-upcoming", h.APIRefreshUpcoming)
	mux.HandleFunc("POST /api/tmdb/refresh-all", h.APIRefreshAllTMDB)

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "web/static/sw.js")
	})
	// Cached images
	mux.Handle("GET /static/cache/images/", http.StripPrefix("/static/cache/images/", http.FileServer(http.Dir(cacheDir))))

	// API Keys
	mux.HandleFunc("POST /api/keys", h.APICreateKey)
	mux.HandleFunc("DELETE /api/keys/{id}", h.APIDeleteKey)

	// MCP endpoint
	mcpSrv := mcpserver.New(database, tmdbClient)
	mux.Handle("POST /mcp", mcpSrv.Handler())
	mux.Handle("GET /mcp", mcpSrv.Handler())

	log.Printf("Listening on http://localhost%s", *addr)

	srv := &http.Server{
		Addr:         *addr,
		Handler:      securityHeadersMiddleware(loggingMiddleware(setupMiddleware(mux, database))),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second, // high for SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel() // stop background worker
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped")
}
