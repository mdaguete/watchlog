package db

import (
	"os"
	"testing"

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

func TestShowMovieProvidersRoundtrip(t *testing.T) {
	database := providersTestDB(t)

	showID, _ := database.UpsertShow(models.Show{ExternalID: 1, Name: "S"})
	if err := database.UpdateShowProviders(showID, []models.Provider{
		{Name: "Netflix", LogoPath: "/nf.jpg"},
		{Name: "Max", LogoPath: "/max.jpg"},
	}); err != nil {
		t.Fatal(err)
	}
	s, err := database.GetShow(showID)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Providers) != 2 || s.Providers[0].Name != "Netflix" || s.Providers[0].LogoPath != "/nf.jpg" {
		t.Fatalf("show providers roundtrip failed: %+v", s.Providers)
	}

	movieID, _ := database.UpsertMovie(models.Movie{ExternalID: "m1", Name: "M"})
	database.UpdateMovieProviders(movieID, []models.Provider{{Name: "Prime Video", LogoPath: "/pv.jpg"}})
	m, _ := database.GetMovie(movieID)
	if len(m.Providers) != 1 || m.Providers[0].Name != "Prime Video" {
		t.Fatalf("movie providers roundtrip failed: %+v", m.Providers)
	}

	// A title with no providers parses to an empty slice, not an error.
	showID2, _ := database.UpsertShow(models.Show{ExternalID: 2, Name: "S2"})
	s2, _ := database.GetShow(showID2)
	if len(s2.Providers) != 0 {
		t.Errorf("expected no providers, got %+v", s2.Providers)
	}
}
