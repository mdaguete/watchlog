package db

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Migration represents a single database migration step.
type Migration struct {
	Version          int
	Description      string
	Up               func(tx *sql.Tx) error
	NeedsTMDBRefresh bool // if true, a TMDB refresh is needed after this migration
}

// migrations is the ordered list of all database migrations.
// Each migration builds on the previous one.
var migrations = []Migration{
	{Version: 1, Description: "initial schema", Up: migrateV1},
	{Version: 2, Description: "i18n columns for shows and movies", Up: migrateV2},
	{Version: 3, Description: "email column on users", Up: migrateV3},
	{Version: 4, Description: "season episodes cache table", Up: migrateV4},
	{Version: 5, Description: "episode details table", Up: migrateV5, NeedsTMDBRefresh: true},
	{Version: 6, Description: "episode still image URL", Up: migrateV6, NeedsTMDBRefresh: true},
	{Version: 7, Description: "snooze shows from continue watching", Up: migrateV7},
	{Version: 8, Description: "user theme preference", Up: migrateV8},
	{Version: 9, Description: "API keys for MCP", Up: migrateV9},
	{Version: 10, Description: "user blocked column", Up: migrateV10},
	{Version: 11, Description: "hash existing plaintext API keys", Up: migrateV11},
	{Version: 12, Description: "user invitations", Up: migrateV12},
	{Version: 13, Description: "normalize watched_at to ISO-8601 text", Up: migrateV13},
	{Version: 14, Description: "movie release_date column", Up: migrateV14},
	{Version: 15, Description: "merge duplicate shows by name", Up: migrateV15},
	{Version: 16, Description: "viewing-history import staging tables", Up: migrateV16},
	{Version: 17, Description: "viewing-history unmatched entries", Up: migrateV17},
	{Version: 18, Description: "drop unused lists tables", Up: migrateV18},
	{Version: 19, Description: "streaming providers on shows and movies", Up: migrateV19},
}

// runMigrations checks the current schema version and applies pending migrations.
func (db *DB) runMigrations() error {
	// Ensure schema_migrations table exists
	_, err := db.conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	current := db.currentVersion()

	// Bootstrap: if tables exist but no migrations recorded, detect and mark
	if current == 0 && db.tablesExist() {
		bootstrapVersion := db.detectVersion()
		if bootstrapVersion > 0 {
			log.Printf("DB: existing database detected, marking as version %d", bootstrapVersion)
			for _, m := range migrations {
				if m.Version <= bootstrapVersion {
					db.conn.Exec("INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, ?)", m.Version, time.Now())
				}
			}
			current = bootstrapVersion
		}
	}

	latest := migrations[len(migrations)-1].Version
	if current == latest {
		log.Printf("DB: schema at version %d (up to date)", current)
		return nil
	}

	log.Printf("DB: migrating from version %d to %d...", current, latest)

	// Backup database before applying migrations
	if err := db.backupBeforeMigration(current); err != nil {
		log.Printf("DB: WARNING: backup failed: %v (proceeding with migration)", err)
	}

	needsRefresh := false
	for _, m := range migrations {
		if m.Version <= current {
			continue
		}

		tx, err := db.conn.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for v%d: %w", m.Version, err)
		}

		log.Printf("DB: applying migration v%d: %s", m.Version, m.Description)
		if err := m.Up(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration v%d (%s): %w", m.Version, m.Description, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", m.Version, time.Now()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration v%d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration v%d: %w", m.Version, err)
		}

		if m.NeedsTMDBRefresh {
			needsRefresh = true
		}
	}

	if needsRefresh {
		db.SetSetting("tmdb_refresh_pending", "1")
		log.Printf("DB: TMDB refresh scheduled (migration requires data update)")
	}

	log.Printf("DB: migration complete, now at version %d", latest)
	return nil
}

// currentVersion returns the highest applied migration version, or 0 if none.
func (db *DB) currentVersion() int {
	var version int
	db.conn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	return version
}

// tablesExist checks if the main application tables are present (pre-migration DB).
func (db *DB) tablesExist() bool {
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&count)
	return count > 0
}

// detectVersion inspects the database to determine which migrations have already been applied.
func (db *DB) detectVersion() int {
	version := 0

	// If users table exists, at least v1
	var userCount int
	db.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&userCount)
	if userCount > 0 {
		version = 1
	}

	// Check for i18n columns (v2)
	var overviewEN int
	db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('shows') WHERE name='overview_en'").Scan(&overviewEN)
	if overviewEN > 0 {
		version = 2
	}

	// Check for email column (v3)
	var emailCol int
	db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='email'").Scan(&emailCol)
	if emailCol > 0 {
		version = 3
	}

	return version
}

// --- Migration functions ---

func migrateV1(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	lang TEXT NOT NULL DEFAULT 'es',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS shows (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	external_id INTEGER UNIQUE NOT NULL,
	name TEXT NOT NULL,
	tmdb_id INTEGER NOT NULL DEFAULT 0,
	poster_url TEXT NOT NULL DEFAULT '',
	backdrop_url TEXT NOT NULL DEFAULT '',
	overview TEXT NOT NULL DEFAULT '',
	genres TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT '',
	total_seasons INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_shows (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	show_id INTEGER NOT NULL REFERENCES shows(id),
	is_followed INTEGER NOT NULL DEFAULT 1,
	is_favorited INTEGER NOT NULL DEFAULT 0,
	is_archived INTEGER NOT NULL DEFAULT 0,
	episodes_seen INTEGER NOT NULL DEFAULT 0,
	followed_at DATETIME,
	updated_at DATETIME,
	UNIQUE(user_id, show_id)
);

CREATE TABLE IF NOT EXISTS episodes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	external_id INTEGER NOT NULL,
	show_id INTEGER NOT NULL REFERENCES shows(id),
	season_number INTEGER NOT NULL,
	episode_number INTEGER NOT NULL,
	watched INTEGER NOT NULL DEFAULT 1,
	watched_at DATETIME,
	runtime INTEGER NOT NULL DEFAULT 0,
	UNIQUE(user_id, show_id, season_number, episode_number)
);

CREATE TABLE IF NOT EXISTS movies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	external_id TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL,
	tmdb_id INTEGER NOT NULL DEFAULT 0,
	poster_url TEXT NOT NULL DEFAULT '',
	overview TEXT NOT NULL DEFAULT '',
	genres TEXT NOT NULL DEFAULT '',
	runtime INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_movies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	movie_id INTEGER NOT NULL REFERENCES movies(id),
	watched_at DATETIME,
	UNIQUE(user_id, movie_id)
);

CREATE TABLE IF NOT EXISTS lists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	name TEXT NOT NULL,
	is_public INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME
);

CREATE TABLE IF NOT EXISTS list_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	list_id INTEGER NOT NULL REFERENCES lists(id),
	entity_type TEXT NOT NULL,
	entity_id INTEGER NOT NULL DEFAULT 0,
	name TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS watch_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	period TEXT NOT NULL,
	count INTEGER NOT NULL DEFAULT 0,
	runtime INTEGER NOT NULL DEFAULT 0,
	UNIQUE(user_id, period)
);

CREATE TABLE IF NOT EXISTS show_progress (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	show_id INTEGER NOT NULL REFERENCES shows(id),
	show_name TEXT NOT NULL,
	last_season_number INTEGER NOT NULL DEFAULT 0,
	last_episode_number INTEGER NOT NULL DEFAULT 0,
	last_episode_id INTEGER NOT NULL DEFAULT 0,
	updated_at DATETIME,
	UNIQUE(user_id, show_id)
);

CREATE TABLE IF NOT EXISTS upcoming_cache (
	show_id INTEGER PRIMARY KEY REFERENCES shows(id),
	show_name TEXT NOT NULL DEFAULT '',
	poster_url TEXT NOT NULL DEFAULT '',
	episode_name TEXT NOT NULL DEFAULT '',
	season_number INTEGER NOT NULL DEFAULT 0,
	episode_number INTEGER NOT NULL DEFAULT 0,
	air_date TEXT NOT NULL DEFAULT '',
	overview TEXT NOT NULL DEFAULT '',
	fetched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY NOT NULL,
	value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY NOT NULL,
	user_id INTEGER NOT NULL REFERENCES users(id),
	expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS magic_links (
	token TEXT PRIMARY KEY NOT NULL,
	user_id INTEGER NOT NULL REFERENCES users(id),
	purpose TEXT NOT NULL,
	expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_user_shows_user ON user_shows(user_id);
CREATE INDEX IF NOT EXISTS idx_user_shows_show ON user_shows(show_id);
CREATE INDEX IF NOT EXISTS idx_episodes_user_show ON episodes(user_id, show_id);
CREATE INDEX IF NOT EXISTS idx_episodes_external_id ON episodes(external_id);
CREATE INDEX IF NOT EXISTS idx_lists_user ON lists(user_id);
CREATE INDEX IF NOT EXISTS idx_watch_stats_user ON watch_stats(user_id);
CREATE INDEX IF NOT EXISTS idx_show_progress_user ON show_progress(user_id, show_id);
CREATE INDEX IF NOT EXISTS idx_user_movies_user ON user_movies(user_id);
`)
	return err
}

func migrateV2(tx *sql.Tx) error {
	stmts := []string{
		"ALTER TABLE shows ADD COLUMN overview_en TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE shows ADD COLUMN genres_en TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE shows ADD COLUMN name_es TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE shows ADD COLUMN name_en TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE movies ADD COLUMN overview_en TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE movies ADD COLUMN genres_en TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE movies ADD COLUMN name_es TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE movies ADD COLUMN name_en TEXT NOT NULL DEFAULT ''",
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func migrateV3(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''")
	return err
}

func migrateV4(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS season_episodes (
	show_id INTEGER NOT NULL REFERENCES shows(id),
	season_number INTEGER NOT NULL,
	episode_count INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (show_id, season_number)
)`)
	return err
}

func migrateV5(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS episode_details (
	show_id INTEGER NOT NULL REFERENCES shows(id),
	season_number INTEGER NOT NULL,
	episode_number INTEGER NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	name_en TEXT NOT NULL DEFAULT '',
	overview TEXT NOT NULL DEFAULT '',
	overview_en TEXT NOT NULL DEFAULT '',
	air_date TEXT NOT NULL DEFAULT '',
	runtime INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (show_id, season_number, episode_number)
)`)
	return err
}

func migrateV6(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE episode_details ADD COLUMN still_url TEXT NOT NULL DEFAULT ''")
	return err
}

func migrateV7(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE user_shows ADD COLUMN snoozed_until DATETIME")
	return err
}

func migrateV8(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT 'system'")
	return err
}

// TMDBRefreshPending returns true if a migration flagged a TMDB refresh as needed.
func (db *DB) TMDBRefreshPending() bool {
	return db.GetSetting("tmdb_refresh_pending") == "1"
}

// ClearTMDBRefreshPending clears the pending refresh flag.
func (db *DB) ClearTMDBRefreshPending() {
	db.SetSetting("tmdb_refresh_pending", "0")
}

// Backup creates a timestamped backup of the database file with a descriptive
// label. Returns the backup path or an error.
func (db *DB) Backup(label string) (string, error) {
	if db.path == "" {
		return "", fmt.Errorf("database path not set")
	}
	backupDir := filepath.Join(filepath.Dir(db.path), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	baseName := filepath.Base(db.path)
	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s.%s.%s.bak", baseName, label, timestamp)
	backupPath := filepath.Join(backupDir, backupName)
	src, err := os.Open(db.path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.Create(backupPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return backupPath, nil
}

// backupBeforeMigration copies the database file to a backups/ folder next to the DB.
func (db *DB) backupBeforeMigration(currentVersion int) error {
	if db.path == "" {
		return fmt.Errorf("database path not set")
	}

	backupDir := filepath.Join(filepath.Dir(db.path), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	baseName := filepath.Base(db.path)
	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s.v%d.%s.bak", baseName, currentVersion, timestamp)
	backupPath := filepath.Join(backupDir, backupName)

	src, err := os.Open(db.path)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	log.Printf("DB: backup created at %s", backupPath)
	return nil
}

func migrateV9(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	key_hash TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL,
	scopes TEXT NOT NULL DEFAULT 'read',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_used_at DATETIME
)`)
	return err
}

func migrateV10(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE users ADD COLUMN blocked INTEGER NOT NULL DEFAULT 0")
	return err
}

// migrateV11 replaces any plaintext API keys stored at rest with their SHA-256
// hash. Keys created before this migration were stored verbatim and carry the
// "wl_" prefix; hashed values (64 hex chars) never do, so the prefix reliably
// identifies rows that still need hashing. This is idempotent.
func migrateV11(tx *sql.Tx) error {
	rows, err := tx.Query("SELECT id, key_hash FROM api_keys WHERE key_hash LIKE 'wl\\_%' ESCAPE '\\'")
	if err != nil {
		return err
	}
	type row struct {
		id  int64
		key string
	}
	var toHash []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.key); err != nil {
			rows.Close()
			return err
		}
		toHash = append(toHash, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range toHash {
		if _, err := tx.Exec("UPDATE api_keys SET key_hash = ? WHERE id = ?", HashAPIKey(r.key), r.id); err != nil {
			return err
		}
	}
	return nil
}

func migrateV12(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS invitations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL DEFAULT '',
	token TEXT UNIQUE NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL,
	accepted_at DATETIME
)`)
	return err
}

// migrateV13 normalizes watched_at values to the canonical ISO-8601 text layout
// ("2006-01-02 15:04:05"). Older rows were stored with Go's time.String() format
// ("2006-01-02 15:04:05 +0000 UTC"), which SQLite's date/time functions cannot
// parse. This reformats them so they are sortable and natively parseable.
// Idempotent: values already in the canonical layout are left unchanged.
func migrateV13(tx *sql.Tx) error {
	// Candidate layouts seen in existing data.
	layouts := []string{
		"2006-01-02 15:04:05 -0700 MST", // Go time.String()
		"2006-01-02T15:04:05Z07:00",     // RFC3339
		"2006-01-02 15:04:05.999999999 -0700 MST",
		WatchedAtLayout, // already canonical
	}
	normalize := func(table, col, pk string) error {
		// Skip if the table doesn't exist (partial/pre-existing databases).
		var exists int
		tx.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&exists)
		if exists == 0 {
			return nil
		}
		rows, err := tx.Query("SELECT " + pk + ", " + col + " FROM " + table + " WHERE " + col + " IS NOT NULL AND " + col + " != ''")
		if err != nil {
			return err
		}
		type row struct {
			id  int64
			val string
		}
		var toFix []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.val); err != nil {
				rows.Close()
				return err
			}
			// Already canonical (exactly 19 chars, no trailing zone).
			if len(r.val) == len(WatchedAtLayout) {
				continue
			}
			toFix = append(toFix, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range toFix {
			var norm string
			for _, l := range layouts {
				if t, err := time.Parse(l, r.val); err == nil {
					norm = t.Format(WatchedAtLayout)
					break
				}
			}
			if norm == "" {
				// Fallback: keep the first 19 chars if they look like a datetime.
				if len(r.val) >= 19 {
					norm = r.val[:19]
				} else {
					continue
				}
			}
			if _, err := tx.Exec("UPDATE "+table+" SET "+col+" = ? WHERE "+pk+" = ?", norm, r.id); err != nil {
				return err
			}
		}
		return nil
	}
	if err := normalize("episodes", "watched_at", "id"); err != nil {
		return err
	}
	return normalize("user_movies", "watched_at", "id")
}

func migrateV14(tx *sql.Tx) error {
	_, err := tx.Exec("ALTER TABLE movies ADD COLUMN release_date TEXT NOT NULL DEFAULT ''")
	return err
}

// migrateV15 merges duplicate catalog shows that share the same name (caused by
// an old importer bug that created one show per episode). For each duplicated
// name it keeps a canonical show (prefer one tracked in user_shows, else lowest
// id), moves per-user data (episodes, user_shows, show_progress, list items) to
// it — ignoring unique-key conflicts — and deletes the duplicates and their
// cached season/episode metadata. episodes_seen is left untouched (it holds the
// real total imported from TVTime, which may exceed the individual episode rows).
func migrateV15(tx *sql.Tx) error {
	var exists int
	tx.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='shows'").Scan(&exists)
	if exists == 0 {
		return nil
	}

	rows, err := tx.Query("SELECT name FROM shows GROUP BY name HAVING COUNT(*) > 1")
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, name := range names {
		var canonical int64
		if err := tx.QueryRow(`SELECT s.id FROM shows s WHERE s.name = ?
			ORDER BY (SELECT COUNT(*) FROM user_shows us WHERE us.show_id = s.id) DESC, s.id ASC
			LIMIT 1`, name).Scan(&canonical); err != nil {
			return err
		}

		drows, err := tx.Query("SELECT id FROM shows WHERE name = ? AND id != ?", name, canonical)
		if err != nil {
			return err
		}
		var dups []int64
		for drows.Next() {
			var id int64
			if err := drows.Scan(&id); err != nil {
				drows.Close()
				return err
			}
			dups = append(dups, id)
		}
		drows.Close()

		for _, d := range dups {
			stmts := []string{
				"UPDATE OR IGNORE episodes SET show_id = ? WHERE show_id = ?",
				"DELETE FROM episodes WHERE show_id = ?",
				"UPDATE OR IGNORE user_shows SET show_id = ? WHERE show_id = ?",
				"DELETE FROM user_shows WHERE show_id = ?",
				"UPDATE OR IGNORE show_progress SET show_id = ? WHERE show_id = ?",
				"DELETE FROM show_progress WHERE show_id = ?",
			}
			// The first of each move/delete pair takes (canonical, d); deletes take (d).
			if _, err := tx.Exec(stmts[0], canonical, d); err != nil {
				return err
			}
			if _, err := tx.Exec(stmts[1], d); err != nil {
				return err
			}
			if _, err := tx.Exec(stmts[2], canonical, d); err != nil {
				return err
			}
			if _, err := tx.Exec(stmts[3], d); err != nil {
				return err
			}
			if _, err := tx.Exec(stmts[4], canonical, d); err != nil {
				return err
			}
			if _, err := tx.Exec(stmts[5], d); err != nil {
				return err
			}
			tx.Exec("UPDATE list_items SET entity_id = ? WHERE entity_type = 'series' AND entity_id = ?", canonical, d)
			tx.Exec("DELETE FROM season_episodes WHERE show_id = ?", d)
			tx.Exec("DELETE FROM episode_details WHERE show_id = ?", d)
			tx.Exec("DELETE FROM upcoming_cache WHERE show_id = ?", d)
			if _, err := tx.Exec("DELETE FROM shows WHERE id = ?", d); err != nil {
				return err
			}
		}
	}
	return nil
}

// migrateV16 creates the staging tables used by the viewing-history import
// (Netflix, etc.): a batch per upload and one row per proposed watched-date
// change, so the user can review, edit and confirm changes over time.
func migrateV16(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS import_batches (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			source TEXT NOT NULL DEFAULT 'netflix',
			filename TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			entries INTEGER NOT NULL DEFAULT 0,
			series_matched INTEGER NOT NULL DEFAULT 0,
			unmatched_series TEXT,
			created_at TEXT NOT NULL,
			applied_at TEXT,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS import_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			target_id INTEGER NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			season INTEGER NOT NULL DEFAULT 0,
			episode INTEGER NOT NULL DEFAULT 0,
			netflix_title TEXT NOT NULL DEFAULT '',
			current_date TEXT NOT NULL DEFAULT '',
			new_date TEXT NOT NULL DEFAULT '',
			selected INTEGER NOT NULL DEFAULT 1,
			applied INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(batch_id) REFERENCES import_batches(id) ON DELETE CASCADE
		)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_import_changes_batch ON import_changes(batch_id)`)
	return err
}

// migrateV17 stores the individual Netflix entries whose series/movie was not
// found in the library, so the user can later reconcile them against TMDB and
// apply their watched dates.
func migrateV17(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS import_unmatched (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			kind TEXT NOT NULL DEFAULT 'series',
			netflix_name TEXT NOT NULL,
			season INTEGER NOT NULL DEFAULT 0,
			netflix_episode TEXT NOT NULL DEFAULT '',
			watched_date TEXT NOT NULL DEFAULT '',
			resolved INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(batch_id) REFERENCES import_batches(id) ON DELETE CASCADE
		)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_import_unmatched_batch ON import_unmatched(batch_id)`)
	return err
}

// migrateV18 drops the lists and list_items tables. The Lists feature was
// removed from the app; the tables are no longer read or written.
func migrateV18(tx *sql.Tx) error {
	if _, err := tx.Exec(`DROP TABLE IF EXISTS list_items`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE IF EXISTS lists`); err != nil {
		return err
	}
	return nil
}

// migrateV19 adds a providers column (JSON array of streaming platforms for the
// configured region) to shows and movies.
func migrateV19(tx *sql.Tx) error {
	for _, table := range []string{"shows", "movies"} {
		if _, err := tx.Exec("ALTER TABLE " + table + " ADD COLUMN providers TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	return nil
}
