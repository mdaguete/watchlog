# TMDB Integration

TMDB API v3 client for metadata enrichment (posters, synopses, genres, airing status, episode details).

## Configuration

- API key in `.env` loaded with godotenv (`TMDB_API_KEY`)
- HTTP client with 10s timeout

## Metadata Caching

- Metadata cached in SQLite: `poster_url`, `overview`, `genres`, `status`
- Episode details: `name`, `synopsis`, `air_date`, `runtime`, `still_url` per episode
- Episode still images stored as `still_url` (TMDB w300 size)
- All metadata stored in both Spanish (es) and English (en)

## Operations

### fetch-all

- Processes all shows/movies without `tmdb_id`
- Logs individual progress
- Endpoint: `POST /api/tmdb/fetch-all`

### refresh-all

- Updates metadata for all shows/movies that already have a `tmdb_id`
- Refreshes both es + en translations
- Includes episode details refresh
- Endpoint: `POST /api/tmdb/refresh-all`

### Single show fetch

- Endpoint: `POST /api/shows/:id/fetch-tmdb`
- Enriches a single show on demand

### Add from TMDB

- `POST /api/tmdb/add-show` — add show by `{tmdb_id}`
- `POST /api/tmdb/add-movie` — add movie by `{tmdb_id}`

## Search Behavior

- Show search strips `(YYYY)` suffix from name if no results found initially
- Season episode counts fetched from TMDB for show detail page grids

## Background Worker

- Background goroutine refreshes upcoming episodes cache daily
- Accepts `context.Context` for graceful cancellation
- Only processes shows with status != "Ended"/"Canceled" and `tmdb_id > 0`
- `RunTMDBRefresh`: full metadata refresh callable programmatically (used by post-migration hook)

## Auto-Unarchive

- When TMDB refresh detects new season on "Returning Series", unarchives the show for all users

## Key Files

| File | Responsibility |
|------|---------------|
| `internal/tmdb/client.go` | HTTP client for TMDB API v3 |
| `internal/worker/upcoming.go` | Background worker for upcoming episode cache |
| `internal/worker/refresh.go` | Full TMDB metadata refresh (post-migration hook) |
