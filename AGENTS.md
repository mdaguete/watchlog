# AGENTS.md

Project context for AI agents and developers.

## Overview

WatchLog is a self-hosted replacement for the TVTime app (which shut down). It's a multi-user application for personal TV show and movie tracking. Data was originally imported from TVTime's CSV export and enriched with TMDB metadata.

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌──────────┐
│   Browser   │────▶│  Go Server   │────▶│  SQLite  │
│  HTMX+TW   │◀────│  net/http    │◀────│   WAL    │
└─────────────┘     └──────┬───────┘     └──────────┘
                           │
                    ┌──────▼───────┐
                    │   TMDB API   │
                    │   (v3 REST)  │
                    └──────────────┘
```

- **Go monolith** served as a single binary (HTTP server + API + frontend)
- **SQLite** with WAL mode, single `.db` file
- **No frameworks**: stdlib `net/http` with mux (Go 1.22+), no external router
- **No JS frameworks**: HTMX for interactivity, Tailwind CSS via CDN
- **Go templates**: pattern `{{template "head"}}` / `{{template "foot"}}` (do NOT use shared `{{template "content"}}` because Go templates have global namespace)

## Design Decisions

### Database
- SQLite chosen for simplicity (single-file backup, easy deployment)
- WAL mode for concurrent reads during writes
- `MaxOpenConns(1)` because SQLite doesn't support concurrent writes
- **Important**: never hold a `rows` cursor open while calling `Exec` — this deadlocks with `MaxOpenConns(1)`. Always read rows into a slice first, close the cursor, then execute writes.
- Inline migrations in the schema const (CREATE IF NOT EXISTS) + ALTER TABLE for new columns
- Episode UNIQUE constraint is `(user_id, show_id, season_number, episode_number)` — an episode is recorded once per user

### Frontend
- Minimalist editorial design
- White background, serif font for titles, sans-serif for UI
- UI text in uppercase with wide letter-spacing
- No emojis or saturated colors in UI
- Templates use `{{template "head" .}}` at start and `{{template "foot" .}}` at end (NOT layout/content pattern)
- Episode grids use vanilla JS with `fetch()` for mark/unmark — not htmx (htmx.ajax doesn't send JSON body correctly)
- "Mark next" button is fully dynamic (JS), recalculated after every mark/unmark action

### Security
- bcrypt password hashing (DefaultCost)
- Session tokens: 32 bytes from `crypto/rand`, stored in memory with periodic cleanup of expired sessions
- Cookie flags: `HttpOnly`, `Secure`, `SameSite=Lax`
- All path parameters validated with `parsePathID` helper (returns 400 on invalid input)
- All API endpoints require authentication (including `/api/tmdb/fetch-all`)
- List item operations verify ownership (IDOR protection)
- HTML responses with dynamic content use `html.EscapeString` (XSS prevention)
- Zip extraction uses `io.LimitReader` (100MB per file) and `filepath.Base` (path traversal prevention)
- HTTP server configured with Read/Write/Idle timeouts
- Security headers middleware: X-Content-Type-Options, X-Frame-Options, Referrer-Policy, Permissions-Policy
- Graceful shutdown on SIGINT/SIGTERM
- Minimum password length: 8 characters

### i18n
- Supported languages: Spanish (es) and English (en)
- Translation function `T(lang, key)` registered in template FuncMap
- All templates receive `.Lang` in their data map
- Language detection: user DB preference > Accept-Language header
- User can change language in `/settings`
- Translations stored in `internal/i18n/i18n.go` as `map[string]map[string]string`

### TMDB
- API key in `.env` loaded with godotenv
- Metadata cached in SQLite (poster_url, overview, genres, status)
- Show search strips `(YYYY)` suffix from name if no results found
- `fetch-all` processes all shows/movies without tmdb_id, logs individual progress
- Season episode counts fetched from TMDB for show detail page grids
- HTTP client with 10s timeout

### CSV Importer
- TVTime export has ~50 CSV files, only ~9 are used
- `tracking-prod-records-v2.csv` (watch-episode-*): main episode source with dates
- `user_tv_show_data.csv` field `nb_episodes_seen` is the real total episode count per show
- Individual imported episodes are a subset (TVTime only exports recent tracking)
- Movies come from two sources: watch history + ratings
- Watch stats are imported from CSV then recalculated from actual DB data (episodes + movies `watched_at`) to cover periods not in the CSV
- **Note**: `watched_at` dates are stored in Go format (`2006-01-02 15:04:05 +0000 UTC`), not ISO 8601. Use `substr(watched_at, 1, 7)` in SQL instead of `strftime()`

### Episodes & Seasons
- Users can mark/unmark individual episodes (toggle)
- "Mark all" button on a season toggles: marks all if any unwatched, unmarks all if all watched
- Episode count per season comes from TMDB (fallback: max episode number seen)
- Watch stats (`watch_stats` table) are incremented in real-time when marking episodes via web

### Worker
- Background goroutine refreshes upcoming episodes cache daily
- Accepts `context.Context` for graceful cancellation
- Only processes shows with status != "Ended"/"Canceled" and tmdb_id > 0

## Key Files

| File | Responsibility |
|------|---------------|
| `cmd/server/main.go` | Entrypoint, routes, middleware, template loading, FuncMap |
| `cmd/importer/main.go` | CLI to import TVTime data |
| `internal/db/db.go` | Schema, migrations, all queries |
| `internal/models/models.go` | Domain structs |
| `internal/handlers/handlers.go` | HTTP handlers (API JSON + HTML pages) |
| `internal/i18n/i18n.go` | Translation maps and language detection |
| `internal/importer/importer.go` | CSV parsing logic for TVTime export |
| `internal/tmdb/client.go` | HTTP client for TMDB API v3 |
| `internal/auth/auth.go` | Password hashing, sessions, cookies |
| `internal/worker/upcoming.go` | Background worker for upcoming episode cache |
| `web/templates/layout.html` | `head` and `foot` (nav + footer) |
| `web/templates/*.html` | One template per page |
| `web/templates/settings.html` | Language preference page |
| `Dockerfile` | Multi-stage distroless container |
| `.goreleaser.yaml` | Release builds (linux/darwin × amd64/arm64) |
| `Makefile` | Dev commands, build, release |
| `flake.nix` | Nix devShell (go, make, sqlite, goreleaser, gopls) |
| `.env` | TMDB_API_KEY (do not commit) |

## Conventions

- **Standard Go**: no frameworks, no ORMs, no generators
- **Module path**: `github.com/mdaguete/watchlog`
- **Errors**: logging with `log.Printf`, no panic
- **API**: JSON for `/api/*` endpoints, HTML for web pages
- **HTMX**: handlers detect `HX-Request` header to return HTML fragment vs JSON
- **JS fetch**: episode mark/unmark uses `fetch()` with JSON body (not htmx.ajax)
- **Logs**: HTTP requests logged via middleware (method, path, status, duration), important actions with `ACTION:` prefix
- **CGO**: disabled (CGO_ENABLED=0), SQLite via modernc.org/sqlite (pure Go)
- **Tests**: stdlib `testing` package, no external test frameworks. Tests use temp SQLite files.
- **Input validation**: all path IDs validated via `parsePathID` helper, returns 400 on parse error
- **Authorization**: every handler calls `requireAuth`, list/item operations verify ownership

## Useful Commands

```bash
# Development
nix develop                              # Shell with go, sqlite, goreleaser
make run                                 # Build and start server
go run ./cmd/importer/ -data ./data      # Import data

# Testing
go test ./internal/...                   # Run all tests
go test -cover ./internal/...            # Run with coverage

# Build
make                                     # Build server + importer
make snapshot                            # GoReleaser local snapshot
make release                             # GoReleaser tagged release

# Docker
docker build -t watchlog .               # Build image
docker run -d -p 8080:8080 \
  -v watchlog-data:/data \
  -e TMDB_API_KEY=key watchlog           # Run container

# TMDB
curl -X POST http://localhost:8080/api/tmdb/fetch-all  # Enrich metadata
```

## Deployment

### Docker (recommended)

```bash
docker build -t watchlog .
docker run -d -p 8080:8080 -v watchlog-data:/data -e TMDB_API_KEY=key watchlog
```

Database created automatically at `/data/watchlog.db`. Container runs as nonroot user on distroless base.

### Binary

Download from GoReleaser releases or build with `make`. Run the `server` binary — it expects `web/` directory in the working directory for templates and static files.

## What It Does NOT Have (potential additions)

- Push notifications for new episodes
- Dark mode
- PWA / improved mobile responsive
- Rate limiting on login
- CSRF tokens (mitigated by SameSite=Lax cookie)
