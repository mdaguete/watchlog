package main

import (
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/importer"
	"github.com/mdaguete/watchlog/internal/tmdb"
	"github.com/mdaguete/watchlog/internal/worker"
)

func main() {
	dataDir := flag.String("data", "./data", "Path to TVTime export data directory")
	dbPath := flag.String("db", "./watchlog.db", "Path to SQLite database")
	username := flag.String("user", "admin", "Username to import data for (created if not exists)")
	skipTMDB := flag.Bool("skip-tmdb", false, "Skip TMDB metadata fetch")
	flag.Parse()

	godotenv.Load()

	log.Println("WatchLog - TVTime Data Importer")
	log.Printf("Data dir: %s", *dataDir)
	log.Printf("Database: %s", *dbPath)

	database, err := db.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Import CSV data
	// Ensure a default user exists for the import
	user, err := database.GetUserByUsername(*username)
	if err != nil {
		hash, _ := auth.HashPassword("watchlog")
		userID, err := database.CreateUser(*username, hash)
		if err != nil {
			log.Fatalf("Failed to create user: %v", err)
		}
		log.Printf("Created user %q (id=%d) with default password 'watchlog'", *username, userID)
		user.ID = userID
	} else {
		log.Printf("Using existing user %q (id=%d)", user.Username, user.ID)
	}

	imp := importer.New(database, *dataDir, user.ID)
	if err := imp.ImportAll(); err != nil {
		log.Fatalf("Import failed: %v", err)
	}

	// Fetch TMDB metadata
	if *skipTMDB {
		log.Println("Skipping TMDB fetch (--skip-tmdb)")
	} else {
		tmdbKey := os.Getenv("TMDB_API_KEY")
		if tmdbKey == "" {
			log.Println("TMDB: skipped (no TMDB_API_KEY in .env or environment)")
		} else {
			client := tmdb.NewClient(tmdbKey)
			fetchTMDB(database, tmdbKey)

			// Refresh upcoming episodes cache
			log.Println("Refreshing upcoming episodes cache...")
			worker.RefreshUpcomingCache(database, client)
		}
	}

	log.Println("Import complete!")
}

func fetchTMDB(database *db.DB, apiKey string) {
	client := tmdb.NewClient(apiKey)

	// Fetch shows
	shows, err := database.GetShowsWithoutTMDB()
	if err != nil {
		log.Printf("TMDB: error getting shows: %v", err)
		return
	}

	if len(shows) == 0 {
		log.Println("TMDB: all shows already have metadata")
	} else {
		log.Printf("TMDB FETCH: starting %d shows...", len(shows))
		fetched := 0
		for i, show := range shows {
			result, err := client.FindTVByName(show.Name)
			if err != nil {
				log.Printf("TMDB [%d/%d] ✗ %q: %v", i+1, len(shows), show.Name, err)
				continue
			}

			genres := genreNames(result.Genres)
			posterURL := tmdb.PosterURL(result.PosterPath, "w342")
			backdropURL := tmdb.BackdropURL(result.BackdropPath, "w780")

			database.UpdateShowTMDB(show.ID, result.ID, posterURL, backdropURL, result.Overview, genres, result.Status, len(result.Seasons))
			fetched++
			log.Printf("TMDB [%d/%d] ✓ %q → tmdb_id=%d, %d seasons, %s", i+1, len(shows), show.Name, result.ID, len(result.Seasons), result.Status)
		}
		log.Printf("TMDB FETCH: shows done — %d/%d", fetched, len(shows))
	}

	// Fetch movies
	movies, err := database.GetMoviesWithoutTMDB()
	if err != nil {
		log.Printf("TMDB: error getting movies: %v", err)
		return
	}

	if len(movies) == 0 {
		log.Println("TMDB: all movies already have metadata")
	} else {
		log.Printf("TMDB FETCH: starting %d movies...", len(movies))
		fetched := 0
		for i, movie := range movies {
			results, err := client.SearchMovie(movie.Name)
			if err != nil || len(results) == 0 {
				log.Printf("TMDB [%d/%d] ✗ movie %q: no results", i+1, len(movies), movie.Name)
				continue
			}
			detail, err := client.GetMovie(results[0].ID)
			if err != nil {
				log.Printf("TMDB [%d/%d] ✗ movie %q: %v", i+1, len(movies), movie.Name, err)
				continue
			}
			genres := genreNames(detail.Genres)
			posterURL := tmdb.PosterURL(detail.PosterPath, "w342")
			database.UpdateMovieTMDB(movie.ID, detail.ID, posterURL, detail.Overview, genres, detail.Runtime)
			fetched++
			log.Printf("TMDB [%d/%d] ✓ movie %q → tmdb_id=%d", i+1, len(movies), movie.Name, detail.ID)
		}
		log.Printf("TMDB FETCH: movies done — %d/%d", fetched, len(movies))
	}
}

func genreNames(genres []tmdb.Genre) string {
	s := ""
	for i, g := range genres {
		if i > 0 {
			s += ", "
		}
		s += g.Name
	}
	return s
}
