# WatchLog

Personal TV show and movie tracking app. Self-hosted replacement for TVTime, built with Go and SQLite.

## Features

- **Import TVTime data** — migrate your full history from the CSV export
- **TV show tracking** — mark episodes/seasons as watched
- **Continue watching** — dashboard shows next unwatched episode for recent shows
- **Auto-archive** — shows archived automatically when fully watched and ended
- **Movies** — your watched movie collection with stats (count + runtime)
- **TMDB integration** — posters, synopses, genres, airing status and upcoming episodes
- **Episode details** — names, synopses, and still images from TMDB
- **Add new content** — search TMDB and add shows/movies to your library
- **Upcoming episodes** — see what's airing soon from shows you follow
- **Custom lists** — create and manage themed lists
- **Statistics** — monthly activity, total watch time, calendar heatmap
- **Instant search** — search your library with HTMX
- **i18n** — Spanish and English, auto-detected or user-selected
- **Multi-user** — each user has their own library and preferences
- **Email notifications** — styled HTML emails for magic links and password reset
- **PWA** — installable on iOS and Android, works like a native app

## Requirements

- Go 1.22+ (uses new mux routing syntax)
- [Optional] Nix (for development shell)
- [Optional] Docker (for containerized deployment)
- [Optional] GoReleaser (for creating releases)

## Quick Start

### With Docker (recommended for deployment)

```bash
docker build -t watchlog .

docker run -d \
  -p 8080:8080 \
  -v watchlog-data:/data \
  -e TMDB_API_KEY=your_api_key_here \
  watchlog
```

The database is automatically created at `/data/watchlog.db` on first run.

### With Make (development)

```bash
make              # Build bin/server
make run          # Build and start server
make clean        # Clean binaries and DB
```

### With Nix (development shell)

```bash
nix develop       # Shell with go, make, sqlite, goreleaser, gopls
```

### Manual

```bash
go build -o bin/server ./cmd/server/
./bin/server
```

## Configuration

Create a `.env` file in the project root:

```env
TMDB_API_KEY=your_api_key_here
SMTP_URL=smtps://user:password@smtp.example.com:465/noreply@example.com
WATCHLOG_URL=https://watchlog.example.com
```

The server automatically loads `.env` on startup (godotenv). All settings are persisted to the database and can also be configured via the `/admin` page.

| Variable | Required | Description |
|----------|----------|-------------|
| `TMDB_API_KEY` | Optional | TMDB API key for metadata |
| `SMTP_URL` | Optional | SMTP connection for magic links and password recovery |
| `WATCHLOG_URL` | Optional | Public URL for email links |

### Getting a TMDB API Key

> ⚠️ **TMDB integration is optional** but required for posters, synopses, genres, and upcoming episodes.

1. Create a free account at [themoviedb.org](https://www.themoviedb.org/signup)
2. Go to [Settings → API](https://www.themoviedb.org/settings/api) (use a desktop browser — the page is not mobile-optimized)
3. Click "Create" → choose "Developer" → accept the [Terms of Use](https://www.themoviedb.org/documentation/api/terms-of-use)
4. Fill in the application form (for personal/non-commercial use, a brief description is enough)
5. Copy your **API Key (v3 auth)** and add it to your `.env` file

More details: [TMDB Getting Started](https://developer.themoviedb.org/docs/getting-started)

## Importing TVTime Data

### 1. Export your TVTime data

Request your GDPR export from TVTime. You'll receive a ZIP with ~50 CSV files.

### 2. Run the import

Start the server and go through the setup wizard. On step 3, upload the ZIP file directly. Alternatively, after setup, go to `/import` to upload at any time.

### 3. Enrich later (if you skipped TMDB)

```bash
curl -X POST http://localhost:8080/api/tmdb/fetch-all
```

### Notes on Imported Data

- **Individual episodes**: TVTime only exports the ~400 most recent episodes with exact dates. The real total count per show is imported separately from `user_tv_show_data.csv`.
- **Movies**: imported from watch history + rated movies. Overlaps are deduplicated.
- **Shows not found on TMDB**: those with year in the name (e.g., "Foundation (2021)") are searched without the year automatically.

## REST API

### Shows
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/shows` | List shows |
| GET | `/api/shows/:id` | Show details |
| POST | `/api/shows/:id/follow` | Toggle follow |
| POST | `/api/shows/:id/favorite` | Toggle favorite |
| POST | `/api/shows/:id/archive` | Toggle archived |
| GET | `/api/shows/:id/episodes` | Episodes for a show |
| POST | `/api/shows/:id/episodes/watched` | Mark episode watched `{season, episode}` |
| DELETE | `/api/shows/:id/episodes/watched` | Unmark episode `{season, episode}` |
| POST | `/api/shows/:id/season/watched` | Mark full season `{season, episodes}` |
| DELETE | `/api/shows/:id/season/watched` | Unmark full season `{season}` |

### Movies
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/movies` | List movies |

### Lists
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/lists` | List all lists |
| GET | `/api/lists/:id` | List detail with items |
| POST | `/api/lists` | Create list `{name, is_public}` |
| PUT | `/api/lists/:id` | Edit list |
| DELETE | `/api/lists/:id` | Delete list |
| POST | `/api/lists/:id/items` | Add item `{show_id}` or `{movie_id}` |
| DELETE | `/api/lists/:id/items/:itemId` | Remove item |

### Statistics
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/stats` | Dashboard stats |
| GET | `/api/stats/history` | Monthly history |

### TMDB
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/tmdb/add-show` | Add show from TMDB `{tmdb_id}` |
| POST | `/api/tmdb/add-movie` | Add movie from TMDB `{tmdb_id}` |
| POST | `/api/shows/:id/fetch-tmdb` | Enrich a single show |
| POST | `/api/tmdb/fetch-all` | Enrich all without metadata |
| POST | `/api/tmdb/refresh-all` | Refresh all metadata (shows + movies + episodes) |

### Continue Watching
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/continue-watching?page=N` | Next unwatched episodes (HTMX partial) |

### Search
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/search?q=...` | Search your library |

## Project Structure

```
cmd/server/          Web server (API + HTMX frontend)
internal/db/         SQLite with WAL, auto-migrations
internal/models/     Domain models
internal/handlers/   HTTP handlers (API + pages)
internal/i18n/       Internationalization (ES/EN)
internal/importer/   CSV import logic (TVTime)
internal/tmdb/       TMDB API client
internal/auth/       Password hashing, sessions, cookies
internal/mail/       SMTP email sending
internal/ratelimit/  Login rate limiting
internal/cache/      Disk image cache
internal/worker/     Background workers (upcoming episodes)
web/templates/       HTML templates (HTMX + Tailwind)
web/static/          Static assets
```

## Releases

```bash
# Local snapshot (no tag needed)
make snapshot

# Tagged release
git tag v0.1.0
make release
```

GoReleaser builds for linux/darwin × amd64/arm64 and includes web templates in the archive.

## Tech Stack

- **Backend**: Go (stdlib `net/http`, no frameworks)
- **Database**: SQLite with WAL mode
- **Frontend**: HTML + HTMX + Tailwind CSS (CDN)
- **Design**: Minimalist editorial style
- **Metadata**: TMDB API v3
- **Build**: Make + GoReleaser
- **Container**: Distroless (nonroot)
- **Dev**: Nix flake (devShell)
- **Auth**: bcrypt password hashing, cookie sessions
- **i18n**: Spanish/English with browser detection + user preference

## License

Apache 2.0
