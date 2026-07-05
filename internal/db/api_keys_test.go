package db

import (
	"strings"
	"testing"
)

// TestAPIKey_StoredHashed verifies that CreateAPIKey never persists the raw key
// and that ValidateAPIKey resolves the raw key via its hash.
func TestAPIKey_StoredHashed(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")

	raw := "wl_deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	if _, err := db.CreateAPIKey(uid, raw, "test", "read,write"); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// The raw key must not be present in the table.
	var stored string
	if err := db.conn.QueryRow("SELECT key_hash FROM api_keys WHERE user_id = ?", uid).Scan(&stored); err != nil {
		t.Fatalf("select: %v", err)
	}
	if stored == raw {
		t.Fatal("raw API key stored in plaintext")
	}
	if strings.HasPrefix(stored, "wl_") {
		t.Errorf("stored value looks like a raw key: %q", stored)
	}
	if stored != HashAPIKey(raw) {
		t.Errorf("stored value is not the SHA-256 hash of the key")
	}

	// Validation with the raw key succeeds and returns scopes.
	gotUID, scopes, ok := db.ValidateAPIKey(raw)
	if !ok || gotUID != uid {
		t.Fatalf("ValidateAPIKey failed: uid=%d ok=%v", gotUID, ok)
	}
	if scopes != "read,write" {
		t.Errorf("scopes = %q, want read,write", scopes)
	}

	// A wrong key is rejected.
	if _, _, ok := db.ValidateAPIKey("wl_wrong"); ok {
		t.Error("ValidateAPIKey accepted an unknown key")
	}
}

// TestMigrateV11_HashesExistingKeys inserts a plaintext key (as pre-v11 code
// would) and verifies migrateV11 rewrites it to its hash while remaining valid.
func TestMigrateV11_HashesExistingKeys(t *testing.T) {
	db := newTestDB(t)
	uid, _ := db.CreateUser("u", "h")

	raw := "wl_00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	// Simulate legacy plaintext storage directly.
	if _, err := db.conn.Exec("INSERT INTO api_keys (user_id, key_hash, name, scopes) VALUES (?, ?, ?, ?)", uid, raw, "legacy", "read"); err != nil {
		t.Fatalf("insert legacy key: %v", err)
	}

	tx, err := db.conn.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := migrateV11(tx); err != nil {
		tx.Rollback()
		t.Fatalf("migrateV11: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var stored string
	db.conn.QueryRow("SELECT key_hash FROM api_keys WHERE user_id = ?", uid).Scan(&stored)
	if stored != HashAPIKey(raw) {
		t.Errorf("legacy key not hashed by migration: got %q", stored)
	}

	// The original raw key still authenticates after migration.
	if _, _, ok := db.ValidateAPIKey(raw); !ok {
		t.Error("raw key no longer validates after v11 migration")
	}

	// Running the migration again is a no-op (idempotent).
	tx2, _ := db.conn.Begin()
	if err := migrateV11(tx2); err != nil {
		tx2.Rollback()
		t.Fatalf("migrateV11 (second run): %v", err)
	}
	tx2.Commit()
	var after string
	db.conn.QueryRow("SELECT key_hash FROM api_keys WHERE user_id = ?", uid).Scan(&after)
	if after != stored {
		t.Errorf("second migration run mutated the hash")
	}
}
