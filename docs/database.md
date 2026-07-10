# Database

SQLite with WAL mode, single `.db` file. Pure Go driver (`modernc.org/sqlite`, no CGO).

## Schema & Constraints

- `MaxOpenConns(1)` — SQLite doesn't support concurrent writes
- WAL mode for concurrent reads during writes
- Episode UNIQUE constraint: `(user_id, show_id, season_number, episode_number)` — an episode is recorded once per user
- `watched_at` dates stored in Go format (`2006-01-02 15:04:05 +0000 UTC`), not ISO 8601. Use `substr(watched_at, 1, 7)` in SQL instead of `strftime()`

## Deadlock Rules

**Critical**: never hold a `rows` cursor open while calling `Exec` — this deadlocks with `MaxOpenConns(1)`. Always read rows into a slice first, close the cursor, then execute writes.

## Migrations

- Automatic on startup via `schema_migrations` table
- Each migration is a Go function that runs in a transaction (rollback on failure)
- Detects pre-existing databases and bootstraps version without re-running migrations
- Migration code in `internal/db/migrations.go` (v1–v18)

### Recent migrations

- **v13**: `watched_at` normalized to ISO-8601 text
- **v14**: `movies.release_date` (year-aware matching)
- **v15**: merge duplicate catalog shows by name
- **v16**: viewing-history import staging tables (`import_batches`, `import_changes`)
- **v17**: viewing-history unmatched entries (`import_unmatched`)
- **v18**: drop the unused `lists` / `list_items` tables (Lists feature removed)

## Backup

- **Backup before migration**: copies DB file to `backups/` folder before applying pending migrations
- Single-file backup (just copy the `.db` file when WAL is checkpointed)

## Post-Migration TMDB Refresh

- Migrations can flag `NeedsTMDBRefresh: true`
- On startup, if pending, a background goroutine runs a full TMDB refresh automatically
- Implemented in `internal/worker/refresh.go` → `RunTMDBRefresh`

## Sessions

- Session tokens: 32 bytes from `crypto/rand`, stored in SQLite `sessions` table (persistent across restarts)
- Periodic cleanup of expired sessions (goroutine)

## Watch Stats

- `watch_stats` table incremented in real-time when marking episodes via web
- Stats imported from CSV then recalculated from actual DB data (episodes + movies `watched_at`)

## CSV Importer

- TVTime export has ~50 CSV files, only ~9 are used
- `tracking-prod-records-v2.csv` (watch-episode-*): main episode source with dates
- `user_tv_show_data.csv` field `nb_episodes_seen` is the real total episode count per show
- Individual imported episodes are a subset (TVTime only exports recent tracking)
- Movies come from two sources: watch history + ratings
- Watch stats are imported from CSV then recalculated from actual DB data to cover periods not in the CSV

## Episodes & Seasons

- Users can mark/unmark individual episodes (toggle)
- "Mark all" button on a season toggles: marks all if any unwatched, unmarks all if all watched
- Episode count per season comes from TMDB (fallback: max episode number seen)
- **Auto-archive**: when all episodes are watched and show status is "Ended"/"Canceled", the show is automatically archived
- **Auto-unarchive**: unmarking an episode on a completed+archived show unarchives it

## Security (DB-related)

- bcrypt password hashing (DefaultCost)
- API keys stored hashed in `api_keys` table, prefixed `wl_`
- Rate limiting: max 5 failed login attempts per IP / 15-minute window (in-memory, not DB)
- User blocking: blocked users cannot login (managed via `watchdog` CLI)

## Key Files

| File | Responsibility |
|------|---------------|
| `internal/db/db.go` | Schema, migrations, all queries |
| `internal/db/migrations.go` | Database migrations (v1–v18) |
| `internal/models/models.go` | Domain structs |
| `internal/importer/importer.go` | CSV parsing logic for TVTime export |
| `internal/auth/auth.go` | Password hashing, sessions, cookies |
