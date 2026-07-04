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
