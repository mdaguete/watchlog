package db

import (
	"os"
	"testing"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/models"
)

func providersTestDB(t *testing.T) *DB {
	t.Helper()
	f, _ := os.CreateTemp("", "wl-prov-*.db")
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	database, err := New(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestUserRegion(t *testing.T) {
	database := providersTestDB(t)
	hash, _ := auth.HashPassword("x")
	uid, _ := database.CreateUser("u", hash)

	// Defaults to ES when unset.
	if r := database.GetUserRegion(uid); r != "ES" {
		t.Errorf("default region = %q, want ES", r)
	}
	if err := database.SetUserRegion(uid, "US"); err != nil {
		t.Fatal(err)
	}
	if r := database.GetUserRegion(uid); r != "US" {
		t.Errorf("region = %q, want US", r)
	}
}

func TestProviderCache(t *testing.T) {
	database := providersTestDB(t)

	// Missing cache row.
	if _, _, ok := database.GetProviderCache("tv", 123, "ES"); ok {
		t.Error("expected no cache row")
	}

	provs := []models.Provider{{Name: "Netflix", LogoPath: "/nf.jpg"}, {Name: "Max", LogoPath: "/max.jpg"}}
	if err := database.UpsertProviderCache("tv", 123, "ES", provs); err != nil {
		t.Fatal(err)
	}
	got, fetchedAt, ok := database.GetProviderCache("tv", 123, "ES")
	if !ok || len(got) != 2 || got[0].Name != "Netflix" || fetchedAt == "" {
		t.Fatalf("cache roundtrip failed: ok=%v got=%+v fetchedAt=%q", ok, got, fetchedAt)
	}
	// Region-scoped: a different region has no row.
	if _, _, ok := database.GetProviderCache("tv", 123, "US"); ok {
		t.Error("expected no US cache row")
	}
	// Upsert overwrites.
	database.UpsertProviderCache("tv", 123, "ES", []models.Provider{{Name: "Prime Video"}})
	if got, _, _ := database.GetProviderCache("tv", 123, "ES"); len(got) != 1 || got[0].Name != "Prime Video" {
		t.Errorf("upsert did not overwrite: %+v", got)
	}
}
