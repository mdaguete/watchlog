package main

import (
	"fmt"
	"os"
	"time"

	"github.com/mdaguete/watchlog/internal/db"
)

// cmdFillAired marks every already-aired episode of a show as watched, dated by
// its air date, for episodes the user hasn't got a watched row for. Useful when
// a show's episodes appear unwatched due to a season/episode numbering mismatch
// (e.g. TMDB "parts" vs TVTime numbering) even though it was fully watched.
// Existing watched episodes are never modified. Dry-run by default.
func cmdFillAired(database *db.DB, showID, userID int64, apply bool) {
	show, err := database.GetShow(showID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: show %d not found: %v\n", showID, err)
		os.Exit(1)
	}
	details, err := database.GetEpisodeDetails(showID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading episode details: %v\n", err)
		os.Exit(1)
	}
	if len(details) == 0 {
		fmt.Printf("Show %d (%q) has no episode details cached; run a TMDB refresh first.\n", showID, show.Name)
		return
	}

	today := time.Now()
	var toFill []db.EpisodeDetail
	for _, d := range details {
		if d.AirDate == "" {
			continue
		}
		air, err := time.ParseInLocation("2006-01-02", d.AirDate, time.Local)
		if err != nil || air.After(today) {
			continue // unknown or not yet aired
		}
		if database.GetEpisodeWatchedAt(userID, showID, d.SeasonNumber, d.EpisodeNumber) != "" {
			continue // already watched — leave its real date untouched
		}
		toFill = append(toFill, d)
	}

	fmt.Printf("Show %d: %q (user %d)\n", showID, show.Name, userID)
	fmt.Printf("%d aired episodes, %d to fill as watched by air date\n\n", len(details), len(toFill))

	if len(toFill) == 0 {
		fmt.Println("Nothing to do.")
		return
	}

	if apply {
		if bkp, err := database.Backup("fill-aired"); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating backup: %v\nAborting.\n", err)
			os.Exit(1)
		} else {
			fmt.Printf("Backup created: %s\n\n", bkp)
		}
		for _, d := range toFill {
			fmt.Printf("  \u2713 S%02dE%02d %q \u2192 %s\n", d.SeasonNumber, d.EpisodeNumber, d.Name, d.AirDate)
		}
		filled, err := database.FillAiredEpisodes(userID, showID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		database.SyncWatchStatsFromDB(userID)
		fmt.Printf("\nMarked %d episodes watched and recalculated stats.\n", filled)
		return
	}

	for _, d := range toFill {
		fmt.Printf("  S%02dE%02d %q \u2192 %s\n", d.SeasonNumber, d.EpisodeNumber, d.Name, d.AirDate)
	}
	fmt.Println("\nRun with --apply to write changes (a backup is created first).")
}
