package importer

import (
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mdaguete/watchlog/internal/db"
)

// NetflixChange is a single proposed watched-date correction.
type NetflixChange struct {
	Type         string `json:"type"` // "episode" | "movie"
	ID           int64  `json:"id"`   // show id or movie id
	Title        string `json:"title"`
	Season       int    `json:"season"`
	Episode      int    `json:"episode"`
	NetflixTitle string `json:"netflix_title"`
	CurrentDate  string `json:"current_date"` // YYYY-MM-DD
	NewDate      string `json:"new_date"`     // YYYY-MM-DD
}

// NetflixAnalysis is the result of analyzing a Netflix history against the DB.
type NetflixAnalysis struct {
	Changes          []NetflixChange
	UnmatchedSeries  []string
	UnmatchedEntries []NetflixEntry
	Entries          int
	SeriesMatched    int
}

var netflixSeasonRe = regexp.MustCompile(`(?i)(?:temporada|season|parte|part|volumen|volume)\s+(\d+)`)

// AnalyzeNetflix parses a Netflix ViewingHistory.csv and returns the watched-date
// changes that would bring the DB in line with Netflix, matching series by name
// and episodes by title or chronological order, plus movies by title.
// NetflixEntry is a parsed row from a Netflix viewing-history CSV.
type NetflixEntry struct {
	Series  string
	Season  int
	EpTitle string
	IsMovie bool
	Date    time.Time
}

// ParseNetflixCSV parses a Netflix ViewingHistory.csv into structured entries.
func ParseNetflixCSV(r io.Reader) ([]NetflixEntry, error) {
	cr := csv.NewReader(r)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, nil
	}
	var entries []NetflixEntry
	for _, rec := range records[1:] {
		if len(rec) < 2 {
			continue
		}
		title := strings.TrimSpace(rec[0])
		t, err := parseNetflixDate(strings.TrimSpace(rec[1]))
		if err != nil {
			continue
		}
		parts := strings.SplitN(title, ": ", 3)
		if len(parts) >= 3 {
			m := netflixSeasonRe.FindStringSubmatch(strings.TrimSpace(parts[1]))
			if m == nil {
				continue
			}
			season, _ := strconv.Atoi(m[1])
			entries = append(entries, NetflixEntry{Series: strings.TrimSpace(parts[0]), Season: season, EpTitle: strings.TrimSpace(parts[2]), Date: t})
		} else if len(parts) == 1 {
			entries = append(entries, NetflixEntry{Series: strings.TrimSpace(parts[0]), IsMovie: true, Date: t})
		}
	}
	return entries, nil
}

// AnalyzeNetflix parses a Netflix ViewingHistory.csv and returns the watched-date
// changes that would bring the DB in line with Netflix, matching series by name
// and episodes by title or chronological order, plus movies by title.
func AnalyzeNetflix(database *db.DB, userID int64, r io.Reader) (*NetflixAnalysis, error) {
	entries, err := ParseNetflixCSV(r)
	if err != nil {
		return nil, err
	}
	return Analyze(database, userID, entries), nil
}

// Analyze matches parsed entries against the library and returns proposed
// changes plus the entries whose series/movie was not found.
func Analyze(database *db.DB, userID int64, entries []NetflixEntry) *NetflixAnalysis {
	res := &NetflixAnalysis{Entries: len(entries)}

	// --- Shows index ---
	showIndex := map[string]int64{}
	showDisplay := map[int64]string{}
	if rows, err := database.RawQuery("SELECT id, name, COALESCE(name_es,''), COALESCE(name_en,'') FROM shows"); err == nil {
		for rows.Next() {
			var id int64
			var name, es, en string
			rows.Scan(&id, &name, &es, &en)
			disp := es
			if disp == "" {
				disp = name
			}
			showDisplay[id] = disp
			for _, n := range []string{name, es, en} {
				if nn := normalizeTitle(n); nn != "" {
					if _, ok := showIndex[nn]; !ok {
						showIndex[nn] = id
					}
				}
			}
		}
		rows.Close()
	}

	// --- Episode name index: show -> season -> normalized(name) -> ep number ---
	epNames := map[int64]map[int]map[string]int{}
	if rows, err := database.RawQuery("SELECT show_id, season_number, episode_number, name FROM episode_details"); err == nil {
		for rows.Next() {
			var sid int64
			var sn, en int
			var name string
			rows.Scan(&sid, &sn, &en, &name)
			if epNames[sid] == nil {
				epNames[sid] = map[int]map[string]int{}
			}
			if epNames[sid][sn] == nil {
				epNames[sid][sn] = map[string]int{}
			}
			epNames[sid][sn][normalizeTitle(name)] = en
		}
		rows.Close()
	}

	// --- Movies index (watched by user) ---
	movieIndex := map[string]int64{}
	movieDisplay := map[int64]string{}
	movieDate := map[int64]string{}
	if movies, err := database.GetUserMoviesSorted(userID, "name"); err == nil {
		for _, m := range movies {
			disp := m.NameES
			if disp == "" {
				disp = m.Name
			}
			movieDisplay[m.ID] = disp
			if !m.WatchedAt.IsZero() {
				movieDate[m.ID] = m.WatchedAt.Format("2006-01-02")
			}
			for _, n := range []string{m.Name, m.NameES, m.NameEN} {
				if nn := normalizeTitle(n); nn != "" {
					if _, ok := movieIndex[nn]; !ok {
						movieIndex[nn] = m.ID
					}
				}
			}
		}
	}

	// Chronological episode-order fallback per series+season.
	type gk struct {
		showID int64
		season int
	}
	groups := map[gk][]int{}
	for i, e := range entries {
		if e.IsMovie {
			continue
		}
		if sid, ok := showIndex[normalizeTitle(e.Series)]; ok {
			k := gk{sid, e.Season}
			groups[k] = append(groups[k], i)
		}
	}
	epOrder := map[int]int{}
	for _, idxs := range groups {
		for i, j := 0, len(idxs)-1; i < j; i, j = i+1, j-1 {
			idxs[i], idxs[j] = idxs[j], idxs[i] // CSV newest-first -> chrono
		}
		for n, idx := range idxs {
			epOrder[idx] = n + 1
		}
	}

	seenUnmatched := map[string]bool{}
	seriesMatched := map[int64]bool{}
	movieSeen := map[int64]int{}  // movie id -> index in res.Changes
	epSeen := map[string]int{}    // "sid-season-ep" -> index in res.Changes

	for i, e := range entries {
		if e.IsMovie {
			mid, ok := movieIndex[normalizeTitle(e.Series)]
			if !ok {
				res.UnmatchedEntries = append(res.UnmatchedEntries, e)
				continue
			}
			cur := movieDate[mid]
			if cur == "" {
				continue // not watched in DB
			}
			nd := e.Date.Format("2006-01-02")
			if cur == nd {
				continue
			}
			if idx, ok := movieSeen[mid]; ok {
				// Rewatch: keep the oldest date (first watch).
				if nd < res.Changes[idx].NewDate {
					res.Changes[idx].NewDate = nd
				}
				continue
			}
			res.Changes = append(res.Changes, NetflixChange{
				Type: "movie", ID: mid, Title: movieDisplay[mid],
				NetflixTitle: e.Series, CurrentDate: cur, NewDate: nd,
			})
			movieSeen[mid] = len(res.Changes) - 1
			continue
		}

		sid, ok := showIndex[normalizeTitle(e.Series)]
		if !ok {
			if !seenUnmatched[e.Series] {
				seenUnmatched[e.Series] = true
				res.UnmatchedSeries = append(res.UnmatchedSeries, e.Series)
			}
			res.UnmatchedEntries = append(res.UnmatchedEntries, e)
			continue
		}
		seriesMatched[sid] = true

		epNum := 0
		if sm, ok := epNames[sid]; ok {
			if nm, ok := sm[e.Season]; ok {
				epNum = nm[normalizeTitle(e.EpTitle)]
			}
		}
		if epNum == 0 {
			epNum = epOrder[i]
		}
		if epNum == 0 {
			continue
		}
		cur := database.GetEpisodeWatchedAt(userID, sid, e.Season, epNum)
		if cur == "" {
			continue // not watched
		}
		curDate := cur
		if len(cur) >= 10 {
			curDate = cur[:10]
		}
		nd := e.Date.Format("2006-01-02")
		if curDate == nd {
			continue
		}
		key := fmt.Sprintf("%d-%d-%d", sid, e.Season, epNum)
		if idx, ok := epSeen[key]; ok {
			if nd < res.Changes[idx].NewDate {
				res.Changes[idx].NewDate = nd
			}
			continue
		}
		res.Changes = append(res.Changes, NetflixChange{
			Type: "episode", ID: sid, Title: showDisplay[sid],
			Season: e.Season, Episode: epNum, NetflixTitle: e.EpTitle,
			CurrentDate: curDate, NewDate: nd,
		})
		epSeen[key] = len(res.Changes) - 1
	}
	res.SeriesMatched = len(seriesMatched)
	return res
}

// SeriesEpisodeMatch is a resolved episode with its (oldest) watched date.
type SeriesEpisodeMatch struct {
	Season       int
	Episode      int
	NetflixTitle string
	Date         time.Time // oldest watch date for this episode
}

// MatchSeriesEpisodes maps Netflix entries of a single (freshly added) show to
// episode numbers, using episode_details titles when available and falling back
// to chronological order within each season. When an episode appears multiple
// times (rewatch) the oldest date is kept. Entries must all belong to the same
// series; IsMovie entries are ignored.
func MatchSeriesEpisodes(database *db.DB, showID int64, entries []NetflixEntry) []SeriesEpisodeMatch {
	// Episode-name index for this show: season -> normalized(name) -> ep number.
	epNames := map[int]map[string]int{}
	if rows, err := database.RawQuery("SELECT season_number, episode_number, name FROM episode_details WHERE show_id = ?", showID); err == nil {
		for rows.Next() {
			var sn, en int
			var name string
			rows.Scan(&sn, &en, &name)
			if epNames[sn] == nil {
				epNames[sn] = map[string]int{}
			}
			epNames[sn][normalizeTitle(name)] = en
		}
		rows.Close()
	}

	// Chronological order fallback per season (Netflix CSV is newest-first).
	order := map[int]int{} // entry index -> episode number
	groups := map[int][]int{}
	for i, e := range entries {
		if e.IsMovie {
			continue
		}
		groups[e.Season] = append(groups[e.Season], i)
	}
	for _, idxs := range groups {
		for a, b := 0, len(idxs)-1; a < b; a, b = a+1, b-1 {
			idxs[a], idxs[b] = idxs[b], idxs[a]
		}
		for n, idx := range idxs {
			order[idx] = n + 1
		}
	}

	type key struct{ s, e int }
	best := map[key]SeriesEpisodeMatch{}
	var keys []key
	for i, e := range entries {
		if e.IsMovie {
			continue
		}
		epNum := 0
		if nm, ok := epNames[e.Season]; ok {
			epNum = nm[normalizeTitle(e.EpTitle)]
		}
		if epNum == 0 {
			epNum = order[i]
		}
		if epNum == 0 {
			continue
		}
		k := key{e.Season, epNum}
		if cur, ok := best[k]; ok {
			if e.Date.Before(cur.Date) {
				cur.Date = e.Date
				best[k] = cur
			}
			continue
		}
		best[k] = SeriesEpisodeMatch{Season: e.Season, Episode: epNum, NetflixTitle: e.EpTitle, Date: e.Date}
		keys = append(keys, k)
	}
	out := make([]SeriesEpisodeMatch, 0, len(keys))
	for _, k := range keys {
		out = append(out, best[k])
	}
	return out
}

// ApplyNetflixChange writes a single change's new date to the DB.
func ApplyNetflixChange(database *db.DB, userID int64, c NetflixChange) error {
	t, err := time.ParseInLocation("2006-01-02", c.NewDate, time.Local)
	if err != nil {
		return err
	}
	// Preserve a midday time so day-grouping stays stable across timezones.
	t = time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.Local)
	if c.Type == "movie" {
		return database.MarkMovieWatched(userID, c.ID, t)
	}
	_, err = database.UpdateEpisodeWatchedAt(userID, c.ID, c.Season, c.Episode, t)
	return err
}

func parseNetflixDate(s string) (time.Time, error) {
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
	return time.Date(year, time.Month(month), day, 12, 0, 0, 0, time.Local), nil
}

func normalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return -1
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
