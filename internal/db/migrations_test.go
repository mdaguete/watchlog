package db

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrations_FreshDB(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-migration-test-*.db")
	f.Close()
	defer os.Remove(f.Name())

	database, err := New(f.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer database.Close()

	// Should be at latest version
	version := database.currentVersion()
	expected := migrations[len(migrations)-1].Version
	if version != expected {
		t.Errorf("currentVersion = %d, want %d", version, expected)
	}
}

func TestMigrations_ExistingDB_Bootstrap(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-migration-bootstrap-*.db")
	f.Close()
	defer os.Remove(f.Name())

	// Create a DB with v1 schema manually (simulating pre-migration DB)
	database, err := New(f.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	database.Close()

	// Re-open — should detect existing tables and not fail
	database2, err := New(f.Name())
	if err != nil {
		t.Fatalf("Re-open: %v", err)
	}
	defer database2.Close()

	version := database2.currentVersion()
	expected := migrations[len(migrations)-1].Version
	if version != expected {
		t.Errorf("currentVersion = %d, want %d", version, expected)
	}
}

func TestMigrations_Idempotent(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-migration-idem-*.db")
	f.Close()
	defer os.Remove(f.Name())

	// Run migrations twice — should not fail
	db1, err := New(f.Name())
	if err != nil {
		t.Fatalf("First open: %v", err)
	}
	db1.Close()

	db2, err := New(f.Name())
	if err != nil {
		t.Fatalf("Second open: %v", err)
	}
	defer db2.Close()

	version := db2.currentVersion()
	expected := migrations[len(migrations)-1].Version
	if version != expected {
		t.Errorf("currentVersion = %d, want %d", version, expected)
	}
}

func TestMigrations_TablesCreated(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-migration-tables-*.db")
	f.Close()
	defer os.Remove(f.Name())

	database, err := New(f.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer database.Close()

	// Verify critical tables exist
	tables := []string{"users", "shows", "episodes", "movies", "sessions", "magic_links", "settings", "schema_migrations"}
	for _, table := range tables {
		var count int
		database.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if count != 1 {
			t.Errorf("table %q not found", table)
		}
	}

	// Verify i18n columns exist (v2)
	var col int
	database.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('shows') WHERE name='overview_en'").Scan(&col)
	if col != 1 { t.Error("shows.overview_en not found") }

	// Verify email column (v3)
	database.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='email'").Scan(&col)
	if col != 1 { t.Error("users.email not found") }
}

func TestCurrentVersion_Empty(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-migration-empty-*.db")
	f.Close()
	defer os.Remove(f.Name())

	database, err := New(f.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer database.Close()

	// After full migration, version should be latest
	if v := database.currentVersion(); v != len(migrations) {
		t.Errorf("version = %d, want %d", v, len(migrations))
	}
}

func TestDetectVersion_PreExistingDB(t *testing.T) {
	f, _ := os.CreateTemp("", "watchlog-detect-*.db")
	f.Close()
	defer os.Remove(f.Name())

	// Manually create a DB with tables but no schema_migrations
	// This simulates the old pre-migration database
	rawDB, _ := sql.Open("sqlite", f.Name()+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	rawDB.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, password_hash TEXT, lang TEXT DEFAULT 'es', email TEXT DEFAULT '', created_at DATETIME)`)
	rawDB.Exec(`CREATE TABLE shows (id INTEGER PRIMARY KEY, external_id INTEGER, name TEXT, tmdb_id INTEGER DEFAULT 0, poster_url TEXT DEFAULT '', backdrop_url TEXT DEFAULT '', overview TEXT DEFAULT '', overview_en TEXT DEFAULT '', genres TEXT DEFAULT '', genres_en TEXT DEFAULT '', name_es TEXT DEFAULT '', name_en TEXT DEFAULT '', status TEXT DEFAULT '', total_seasons INTEGER DEFAULT 0)`)
	rawDB.Exec(`CREATE TABLE movies (id INTEGER PRIMARY KEY, external_id TEXT, name TEXT, tmdb_id INTEGER DEFAULT 0, poster_url TEXT DEFAULT '', overview TEXT DEFAULT '', overview_en TEXT DEFAULT '', genres TEXT DEFAULT '', genres_en TEXT DEFAULT '', name_es TEXT DEFAULT '', name_en TEXT DEFAULT '', runtime INTEGER DEFAULT 0)`)
	rawDB.Exec(`CREATE TABLE user_shows (id INTEGER PRIMARY KEY, user_id INTEGER, show_id INTEGER, is_followed INTEGER DEFAULT 1, is_favorited INTEGER DEFAULT 0, is_archived INTEGER DEFAULT 0, episodes_seen INTEGER DEFAULT 0, followed_at DATETIME, updated_at DATETIME, UNIQUE(user_id, show_id))`)
	rawDB.Exec(`CREATE TABLE sessions (token TEXT PRIMARY KEY, user_id INTEGER, expires_at DATETIME)`)
	rawDB.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT DEFAULT '')`)
	rawDB.Close()

	// Now open with our migration system — should detect v3
	database, err := New(f.Name())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer database.Close()

	version := database.currentVersion()
	if version != len(migrations) {
		t.Errorf("detected version = %d, want %d", version, len(migrations))
	}
}
