package main

import (
	"context"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/handlers"
	"github.com/mdaguete/watchlog/internal/i18n"
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

// setupMiddleware redirects to /setup if no users exist yet.
func setupMiddleware(next http.Handler, database *db.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !database.HasUsers() && r.URL.Path != "/setup" && !strings.HasPrefix(r.URL.Path, "/static/") {
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
	dbPath := flag.String("db", "./watchlog.db", "Path to SQLite database")
	flag.Parse()

	// Load .env if present
	godotenv.Load()

	log.Printf("WatchLog starting on %s", *addr)

	database, err := db.New(*dbPath)
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

	// Start background worker for upcoming episodes cache
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.StartUpcomingRefresher(ctx, database, tmdbClient)

	// Parse templates
	funcMap := template.FuncMap{
		"T": i18n.T,
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
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("web/templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	h := handlers.New(database, tmpl, tmdbClient, auth.NewSessionStore())

	mux := http.NewServeMux()

	// Setup (first run)
	mux.HandleFunc("GET /setup", h.PageSetup)
	mux.HandleFunc("POST /setup", h.HandleSetup)

	// Import
	mux.HandleFunc("GET /import", h.PageImport)
	mux.HandleFunc("POST /import", h.HandleImport)

	// Auth
	mux.HandleFunc("GET /login", h.PageLogin)
	mux.HandleFunc("POST /login", h.HandleLogin)
	mux.HandleFunc("GET /register", h.PageRegister)
	mux.HandleFunc("POST /register", h.HandleRegister)
	mux.HandleFunc("POST /logout", h.HandleLogout)

	// Web pages
	mux.HandleFunc("GET /", h.PageDashboard)
	mux.HandleFunc("GET /shows", h.PageShows)
	mux.HandleFunc("GET /shows/{id}", h.PageShow)
	mux.HandleFunc("GET /movies", h.PageMovies)
	mux.HandleFunc("GET /lists", h.PageLists)
	mux.HandleFunc("GET /lists/{id}", h.PageList)
	mux.HandleFunc("GET /stats", h.PageStats)
	mux.HandleFunc("GET /search", h.PageSearch)
	mux.HandleFunc("GET /search/results", h.SearchResults)
	mux.HandleFunc("GET /add", h.PageAddShow)
	mux.HandleFunc("GET /add/search", h.SearchTMDB)
	mux.HandleFunc("GET /upcoming", h.PageUpcoming)
	mux.HandleFunc("GET /settings", h.PageSettings)
	mux.HandleFunc("POST /settings", h.SaveSettings)

	// API: Shows
	mux.HandleFunc("GET /api/shows", h.APIGetShows)
	mux.HandleFunc("GET /api/shows/{id}", h.APIGetShow)
	mux.HandleFunc("POST /api/shows/{id}/follow", h.APIToggleFollow)
	mux.HandleFunc("POST /api/shows/{id}/favorite", h.APIToggleFavorite)
	mux.HandleFunc("POST /api/shows/{id}/archive", h.APIToggleArchive)

	// API: Episodes
	mux.HandleFunc("GET /api/shows/{id}/episodes", h.APIGetEpisodes)
	mux.HandleFunc("POST /api/shows/{id}/episodes/watched", h.APIMarkEpisodeWatched)
	mux.HandleFunc("DELETE /api/shows/{id}/episodes/watched", h.APIUnmarkEpisodeWatched)
	mux.HandleFunc("POST /api/shows/{id}/season/watched", h.APIMarkSeasonWatched)
	mux.HandleFunc("DELETE /api/shows/{id}/season/watched", h.APIUnmarkSeasonWatched)

	// API: Movies
	mux.HandleFunc("GET /api/movies", h.APIGetMovies)

	// API: Lists
	mux.HandleFunc("GET /api/lists", h.APIGetLists)
	mux.HandleFunc("GET /api/lists/{id}", h.APIGetList)
	mux.HandleFunc("POST /api/lists", h.APICreateList)
	mux.HandleFunc("PUT /api/lists/{id}", h.APIUpdateList)
	mux.HandleFunc("DELETE /api/lists/{id}", h.APIDeleteList)
	mux.HandleFunc("POST /api/lists/{id}/items", h.APIAddToList)
	mux.HandleFunc("DELETE /api/lists/{id}/items/{itemId}", h.APIRemoveFromList)

	// API: Stats
	mux.HandleFunc("GET /api/stats", h.APIGetStats)
	mux.HandleFunc("GET /api/stats/history", h.APIGetWatchStats)

	// API: Search
	mux.HandleFunc("GET /api/search", h.APISearch)

	// API: TMDB
	mux.HandleFunc("POST /api/shows/{id}/fetch-tmdb", h.APIFetchTMDB)
	mux.HandleFunc("POST /api/tmdb/fetch-all", h.APIFetchAllTMDB)
	mux.HandleFunc("POST /api/tmdb/add-show", h.APIAddShowFromTMDB)
	mux.HandleFunc("POST /api/tmdb/add-movie", h.APIAddMovieFromTMDB)
	mux.HandleFunc("POST /api/tmdb/refresh-upcoming", h.APIRefreshUpcoming)
	mux.HandleFunc("POST /api/tmdb/refresh-all", h.APIRefreshAllTMDB)

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

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
