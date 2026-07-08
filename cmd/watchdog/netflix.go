package main

import (
	"fmt"
	"os"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/importer"
)

// cmdNetflixDates reads a Netflix ViewingHistory.csv and adjusts watched_at
// dates in the database for episodes/movies that match by title (or episode
// order). Dry-run (default) only shows what would change; --apply writes them
// after creating a database backup.
func cmdNetflixDates(database *db.DB, csvPath string, userID int64, apply bool) {
	f, err := os.Open(csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	analysis, err := importer.AnalyzeNetflix(database, userID, f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing CSV: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parsed %d entries from Netflix CSV\n", analysis.Entries)

	if apply && len(analysis.Changes) > 0 {
		bkp, err := database.Backup("netflix-dates")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating backup: %v\nAborting.\n", err)
			os.Exit(1)
		}
		fmt.Printf("Backup created: %s\n\n", bkp)
	}

	applied := 0
	for _, c := range analysis.Changes {
		prefix := "  "
		if apply {
			if err := importer.ApplyNetflixChange(database, userID, c); err != nil {
				fmt.Fprintf(os.Stderr, "  ! failed %s: %v\n", c.Title, err)
				continue
			}
			prefix = "  \u2713 "
			applied++
		}
		if c.Type == "movie" {
			fmt.Printf("%s%s (movie) [%s]: %s \u2192 %s\n", prefix, c.Title, c.NetflixTitle, c.CurrentDate, c.NewDate)
		} else {
			fmt.Printf("%s%s S%02dE%02d [%s]: %s \u2192 %s\n", prefix, c.Title, c.Season, c.Episode, c.NetflixTitle, c.CurrentDate, c.NewDate)
		}
	}

	if len(analysis.UnmatchedSeries) > 0 {
		fmt.Printf("\n\u2014 Series not found in your library (%d):\n", len(analysis.UnmatchedSeries))
		for _, s := range analysis.UnmatchedSeries {
			fmt.Printf("  \u00b7 %s\n", s)
		}
	}

	fmt.Printf("\nSummary: %d entries, %d series matched, %d changes, %d series not found\n",
		analysis.Entries, analysis.SeriesMatched, len(analysis.Changes), len(analysis.UnmatchedSeries))
	if !apply && len(analysis.Changes) > 0 {
		fmt.Println("\nRun with --apply to write changes to the database.")
	} else if apply {
		fmt.Printf("\nApplied %d changes.\n", applied)
	}
}
