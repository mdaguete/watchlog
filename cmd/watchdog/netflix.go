package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mdaguete/watchlog/internal/db"
)

// cmdNetflixDates reads a Netflix ViewingHistory.csv and adjusts watched_at
// dates in the database for episodes that match by series name + season +
// episode (name or order). --dry-run (default) only shows what would change.
func cmdNetflixDates(database *db.DB, csvPath string, userID int64, apply bool) {
	f, err := os.Open(csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
		os.Exit(1)
	}
	if len(records) < 2 {
		fmt.Println("CSV is empty")
		return
	}

	// Parse Netflix rows into structured entries.
	type entry struct {
		SeriesName string
		Season     int
		EpTitle    string
		Date       time.Time
	}
	var entries []entry
	// Netflix title patterns:
	// "SERIE: Temporada N: Ep title"  (series episode)
	// "SERIE: Season N: Ep title"
	// "SERIE: Parte N: Ep title"      (parts → treat as seasons)
	// "SERIE: Volumen N: Ep title"
	// "TITLE"                          (movie or single-season special)
	seasonRe := regexp.MustCompile(`(?i)(?:temporada|season|parte|part|volumen|volume)\s+(\d+)`)

	for _, rec := range records[1:] {
		if len(rec) < 2 {
			continue
		}
		title := strings.TrimSpace(rec[0])
		dateStr := strings.TrimSpace(rec[1])
		t, err := parseNetflixDate(dateStr)
		if err != nil {
			continue
		}
		parts := strings.SplitN(title, ": ", 3)
		if len(parts) < 3 {
			// Movie or single entry — skip for episode matching.
			continue
		}
		seriesName := strings.TrimSpace(parts[0])
		seasonPart := strings.TrimSpace(parts[1])
		epTitle := strings.TrimSpace(parts[2])
		m := seasonRe.FindStringSubmatch(seasonPart)
		if m == nil {
			// Maybe "SERIE: Ep title" (no season indicator) — try parts[1] as ep title.
			// Or the format doesn't match; skip.
			continue
		}
		season, _ := strconv.Atoi(m[1])
		entries = append(entries, entry{SeriesName: seriesName, Season: season, EpTitle: epTitle, Date: t})
	}

	fmt.Printf("Parsed %d episode entries from Netflix CSV\n", len(entries))
	if len(entries) == 0 {
		return
	}

	// Load all shows (with name_es/name_en for matching).
	type showInfo struct {
		ID     int64
		Name   string
		NameES string
		NameEN string
	}
	var shows []showInfo
	rows, err := database.RawQuery("SELECT id, name, COALESCE(name_es,''), COALESCE(name_en,'') FROM shows")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading shows: %v\n", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var s showInfo
		rows.Scan(&s.ID, &s.Name, &s.NameES, &s.NameEN)
		shows = append(shows, s)
	}

	// Build a name -> show_id index (case-insensitive, trimmed).
	showIndex := map[string]int64{}
	for _, s := range shows {
		for _, n := range []string{s.Name, s.NameES, s.NameEN} {
			n = normalizeTitle(n)
			if n != "" {
				if _, exists := showIndex[n]; !exists {
					showIndex[n] = s.ID
				}
			}
		}
	}

	// Load episode_details for name matching.
	type epKey struct {
		showID  int64
		season  int
		episode int
	}
	epNames := map[int64]map[int]map[string]int{} // show -> season -> normalized(name) -> episode_number
	detailRows, _ := database.RawQuery("SELECT show_id, season_number, episode_number, name FROM episode_details")
	if detailRows != nil {
		for detailRows.Next() {
			var sid int64
			var sn, en int
			var name string
			detailRows.Scan(&sid, &sn, &en, &name)
			if _, ok := epNames[sid]; !ok {
				epNames[sid] = map[int]map[string]int{}
			}
			if _, ok := epNames[sid][sn]; !ok {
				epNames[sid][sn] = map[string]int{}
			}
			epNames[sid][sn][normalizeTitle(name)] = en
		}
		detailRows.Close()
	}

	// Process entries: match and update.
	matched, updated, skipped, noMatch := 0, 0, 0, 0
	for _, e := range entries {
		showID, ok := showIndex[normalizeTitle(e.SeriesName)]
		if !ok {
			noMatch++
			continue
		}
		matched++

		// Try to find the episode number by title match.
		epNum := 0
		if seasonMap, ok := epNames[showID]; ok {
			if nameMap, ok := seasonMap[e.Season]; ok {
				if n, ok := nameMap[normalizeTitle(e.EpTitle)]; ok {
					epNum = n
				}
			}
		}
		if epNum == 0 {
			// Cannot determine episode number → skip (no fallback by order for safety).
			skipped++
			continue
		}

		// Check current watched_at.
		n, err := database.UpdateEpisodeWatchedAt(userID, showID, e.Season, epNum, e.Date)
		if err != nil || n == 0 {
			skipped++
			continue
		}
		if apply {
			updated++
			fmt.Printf("  ✓ %s S%02dE%02d → %s\n", e.SeriesName, e.Season, epNum, e.Date.Format("2006-01-02"))
		} else {
			updated++
			fmt.Printf("  [dry-run] %s S%02dE%02d → %s\n", e.SeriesName, e.Season, epNum, e.Date.Format("2006-01-02"))
		}
	}
	fmt.Printf("\nSummary: %d entries, %d series matched, %d episodes updated, %d skipped (no ep match), %d no series match\n", len(entries), matched, updated, skipped, noMatch)
	if !apply && updated > 0 {
		fmt.Println("\nRun with --apply to write changes to the database.")
	}
}

func parseNetflixDate(s string) (time.Time, error) {
	// Netflix date format: M/D/YY (US locale)
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid date: %q", s)
	}
	month, _ := strconv.Atoi(parts[0])
	day, _ := strconv.Atoi(parts[1])
	year, _ := strconv.Atoi(parts[2])
	if year < 100 {
		year += 2000
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("invalid date: %q", s)
	}
	return time.Date(year, time.Month(month), day, 20, 0, 0, 0, time.Local), nil
}

func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	// Remove common punctuation for fuzzy matching.
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return -1
	}, s)
	// Collapse spaces.
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}
