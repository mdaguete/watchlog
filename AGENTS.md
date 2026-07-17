# AGENTS.md

Project context for AI agents and developers.

## Overview

WatchLog is a self-hosted replacement for TVTime. Multi-user TV show and movie tracking app. Data imported from TVTime CSV export, enriched with TMDB metadata.

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

- **Go monolith**: single binary (HTTP server + API + frontend), stdlib `net/http` (Go 1.22+ mux)
- **SQLite**: WAL mode, `MaxOpenConns(1)`, pure Go driver (`modernc.org/sqlite`, no CGO)
- **Frontend**: HTMX + Tailwind CSS (CDN), Go templates, no JS frameworks
- **Templates**: `{{template "head" .}}` / `{{template "foot" .}}` (NOT layout/content pattern)

## Conventions

- **Module**: `github.com/mdaguete/watchlog`
- **Errors**: `log.Printf`, no panic
- **API**: JSON for `/api/*`, HTML for web pages; handlers detect `HX-Request` for fragments
- **JS fetch**: episode mark/unmark uses `fetch()` with JSON body (not htmx.ajax)
- **Logs**: middleware logs (method, path, status, duration), actions prefixed `ACTION:`
- **Tests**: stdlib `testing`, temp SQLite files, no external test frameworks
- **Input validation**: `parsePathID` helper on all path IDs (returns 400)
- **Authorization**: `requireAuth` on every handler, ownership checks on list/item operations

## Key Files

| File | Responsibility |
|------|---------------|
| `cmd/server/main.go` | Entrypoint, routes, middleware, template loading |
| `cmd/watchdog/main.go` | Admin CLI (user/config/db management) |
| `internal/db/db.go` | Schema, migrations, all queries |
| `internal/db/migrations.go` | Database migrations (v1–v20) |
| `internal/models/models.go` | Domain structs |
| `internal/handlers/handlers.go` | HTTP handlers (API + pages) |
| `internal/tmdb/client.go` | TMDB API v3 client |
| `internal/auth/auth.go` | Sessions, password hashing |
| `internal/worker/upcoming.go` | Background worker (upcoming episodes) |
| `internal/mcp/server.go` | MCP server + auth |
| `web/templates/layout.html` | `head` and `foot` partials |
| `Dockerfile` | Multi-stage distroless container |
| `.goreleaser.yaml` | Release builds (linux/darwin × amd64/arm64) |
| `Makefile` | Dev commands, build, release |

## Useful Commands

```bash
nix develop                              # Dev shell
make run                                 # Build and start server
go test ./internal/...                   # Run tests
make snapshot                            # GoReleaser local snapshot
docker build -t watchlog .               # Build image
./bin/watchdog --datadir /data users     # Admin CLI
```

## Deployment

```bash
# Docker (recommended)
docker run -d -p 8080:8080 -v watchlog-data:/data -e TMDB_API_KEY=key watchlog

# Binary — expects web/ directory in working directory
./bin/server
```

## Detailed Documentation

| Document | Topics |
|----------|--------|
| [docs/database.md](docs/database.md) | Schema, migrations, deadlock rules, backup, post-migration refresh, CSV importer |
| [docs/frontend.md](docs/frontend.md) | Templates, HTMX, dark mode, PWA, i18n, design decisions, pages |
| [docs/tmdb.md](docs/tmdb.md) | API client, fetch/refresh operations, episode details, search, worker |
| [docs/MCP.md](docs/MCP.md) | MCP server, tools, API key auth, agent configuration |
| [docs/CLI.md](docs/CLI.md) | watchdog admin CLI documentation |

## What It Does NOT Have

- Push notifications for new episodes
- CSRF tokens (mitigated by SameSite=Lax cookie)
