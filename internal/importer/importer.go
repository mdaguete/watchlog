package importer

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/mdaguete/watchlog/internal/models"
)

type Importer struct {
	db           *db.DB
	dataDir      string
	userID       int64
	lastImported int
	LogFunc      func(string)
}

func New(database *db.DB, dataDir string, userID int64) *Importer {
	return &Importer{db: database, dataDir: dataDir, userID: userID, LogFunc: func(s string) { log.Println(s) }}
}

func (imp *Importer) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	imp.LogFunc(msg)
}

func (imp *Importer) ImportAll() error {
	start := time.Now()
	imp.logf("═══════════════════════════════════════")
	imp.logf("  WatchLog Import - Starting")
	imp.logf("═══════════════════════════════════════")

	steps := []struct {
		name string
		fn   func() error
	}{
		{"shows (followed_tv_show)", imp.importFollowedShows},
		{"show data (user_tv_show_data)", imp.importShowData},
		{"episodes (tracking-prod-records-v2)", imp.importEpisodes},
		{"rewatched episodes", imp.importRewatchedEpisodes},
		{"movies (ratings-live-votes)", imp.importMovies},
		{"show progress (show_seen_episode_latest)", imp.importShowProgress},
		{"watch stats (tracking-prod-count-by-timeframe)", imp.importWatchStats},
		{"lists (lists-prod-lists)", imp.importLists},
	}

	for i, step := range steps {
		stepStart := time.Now()
		imp.logf("[%d/%d] Importing %s...", i+1, len(steps), step.name)
		if err := step.fn(); err != nil {
			imp.logf("  ⚠ WARNING: %s: %v", step.name, err)
		}
		imp.logf("  → done in %s", time.Since(stepStart).Round(time.Millisecond))
	}

	imp.logf("═══════════════════════════════════════")
	imp.logf("  Import complete in %s", time.Since(start).Round(time.Millisecond))
	imp.logf("═══════════════════════════════════════")
	return nil
}

func (imp *Importer) openCSV(filename string) (*csv.Reader, *os.File, error) {
	path := filepath.Join(imp.dataDir, filename)
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	r := csv.NewReader(f)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1 // allow variable fields
	return r, f, nil
}

func (imp *Importer) importFollowedShows() error {
	r, f, err := imp.openCSV("followed_tv_show.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	header, err := r.Read()
	if err != nil {
		return err
	}
	idx := indexHeader(header)

	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		extID, _ := strconv.ParseInt(getField(record, idx, "tv_show_id"), 10, 64)
		active, _ := strconv.Atoi(getField(record, idx, "active"))
		archived, _ := strconv.Atoi(getField(record, idx, "archived"))
		followedAt := parseTime(getField(record, idx, "created_at"))

		showID, err := imp.db.UpsertShow(models.Show{
			ExternalID: extID,
			Name:       getField(record, idx, "tv_show_name"),
		})
		if err != nil {
			imp.logf("  show %s: %v", getField(record, idx, "tv_show_name"), err)
			continue
		}

		imp.db.UpsertUserShow(imp.userID, showID, active == 1, false, archived == 1, 0, followedAt)
		count++
	}
	imp.logf("  Imported %d shows", count)
	return nil
}

func (imp *Importer) importShowData() error {
	r, f, err := imp.openCSV("user_tv_show_data.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	header, err := r.Read()
	if err != nil {
		return err
	}
	idx := indexHeader(header)

	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		extID, _ := strconv.ParseInt(getField(record, idx, "tv_show_id"), 10, 64)
		episodesSeen, _ := strconv.Atoi(getField(record, idx, "nb_episodes_seen"))
		isFav := getField(record, idx, "is_favorited") == "1"
		isFollowed := getField(record, idx, "is_followed") == "1"

		showID, err := imp.db.UpsertShow(models.Show{
			ExternalID: extID,
			Name:       getField(record, idx, "tv_show_name"),
		})
		if err != nil || showID == 0 {
			continue
		}

		imp.db.UpsertUserShow(imp.userID, showID, isFollowed, isFav, false, episodesSeen, time.Time{})
		count++
	}
	imp.logf("  Updated %d shows with episode counts", count)
	return nil
}

func (imp *Importer) importEpisodes() error {
	// Primary source: tracking-prod-records.csv (individual watches with full metadata)
	count := 0
	if err := imp.importEpisodesFromV1(); err != nil {
		imp.logf("  tracking-prod-records.csv: %v", err)
	} else {
		count += imp.lastImported
	}

	// Secondary source: tracking-prod-records-v2.csv (watch-episode records)
	if err := imp.importEpisodesFromV2(); err != nil {
		imp.logf("  tracking-prod-records-v2.csv: %v", err)
	} else {
		count += imp.lastImported
	}

	imp.logf("  Imported %d episode watches total", count)
	return nil
}

func (imp *Importer) importEpisodesFromV1() error {
	r, f, err := imp.openCSV("tracking-prod-records.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	header, err := r.Read()
	if err != nil {
		return err
	}
	idx := indexHeader(header)

	count := 0
	skipped := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Only type=watch, entity_type=episode
		if getField(record, idx, "type") != "watch" {
			continue
		}
		if getField(record, idx, "entity_type") != "episode" {
			continue
		}

		seriesName := getField(record, idx, "series_name")
		if seriesName == "" {
			continue
		}

		extShowID, _ := strconv.ParseInt(getField(record, idx, "series_id"), 10, 64)
		epExtID, _ := strconv.ParseInt(getField(record, idx, "episode_id"), 10, 64)
		seasonNum, _ := strconv.Atoi(getField(record, idx, "season_number"))
		epNum, _ := strconv.Atoi(getField(record, idx, "episode_number"))
		runtime, _ := strconv.Atoi(getField(record, idx, "runtime"))

		// watch_date is a unix timestamp
		watchDateStr := getField(record, idx, "watch_date")
		var watchedAt time.Time
		if ts, err := strconv.ParseInt(watchDateStr, 10, 64); err == nil && ts > 0 {
			watchedAt = time.Unix(ts, 0)
		} else {
			watchedAt = parseTime(getField(record, idx, "created_at"))
		}

		if extShowID == 0 || epExtID == 0 {
			skipped++
			continue
		}

		showID, _ := imp.db.UpsertShow(models.Show{
			ExternalID: extShowID,
			Name:       seriesName,
		})
		if showID == 0 {
			continue
		}

		err = imp.db.InsertEpisode(models.Episode{
			UserID:        imp.userID,
			ExternalID:    epExtID,
			ShowID:        showID,
			SeasonNumber:  seasonNum,
			EpisodeNumber: epNum,
			Watched:       true,
			WatchedAt:     watchedAt,
			Runtime:       runtime / 60,
		})
		if err != nil {
			skipped++
			continue
		}
		count++
	}
	imp.logf("  tracking-prod-records.csv: %d episodes imported, %d skipped/duplicates", count, skipped)
	imp.lastImported = count
	return nil
}

func (imp *Importer) importEpisodesFromV2() error {
	r, f, err := imp.openCSV("tracking-prod-records-v2.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	header, err := r.Read()
	if err != nil {
		return err
	}
	idx := indexHeader(header)

	count := 0
	skipped := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Only watch-episode records
		key := getField(record, idx, "key")
		if !strings.HasPrefix(key, "watch-episode-") {
			continue
		}

		seriesName := getField(record, idx, "series_name")
		if seriesName == "" {
			continue
		}

		extShowID, _ := strconv.ParseInt(getField(record, idx, "s_id"), 10, 64)
		epExtID, _ := strconv.ParseInt(getField(record, idx, "episode_id"), 10, 64)
		seasonNum, _ := strconv.Atoi(getField(record, idx, "s_no"))
		epNum, _ := strconv.Atoi(getField(record, idx, "ep_no"))
		runtime, _ := strconv.Atoi(getField(record, idx, "runtime"))
		watchedAt := parseTime(getField(record, idx, "created_at"))

		if extShowID == 0 || epExtID == 0 || seasonNum == 0 || epNum == 0 {
			continue
		}

		// Only import if the show already exists in DB (avoid creating phantom shows from wrong IDs)
		showID, _ := imp.db.UpsertShow(models.Show{
			ExternalID: extShowID,
			Name:       seriesName,
		})
		if showID == 0 {
			continue
		}

		err = imp.db.InsertEpisode(models.Episode{
			UserID:        imp.userID,
			ExternalID:    epExtID,
			ShowID:        showID,
			SeasonNumber:  seasonNum,
			EpisodeNumber: epNum,
			Watched:       true,
			WatchedAt:     watchedAt,
			Runtime:       runtime / 60,
		})
		if err != nil {
			skipped++
			continue
		}
		count++
	}
	imp.logf("  tracking-prod-records-v2.csv: %d episodes imported, %d skipped/duplicates", count, skipped)
	imp.lastImported = count
	return nil
}

func (imp *Importer) importRewatchedEpisodes() error {
	r, f, err := imp.openCSV("rewatched_episode.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	header, err := r.Read()
	if err != nil {
		return err
	}
	idx := indexHeader(header)

	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		seriesName := getField(record, idx, "tv_show_name")
		epExtID, _ := strconv.ParseInt(getField(record, idx, "episode_id"), 10, 64)
		seasonNum, _ := strconv.Atoi(getField(record, idx, "episode_season_number"))
		epNum, _ := strconv.Atoi(getField(record, idx, "episode_number"))
		watchedAt := parseTime(getField(record, idx, "created_at"))

		// Try to find the show by name - use 0 as external ID since we don't have it here
		showID, _ := imp.db.UpsertShow(models.Show{
			ExternalID: epExtID, // use episode ID as temp external to avoid collision
			Name:       seriesName,
		})
		if showID == 0 {
			continue
		}

		err = imp.db.InsertEpisode(models.Episode{
			UserID:        imp.userID,
			ExternalID:    epExtID,
			ShowID:        showID,
			SeasonNumber:  seasonNum,
			EpisodeNumber: epNum,
			Watched:       true,
			WatchedAt:     watchedAt,
		})
		if err != nil {
			continue
		}
		count++
	}
	imp.logf("  Imported %d rewatched episodes", count)
	return nil
}

func (imp *Importer) importMovies() error {
	// Source 1: tracking-prod-records.csv (type=watch, entity_type=movie) — complete watch history
	count := 0
	r, f, err := imp.openCSV("tracking-prod-records.csv")
	if err == nil {
		defer f.Close()
		header, _ := r.Read()
		idx := indexHeader(header)

		for {
			record, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			if getField(record, idx, "type") != "watch" || getField(record, idx, "entity_type") != "movie" {
				continue
			}

			movieName := getField(record, idx, "movie_name")
			if movieName == "" {
				continue
			}

			uuid := getField(record, idx, "uuid")
			watchedAt := parseTime(getField(record, idx, "created_at"))
			releaseDate := getField(record, idx, "release_date")
			if len(releaseDate) > 10 {
				releaseDate = releaseDate[:10]
			}

			movieID, err := imp.db.UpsertMovie(models.Movie{
				ExternalID:  uuid,
				Name:        movieName,
				ReleaseDate: releaseDate,
			})
			if err != nil || movieID == 0 {
				continue
			}
			imp.db.MarkMovieWatched(imp.userID, movieID, watchedAt)
			count++
		}
		imp.logf("  tracking-prod-records.csv: %d movies imported", count)
	}

	// Source 2: ratings-live-votes.csv — adds ratings to movies
	r2, f2, err := imp.openCSV("ratings-live-votes.csv")
	if err != nil {
		imp.logf("  Imported %d movies total", count)
		return nil
	}
	defer f2.Close()

	header2, err := r2.Read()
	if err != nil {
		return nil
	}
	idx2 := indexHeader(header2)

	ratingsCount := 0
	for {
		record, err := r2.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		movieName := getField(record, idx2, "movie_name")
		if movieName == "" {
			continue
		}

		uuid := getField(record, idx2, "uuid")
		voteKey := getField(record, idx2, "vote_key")
		_ = extractRating(voteKey)

		movieID, err := imp.db.UpsertMovie(models.Movie{
			ExternalID: uuid,
			Name:       movieName,
		})
		if err != nil || movieID == 0 {
			continue
		}
		imp.db.MarkMovieWatched(imp.userID, movieID, time.Time{})
		ratingsCount++
	}
	imp.logf("  ratings-live-votes.csv: %d movie ratings imported", ratingsCount)
	imp.logf("  Imported %d movies total", count+ratingsCount)
	return nil
}

func (imp *Importer) importShowProgress() error {
	r, f, err := imp.openCSV("show_seen_episode_latest.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	header, err := r.Read()
	if err != nil {
		return err
	}
	idx := indexHeader(header)

	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		extShowID, _ := strconv.ParseInt(getField(record, idx, "tv_show_id"), 10, 64)
		epID, _ := strconv.ParseInt(getField(record, idx, "episode_id"), 10, 64)
		updatedAt := parseTime(getField(record, idx, "updated_at"))

		// Get show internal ID
		showID, _ := imp.db.UpsertShow(models.Show{
			ExternalID: extShowID,
			Name:       getField(record, idx, "tv_show_name"),
		})
		if showID == 0 {
			continue
		}

		err = imp.db.UpsertShowProgress(imp.userID, models.ShowProgress{
			ShowID:        showID,
			ShowName:      getField(record, idx, "tv_show_name"),
			LastEpisodeID: epID,
			UpdatedAt:     updatedAt,
		})
		if err != nil {
			imp.logf("  show_progress error for %q (showID=%d): %v", getField(record, idx, "tv_show_name"), showID, err)
			continue
		}
		count++
	}
	imp.logf("  Imported %d show progress records", count)
	return nil
}

func (imp *Importer) importWatchStats() error {
	// First try to import from CSV (TVTime export baseline)
	r, f, err := imp.openCSV("tracking-prod-count-by-timeframe.csv")
	if err == nil {
		defer f.Close()
		header, err := r.Read()
		if err == nil {
			idx := indexHeader(header)
			count := 0
			for {
				record, err := r.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					continue
				}
				period := getField(record, idx, "type")
				epCount, _ := strconv.Atoi(getField(record, idx, "count"))
				runtime, _ := strconv.Atoi(getField(record, idx, "runtime"))
				err = imp.db.UpsertWatchStats(imp.userID, models.WatchStats{
					Period:  period,
					Count:   epCount,
					Runtime: runtime / 60,
				})
				if err != nil {
					continue
				}
				count++
			}
			imp.logf("  Imported %d watch stat periods from CSV", count)
		}
	}

	// Then recalculate from actual DB data (episodes + movies watched_at)
	// This ensures months not covered by the CSV (e.g. 2026) are included
	if err := imp.db.RecalcWatchStats(imp.userID); err != nil {
		return err
	}
	imp.logf("  Recalculated watch stats from DB")
	return nil
}

func (imp *Importer) importLists() error {
	r, f, err := imp.openCSV("lists-prod-lists.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	header, err := r.Read()
	if err != nil {
		return err
	}
	idx := indexHeader(header)

	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		name := getField(record, idx, "name")
		if name == "" {
			continue
		}

		isPublic := getField(record, idx, "is_public") == "true"

		listID, err := imp.db.CreateList(imp.userID, name, isPublic)
		if err != nil || listID == 0 {
			continue
		}
		count++

		// The objects field contains serialized data - extract show IDs
		objects := getField(record, idx, "objects")
		if objects != "" {
			imp.parseListObjects(listID, objects)
		}
	}
	imp.logf("  Imported %d lists", count)
	return nil
}

func (imp *Importer) parseListObjects(listID int64, objects string) {
	// Objects are in format: [map[created_at:... id:417271 type:series uuid:...] ...]
	// Simple extraction of id and type fields
	parts := strings.Split(objects, "map[")
	for _, part := range parts {
		if !strings.Contains(part, "id:") {
			continue
		}
		var entityType string
		var entityID int64

		fields := strings.Fields(part)
		for _, field := range fields {
			if strings.HasPrefix(field, "type:") {
				entityType = strings.TrimPrefix(field, "type:")
				entityType = strings.TrimSuffix(entityType, "]")
			}
			if strings.HasPrefix(field, "id:") {
				idStr := strings.TrimPrefix(field, "id:")
				idStr = strings.TrimSuffix(idStr, "]")
				entityID, _ = strconv.ParseInt(idStr, 10, 64)
			}
		}

		if entityID > 0 && entityType != "" {
			imp.db.AddListItem(models.ListItem{
				ListID:     listID,
				EntityType: entityType,
				EntityID:   entityID,
			})
		}
	}
}

// --- Helpers ---

func indexHeader(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}
	return idx
}

func getField(record []string, idx map[string]int, field string) string {
	i, ok := idx[field]
	if !ok || i >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[i])
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

// extractRating parses the rating from vote_key format like "uuid-userID-SCORE"
func extractRating(voteKey string) int {
	parts := strings.Split(voteKey, "-")
	if len(parts) == 0 {
		return 0
	}
	last := parts[len(parts)-1]
	rating, err := strconv.Atoi(last)
	if err != nil {
		return 0
	}
	return rating
}

// FormatRuntime formats minutes into a readable string
func FormatRuntime(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	days := minutes / (24 * 60)
	hours := (minutes % (24 * 60)) / 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes%60)
}
