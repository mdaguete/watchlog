package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mdaguete/watchlog/internal/models"
)

type DB struct {
	conn *sql.DB
	path string
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(1)
	db := &DB{conn: conn, path: path}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	return db.runMigrations()
}

// --- Users ---

func (db *DB) HasUsers() bool {
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count > 0
}

func (db *DB) CreateUser(username, passwordHash string) (int64, error) {
	res, err := db.conn.Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, passwordHash)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) GetUserByUsername(username string) (models.User, error) {
	var u models.User
	err := db.conn.QueryRow("SELECT id, username, email, password_hash, created_at FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, err
}

func (db *DB) GetUserByID(id int64) (models.User, error) {
	var u models.User
	err := db.conn.QueryRow("SELECT id, username, email, password_hash, created_at FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, err
}

func (db *DB) GetUserLang(userID int64) string {
	var lang string
	err := db.conn.QueryRow("SELECT lang FROM users WHERE id = ?", userID).Scan(&lang)
	if err != nil || lang == "" {
		return "es"
	}
	return lang
}

func (db *DB) SetUserLang(userID int64, lang string) error {
	_, err := db.conn.Exec("UPDATE users SET lang = ? WHERE id = ?", lang, userID)
	return err
}

func (db *DB) GetUserTheme(userID int64) string {
	var theme string
	err := db.conn.QueryRow("SELECT theme FROM users WHERE id = ?", userID).Scan(&theme)
	if err != nil || theme == "" {
		return "system"
	}
	return theme
}

func (db *DB) SetUserTheme(userID int64, theme string) error {
	_, err := db.conn.Exec("UPDATE users SET theme = ? WHERE id = ?", theme, userID)
	return err
}

// --- Settings (key-value store) ---

func (db *DB) GetSetting(key string) string {
	var value string
	db.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value
}

func (db *DB) SetSetting(key, value string) error {
	_, err := db.conn.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// --- Shows (shared catalog) ---

func (db *DB) UpsertShow(s models.Show) (int64, error) {
	_, err := db.conn.Exec(`
		INSERT INTO shows (external_id, name, tmdb_id, poster_url, backdrop_url, overview, genres, status, total_seasons)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(external_id) DO UPDATE SET
			name=COALESCE(NULLIF(excluded.name, ''), shows.name),
			tmdb_id=MAX(shows.tmdb_id, excluded.tmdb_id),
			poster_url=COALESCE(NULLIF(excluded.poster_url, ''), shows.poster_url),
			backdrop_url=COALESCE(NULLIF(excluded.backdrop_url, ''), shows.backdrop_url),
			overview=COALESCE(NULLIF(excluded.overview, ''), shows.overview),
			genres=COALESCE(NULLIF(excluded.genres, ''), shows.genres),
			status=COALESCE(NULLIF(excluded.status, ''), shows.status),
			total_seasons=MAX(shows.total_seasons, excluded.total_seasons)`,
		s.ExternalID, s.Name, s.TMDBID, s.PosterURL, s.BackdropURL, s.Overview, s.Genres, s.Status, s.TotalSeasons)
	if err != nil {
		return 0, err
	}
	var id int64
	db.conn.QueryRow("SELECT id FROM shows WHERE external_id = ?", s.ExternalID).Scan(&id)
	return id, nil
}

func (db *DB) GetShow(id int64) (models.Show, error) {
	var s models.Show
	err := db.conn.QueryRow("SELECT id, external_id, name, name_es, name_en, tmdb_id, poster_url, backdrop_url, overview, overview_en, genres, genres_en, status, total_seasons FROM shows WHERE id = ?", id).
		Scan(&s.ID, &s.ExternalID, &s.Name, &s.NameES, &s.NameEN, &s.TMDBID, &s.PosterURL, &s.BackdropURL, &s.Overview, &s.OverviewEN, &s.Genres, &s.GenresEN, &s.Status, &s.TotalSeasons)
	return s, err
}

func (db *DB) GetShowByTMDBID(tmdbID int, id *int64) {
	db.conn.QueryRow("SELECT id FROM shows WHERE tmdb_id = ?", tmdbID).Scan(id)
}

func (db *DB) UpdateShowTMDB(id int64, tmdbID int, posterURL, backdropURL, overview, genres, status string, totalSeasons int) error {
	_, err := db.conn.Exec(`UPDATE shows SET tmdb_id=?, poster_url=?, backdrop_url=?, overview=?, genres=?, status=?, total_seasons=? WHERE id=?`,
		tmdbID, posterURL, backdropURL, overview, genres, status, totalSeasons, id)
	return err
}

func (db *DB) UpdateShowTMDBNames(id int64, nameES, nameEN string) error {
	_, err := db.conn.Exec(`UPDATE shows SET name_es=?, name_en=? WHERE id=?`, nameES, nameEN, id)
	return err
}

func (db *DB) UpdateShowTMDBEN(id int64, overviewEN, genresEN string) error {
	_, err := db.conn.Exec(`UPDATE shows SET overview_en=?, genres_en=? WHERE id=?`, overviewEN, genresEN, id)
	return err
}

func (db *DB) GetShowsWithoutTMDB() ([]models.Show, error) {
	rows, err := db.conn.Query("SELECT id, external_id, name, tmdb_id, poster_url, backdrop_url, overview, genres, status, total_seasons FROM shows WHERE tmdb_id = 0 ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shows []models.Show
	for rows.Next() {
		var s models.Show
		if err := rows.Scan(&s.ID, &s.ExternalID, &s.Name, &s.TMDBID, &s.PosterURL, &s.BackdropURL, &s.Overview, &s.Genres, &s.Status, &s.TotalSeasons); err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, nil
}

func (db *DB) AddShowFromTMDB(tmdbID int, name, posterURL, backdropURL, overview, genres, status string, totalSeasons int) (int64, error) {
	_, err := db.conn.Exec(`
		INSERT INTO shows (external_id, name, tmdb_id, poster_url, backdrop_url, overview, genres, status, total_seasons)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(external_id) DO UPDATE SET
			tmdb_id=excluded.tmdb_id, poster_url=excluded.poster_url, backdrop_url=excluded.backdrop_url,
			overview=excluded.overview, genres=excluded.genres, status=excluded.status, total_seasons=excluded.total_seasons`,
		tmdbID, name, tmdbID, posterURL, backdropURL, overview, genres, status, totalSeasons)
	if err != nil {
		return 0, err
	}
	var id int64
	db.conn.QueryRow("SELECT id FROM shows WHERE external_id = ?", tmdbID).Scan(&id)
	return id, nil
}

// --- User Shows (per-user tracking) ---

func (db *DB) UpsertUserShow(userID, showID int64, isFollowed, isFavorited, isArchived bool, episodesSeen int, followedAt time.Time) error {
	_, err := db.conn.Exec(`
		INSERT INTO user_shows (user_id, show_id, is_followed, is_favorited, is_archived, episodes_seen, followed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, show_id) DO UPDATE SET
			is_followed=MAX(user_shows.is_followed, excluded.is_followed),
			is_favorited=MAX(user_shows.is_favorited, excluded.is_favorited),
			is_archived=MAX(user_shows.is_archived, excluded.is_archived),
			episodes_seen=MAX(user_shows.episodes_seen, excluded.episodes_seen),
			updated_at=COALESCE(NULLIF(excluded.updated_at, ''), user_shows.updated_at)`,
		userID, showID, isFollowed, isFavorited, isArchived, episodesSeen, followedAt, time.Now())
	return err
}

func (db *DB) GetUserShowsSorted(userID int64, sort string) ([]models.UserShow, error) {
	return db.GetUserShowsFiltered(userID, sort, "")
}

func (db *DB) GetUserShowsFiltered(userID int64, sort, filter string) ([]models.UserShow, error) {
	var orderClause string
	var joinClause string
	switch sort {
	case "name":
		orderClause = "ORDER BY s.name ASC"
	case "episodes":
		orderClause = "ORDER BY us.episodes_seen DESC, s.name ASC"
	case "followed":
		orderClause = "ORDER BY us.followed_at DESC, s.name ASC"
	default: // recent — last episode watched
		joinClause = `LEFT JOIN (SELECT show_id, MAX(watched_at) as last_watched FROM episodes WHERE user_id = ? GROUP BY show_id) ew ON ew.show_id = s.id`
		orderClause = `ORDER BY COALESCE(ew.last_watched, sp.updated_at, us.followed_at) DESC, s.name ASC`
	}

	var filterClause string
	switch filter {
	case "favorites":
		filterClause = "AND us.is_favorited = 1"
	case "archived":
		filterClause = "AND us.is_archived = 1"
	default:
		filterClause = "AND us.is_archived = 0" // hide archived by default
	}

	query := `SELECT s.id, s.external_id, s.name, s.name_es, s.name_en, s.tmdb_id, s.poster_url, s.backdrop_url, s.overview, s.genres, s.status, s.total_seasons,
		us.is_followed, us.is_favorited, us.is_archived, us.episodes_seen, us.followed_at, us.updated_at
		FROM user_shows us
		JOIN shows s ON s.id = us.show_id
		LEFT JOIN show_progress sp ON sp.show_id = s.id AND sp.user_id = us.user_id
		` + joinClause + `
		WHERE us.user_id = ? AND us.is_followed = 1 ` + filterClause + ` ` + orderClause

	var rows *sql.Rows
	var err error
	if sort == "recent" {
		rows, err = db.conn.Query(query, userID, userID)
	} else {
		rows, err = db.conn.Query(query, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shows []models.UserShow
	for rows.Next() {
		var us models.UserShow
		if err := rows.Scan(&us.ID, &us.ExternalID, &us.Name, &us.NameES, &us.NameEN, &us.TMDBID, &us.PosterURL, &us.BackdropURL, &us.Overview, &us.Genres, &us.Status, &us.TotalSeasons,
			&us.IsFollowed, &us.IsFavorited, &us.IsArchived, &us.EpisodesSeen, &us.FollowedAt, &us.UpdatedAt); err != nil {
			return nil, err
		}
		shows = append(shows, us)
	}
	return shows, nil
}

func (db *DB) GetUserShow(userID, showID int64) (models.UserShow, error) {
	var us models.UserShow
	err := db.conn.QueryRow(`SELECT s.id, s.external_id, s.name, s.name_es, s.name_en, s.tmdb_id, s.poster_url, s.backdrop_url, s.overview, s.overview_en, s.genres, s.genres_en, s.status, s.total_seasons,
		us.is_followed, us.is_favorited, us.is_archived, us.episodes_seen, us.followed_at, us.updated_at
		FROM user_shows us JOIN shows s ON s.id = us.show_id
		WHERE us.user_id = ? AND us.show_id = ?`, userID, showID).
		Scan(&us.ID, &us.ExternalID, &us.Name, &us.NameES, &us.NameEN, &us.TMDBID, &us.PosterURL, &us.BackdropURL, &us.Overview, &us.OverviewEN, &us.Genres, &us.GenresEN, &us.Status, &us.TotalSeasons,
			&us.IsFollowed, &us.IsFavorited, &us.IsArchived, &us.EpisodesSeen, &us.FollowedAt, &us.UpdatedAt)
	return us, err
}

func (db *DB) ToggleUserShowFollow(userID, showID int64) error {
	_, err := db.conn.Exec("UPDATE user_shows SET is_followed = NOT is_followed, updated_at = ? WHERE user_id = ? AND show_id = ?", time.Now(), userID, showID)
	return err
}

func (db *DB) ToggleUserShowFavorite(userID, showID int64) error {
	_, err := db.conn.Exec("UPDATE user_shows SET is_favorited = NOT is_favorited, updated_at = ? WHERE user_id = ? AND show_id = ?", time.Now(), userID, showID)
	return err
}

func (db *DB) ToggleUserShowArchive(userID, showID int64) error {
	_, err := db.conn.Exec("UPDATE user_shows SET is_archived = NOT is_archived, updated_at = ? WHERE user_id = ? AND show_id = ?", time.Now(), userID, showID)
	return err
}

func (db *DB) GetUserShowField(userID, showID int64, field string, dest *bool) {
	var val int
	db.conn.QueryRow("SELECT "+field+" FROM user_shows WHERE user_id = ? AND show_id = ?", userID, showID).Scan(&val)
	*dest = val == 1
}

// AutoArchiveIfComplete archives a show if it's ended/canceled and fully watched.
func (db *DB) AutoArchiveIfComplete(userID, showID int64) bool {
	// Check if show is ended or canceled
	var status string
	db.conn.QueryRow("SELECT status FROM shows WHERE id = ?", showID).Scan(&status)
	if status != "Ended" && status != "Canceled" {
		return false
	}

	// Get total episodes from season_episodes
	var totalEpisodes int
	db.conn.QueryRow("SELECT COALESCE(SUM(episode_count), 0) FROM season_episodes WHERE show_id = ?", showID).Scan(&totalEpisodes)
	if totalEpisodes == 0 {
		return false
	}

	// Count watched episodes
	var watchedCount int
	db.conn.QueryRow("SELECT COUNT(*) FROM episodes WHERE user_id = ? AND show_id = ?", userID, showID).Scan(&watchedCount)

	if watchedCount >= totalEpisodes {
		db.conn.Exec("UPDATE user_shows SET is_archived = 1, updated_at = ? WHERE user_id = ? AND show_id = ?", time.Now(), userID, showID)
		return true
	}
	return false
}

// AutoUnarchiveIfIncomplete unarchives a show if it was auto-archived but is no longer fully watched.
func (db *DB) AutoUnarchiveIfIncomplete(userID, showID int64) bool {
	// Only unarchive if currently archived
	var isArchived int
	db.conn.QueryRow("SELECT is_archived FROM user_shows WHERE user_id = ? AND show_id = ?", userID, showID).Scan(&isArchived)
	if isArchived != 1 {
		return false
	}

	// Check if show is ended or canceled (only auto-unarchive these)
	var status string
	db.conn.QueryRow("SELECT status FROM shows WHERE id = ?", showID).Scan(&status)
	if status != "Ended" && status != "Canceled" {
		return false
	}

	db.conn.Exec("UPDATE user_shows SET is_archived = 0, updated_at = ? WHERE user_id = ? AND show_id = ?", time.Now(), userID, showID)
	return true
}

// UnarchiveForNewSeason unarchives a show for all users who have it archived,
// if the show is "Returning Series" and has new seasons available.
func (db *DB) UnarchiveForNewSeason(showID int64, newSeasonCount int) {
	// Only act on shows that are returning
	var status string
	db.conn.QueryRow("SELECT status FROM shows WHERE id = ?", showID).Scan(&status)
	if status != "Returning Series" {
		return
	}
	// Get previously cached season count
	var oldCount int
	db.conn.QueryRow("SELECT COUNT(*) FROM season_episodes WHERE show_id = ?", showID).Scan(&oldCount)
	// If new seasons were added, unarchive for all users
	if newSeasonCount > oldCount {
		db.conn.Exec("UPDATE user_shows SET is_archived = 0, updated_at = ? WHERE show_id = ? AND is_archived = 1", time.Now(), showID)
	}
}

// SnoozeShow sets a snooze-until date for a show (hides from continue watching).
func (db *DB) SnoozeShow(userID, showID int64, until time.Time) error {
	_, err := db.conn.Exec("UPDATE user_shows SET snoozed_until = ? WHERE user_id = ? AND show_id = ?", until, userID, showID)
	return err
}

// UnsnoozeShow clears the snooze for a show.
func (db *DB) UnsnoozeShow(userID, showID int64) error {
	_, err := db.conn.Exec("UPDATE user_shows SET snoozed_until = NULL WHERE user_id = ? AND show_id = ?", userID, showID)
	return err
}

func (db *DB) FollowShow(userID, showID int64) error {
	_, err := db.conn.Exec(`INSERT INTO user_shows (user_id, show_id, is_followed, followed_at, updated_at) VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(user_id, show_id) DO UPDATE SET is_followed=1, updated_at=excluded.updated_at`,
		userID, showID, time.Now(), time.Now())
	return err
}

func (db *DB) SearchShows(query string) ([]models.Show, error) {
	rows, err := db.conn.Query("SELECT id, external_id, name, name_es, name_en, tmdb_id, poster_url, backdrop_url, overview, genres, status, total_seasons FROM shows WHERE name LIKE ? OR name_es LIKE ? OR name_en LIKE ? ORDER BY name LIMIT 20", "%"+query+"%", "%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shows []models.Show
	for rows.Next() {
		var s models.Show
		if err := rows.Scan(&s.ID, &s.ExternalID, &s.Name, &s.NameES, &s.NameEN, &s.TMDBID, &s.PosterURL, &s.BackdropURL, &s.Overview, &s.Genres, &s.Status, &s.TotalSeasons); err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, nil
}

func (db *DB) GetFollowedShowsWithTMDB(userID int64) ([]models.UserShow, error) {
	rows, err := db.conn.Query(`SELECT s.id, s.external_id, s.name, s.name_es, s.name_en, s.tmdb_id, s.poster_url, s.backdrop_url, s.overview, s.genres, s.status, s.total_seasons,
		us.is_followed, us.is_favorited, us.is_archived, us.episodes_seen, us.followed_at, us.updated_at
		FROM user_shows us JOIN shows s ON s.id = us.show_id
		WHERE us.user_id = ? AND us.is_followed = 1 AND s.tmdb_id > 0 AND s.status != 'Ended' AND s.status != 'Canceled'
		ORDER BY s.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shows []models.UserShow
	for rows.Next() {
		var us models.UserShow
		if err := rows.Scan(&us.ID, &us.ExternalID, &us.Name, &us.NameES, &us.NameEN, &us.TMDBID, &us.PosterURL, &us.BackdropURL, &us.Overview, &us.Genres, &us.Status, &us.TotalSeasons,
			&us.IsFollowed, &us.IsFavorited, &us.IsArchived, &us.EpisodesSeen, &us.FollowedAt, &us.UpdatedAt); err != nil {
			return nil, err
		}
		shows = append(shows, us)
	}
	return shows, nil
}

// --- Episodes (per-user) ---

func (db *DB) InsertEpisode(e models.Episode) error {
	_, err := db.conn.Exec(`
		INSERT OR IGNORE INTO episodes (user_id, external_id, show_id, season_number, episode_number, watched, watched_at, runtime)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.ExternalID, e.ShowID, e.SeasonNumber, e.EpisodeNumber, e.Watched, e.WatchedAt, e.Runtime)
	return err
}

func (db *DB) GetEpisodesByShow(userID, showID int64) ([]models.Episode, error) {
	rows, err := db.conn.Query("SELECT id, user_id, external_id, show_id, season_number, episode_number, watched, watched_at, runtime FROM episodes WHERE user_id = ? AND show_id = ? ORDER BY season_number, episode_number", userID, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var eps []models.Episode
	for rows.Next() {
		var e models.Episode
		if err := rows.Scan(&e.ID, &e.UserID, &e.ExternalID, &e.ShowID, &e.SeasonNumber, &e.EpisodeNumber, &e.Watched, &e.WatchedAt, &e.Runtime); err != nil {
			return nil, err
		}
		eps = append(eps, e)
	}
	return eps, nil
}

func (db *DB) MarkEpisodeWatched(userID, showID int64, season, episode int) error {
	now := time.Now()
	_, err := db.conn.Exec(`
		INSERT INTO episodes (user_id, external_id, show_id, season_number, episode_number, watched, watched_at, runtime)
		VALUES (?, 0, ?, ?, ?, 1, ?, 0)
		ON CONFLICT(user_id, show_id, season_number, episode_number) DO NOTHING`,
		userID, showID, season, episode, now)
	if err != nil {
		return err
	}
	_, err = db.conn.Exec("UPDATE user_shows SET episodes_seen = episodes_seen + 1, updated_at = ? WHERE user_id = ? AND show_id = ?", now, userID, showID)
	return err
}

func (db *DB) UnmarkEpisodeWatched(userID, showID int64, season, episode int) error {
	res, err := db.conn.Exec(`
		DELETE FROM episodes WHERE user_id = ? AND show_id = ? AND season_number = ? AND episode_number = ?`,
		userID, showID, season, episode)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		db.conn.Exec("UPDATE user_shows SET episodes_seen = MAX(episodes_seen - 1, 0), updated_at = ? WHERE user_id = ? AND show_id = ?", time.Now(), userID, showID)
	}
	return nil
}

func (db *DB) MarkSeasonWatched(userID, showID int64, season, totalEpisodes int) (int, error) {
	now := time.Now()
	marked := 0
	for ep := 1; ep <= totalEpisodes; ep++ {
		res, err := db.conn.Exec(`
			INSERT OR IGNORE INTO episodes (user_id, external_id, show_id, season_number, episode_number, watched, watched_at, runtime)
			VALUES (?, 0, ?, ?, ?, 1, ?, 0)`,
			userID, showID, season, ep, now)
		if err != nil {
			continue
		}
		affected, _ := res.RowsAffected()
		marked += int(affected)
	}
	if marked > 0 {
		db.conn.Exec("UPDATE user_shows SET episodes_seen = episodes_seen + ?, updated_at = ? WHERE user_id = ? AND show_id = ?", marked, now, userID, showID)
	}
	return marked, nil
}

func (db *DB) UnmarkSeasonWatched(userID, showID int64, season int) (int, error) {
	res, err := db.conn.Exec(
		"DELETE FROM episodes WHERE user_id = ? AND show_id = ? AND season_number = ?",
		userID, showID, season)
	if err != nil {
		return 0, err
	}
	removed, _ := res.RowsAffected()
	if removed > 0 {
		db.conn.Exec("UPDATE user_shows SET episodes_seen = MAX(episodes_seen - ?, 0), updated_at = ? WHERE user_id = ? AND show_id = ?", removed, time.Now(), userID, showID)
	}
	return int(removed), nil
}

func (db *DB) GetShowProgress(userID, showID int64) (models.ShowProgress, error) {
	var p models.ShowProgress
	err := db.conn.QueryRow(`
		SELECT show_id, season_number, episode_number, watched_at
		FROM episodes WHERE user_id = ? AND show_id = ? AND watched = 1
		ORDER BY season_number DESC, episode_number DESC LIMIT 1`, userID, showID).
		Scan(&p.ShowID, &p.LastSeasonNumber, &p.LastEpisodeNumber, &p.UpdatedAt)
	if err == nil {
		p.ShowName = "from_episodes"
		return p, nil
	}
	err = db.conn.QueryRow("SELECT show_id, show_name, last_season_number, last_episode_number, last_episode_id, updated_at FROM show_progress WHERE user_id = ? AND show_id = ?", userID, showID).
		Scan(&p.ShowID, &p.ShowName, &p.LastSeasonNumber, &p.LastEpisodeNumber, &p.LastEpisodeID, &p.UpdatedAt)
	return p, err
}

// --- Movies (shared catalog + per-user watch) ---

func (db *DB) UpsertMovie(m models.Movie) (int64, error) {
	_, err := db.conn.Exec(`
		INSERT INTO movies (external_id, name, tmdb_id, poster_url, overview, genres, runtime)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(external_id) DO UPDATE SET
			name=COALESCE(NULLIF(excluded.name, ''), movies.name),
			tmdb_id=MAX(movies.tmdb_id, excluded.tmdb_id),
			poster_url=COALESCE(NULLIF(excluded.poster_url, ''), movies.poster_url)`,
		m.ExternalID, m.Name, m.TMDBID, m.PosterURL, m.Overview, m.Genres, m.Runtime)
	if err != nil {
		return 0, err
	}
	var id int64
	db.conn.QueryRow("SELECT id FROM movies WHERE external_id = ?", m.ExternalID).Scan(&id)
	return id, nil
}

func (db *DB) MarkMovieWatched(userID, movieID int64, watchedAt time.Time) error {
	_, err := db.conn.Exec(`INSERT INTO user_movies (user_id, movie_id, watched_at) VALUES (?, ?, ?)
		ON CONFLICT(user_id, movie_id) DO UPDATE SET watched_at = excluded.watched_at`, userID, movieID, watchedAt)
	return err
}

func (db *DB) AddMovieToLibrary(userID, movieID int64) error {
	_, err := db.conn.Exec(`INSERT INTO user_movies (user_id, movie_id) VALUES (?, ?)
		ON CONFLICT(user_id, movie_id) DO NOTHING`, userID, movieID)
	return err
}

func (db *DB) UnmarkMovieWatched(userID, movieID int64) error {
	_, err := db.conn.Exec(`UPDATE user_movies SET watched_at = NULL WHERE user_id = ? AND movie_id = ?`, userID, movieID)
	return err
}

func (db *DB) GetUserMoviesUnwatched(userID int64) ([]models.UserMovie, error) {
	rows, err := db.conn.Query(`SELECT m.id, m.external_id, m.name, m.name_es, m.name_en, m.tmdb_id, m.poster_url, m.overview, m.overview_en, m.genres, m.genres_en, m.runtime
		FROM user_movies um JOIN movies m ON m.id = um.movie_id
		WHERE um.user_id = ? AND um.watched_at IS NULL ORDER BY m.name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movies []models.UserMovie
	for rows.Next() {
		var um models.UserMovie
		if err := rows.Scan(&um.ID, &um.ExternalID, &um.Name, &um.NameES, &um.NameEN, &um.TMDBID, &um.PosterURL, &um.Overview, &um.OverviewEN, &um.Genres, &um.GenresEN, &um.Runtime); err != nil {
			return nil, err
		}
		movies = append(movies, um)
	}
	return movies, nil
}

func (db *DB) GetUserMoviesSorted(userID int64, sort string) ([]models.UserMovie, error) {
	var orderClause string
	switch sort {
	case "name":
		orderClause = "ORDER BY m.name ASC"
	default:
		orderClause = "ORDER BY um.watched_at DESC, m.name ASC"
	}

	rows, err := db.conn.Query(`SELECT m.id, m.external_id, m.name, m.name_es, m.name_en, m.tmdb_id, m.poster_url, m.overview, m.overview_en, m.genres, m.genres_en, m.runtime, um.watched_at
		FROM user_movies um JOIN movies m ON m.id = um.movie_id
		WHERE um.user_id = ? AND um.watched_at IS NOT NULL `+orderClause, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movies []models.UserMovie
	for rows.Next() {
		var um models.UserMovie
		if err := rows.Scan(&um.ID, &um.ExternalID, &um.Name, &um.NameES, &um.NameEN, &um.TMDBID, &um.PosterURL, &um.Overview, &um.OverviewEN, &um.Genres, &um.GenresEN, &um.Runtime, &um.WatchedAt); err != nil {
			return nil, err
		}
		movies = append(movies, um)
	}
	return movies, nil
}

func (db *DB) UpdateMovieTMDB(id int64, tmdbID int, posterURL, overview, genres string, runtime int) error {
	_, err := db.conn.Exec(`UPDATE movies SET tmdb_id=?, poster_url=?, overview=?, genres=?, runtime=? WHERE id=?`,
		tmdbID, posterURL, overview, genres, runtime, id)
	return err
}

func (db *DB) UpdateMovieTMDBEN(id int64, overviewEN, genresEN string) error {
	_, err := db.conn.Exec(`UPDATE movies SET overview_en=?, genres_en=? WHERE id=?`, overviewEN, genresEN, id)
	return err
}

func (db *DB) UpdateMovieTMDBNames(id int64, nameES, nameEN string) error {
	_, err := db.conn.Exec(`UPDATE movies SET name_es=?, name_en=? WHERE id=?`, nameES, nameEN, id)
	return err
}

func (db *DB) GetMoviesWithoutTMDB() ([]models.Movie, error) {
	rows, err := db.conn.Query("SELECT id, external_id, name, tmdb_id, poster_url, overview, genres, runtime FROM movies WHERE tmdb_id = 0 ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movies []models.Movie
	for rows.Next() {
		var m models.Movie
		if err := rows.Scan(&m.ID, &m.ExternalID, &m.Name, &m.TMDBID, &m.PosterURL, &m.Overview, &m.Genres, &m.Runtime); err != nil {
			return nil, err
		}
		movies = append(movies, m)
	}
	return movies, nil
}

func (db *DB) AddMovieFromTMDB(tmdbID int, name, posterURL, overview, genres string, runtime int) (int64, error) {
	extID := fmt.Sprintf("tmdb-%d", tmdbID)
	_, err := db.conn.Exec(`
		INSERT INTO movies (external_id, name, tmdb_id, poster_url, overview, genres, runtime)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(external_id) DO UPDATE SET name=excluded.name, tmdb_id=excluded.tmdb_id, poster_url=excluded.poster_url`,
		extID, name, tmdbID, posterURL, overview, genres, runtime)
	if err != nil {
		return 0, err
	}
	var id int64
	db.conn.QueryRow("SELECT id FROM movies WHERE external_id = ?", extID).Scan(&id)
	return id, nil
}

func (db *DB) SearchMovies(query string) ([]models.Movie, error) {
	rows, err := db.conn.Query("SELECT id, external_id, name, name_es, name_en, tmdb_id, poster_url, overview, overview_en, genres, genres_en, runtime FROM movies WHERE name LIKE ? OR name_es LIKE ? OR name_en LIKE ? ORDER BY name LIMIT 20", "%"+query+"%", "%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movies []models.Movie
	for rows.Next() {
		var m models.Movie
		if err := rows.Scan(&m.ID, &m.ExternalID, &m.Name, &m.NameES, &m.NameEN, &m.TMDBID, &m.PosterURL, &m.Overview, &m.OverviewEN, &m.Genres, &m.GenresEN, &m.Runtime); err != nil {
			return nil, err
		}
		movies = append(movies, m)
	}
	return movies, nil
}

// --- Lists (per-user) ---

func (db *DB) CreateList(userID int64, name string, isPublic bool) (int64, error) {
	res, err := db.conn.Exec(`INSERT INTO lists (user_id, name, is_public, created_at) VALUES (?, ?, ?, ?)`,
		userID, name, isPublic, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) GetUserLists(userID int64) ([]models.List, error) {
	rows, err := db.conn.Query("SELECT id, user_id, name, is_public, created_at FROM lists WHERE user_id = ? ORDER BY name", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lists []models.List
	for rows.Next() {
		var l models.List
		if err := rows.Scan(&l.ID, &l.UserID, &l.Name, &l.IsPublic, &l.CreatedAt); err != nil {
			return nil, err
		}
		lists = append(lists, l)
	}
	return lists, nil
}

func (db *DB) GetListWithItems(id int64) (models.List, error) {
	var l models.List
	err := db.conn.QueryRow("SELECT id, user_id, name, is_public, created_at FROM lists WHERE id = ?", id).
		Scan(&l.ID, &l.UserID, &l.Name, &l.IsPublic, &l.CreatedAt)
	if err != nil {
		return l, err
	}
	rows, err := db.conn.Query(`
		SELECT li.id, li.list_id, li.entity_type, li.entity_id,
			CASE
				WHEN li.name != '' THEN li.name
				WHEN li.entity_type = 'series' THEN COALESCE((SELECT s.name FROM shows s WHERE s.id = li.entity_id), (SELECT s.name FROM shows s WHERE s.external_id = li.entity_id), '')
				WHEN li.entity_type = 'movie' THEN COALESCE((SELECT m.name FROM movies m WHERE m.id = li.entity_id), '')
				ELSE ''
			END as resolved_name,
			CASE
				WHEN li.entity_type = 'series' THEN COALESCE((SELECT s.poster_url FROM shows s WHERE s.id = li.entity_id), (SELECT s.poster_url FROM shows s WHERE s.external_id = li.entity_id), '')
				WHEN li.entity_type = 'movie' THEN COALESCE((SELECT m.poster_url FROM movies m WHERE m.id = li.entity_id), '')
				ELSE ''
			END as poster_url
		FROM list_items li WHERE li.list_id = ?`, id)
	if err != nil {
		return l, err
	}
	defer rows.Close()
	for rows.Next() {
		var item models.ListItem
		if err := rows.Scan(&item.ID, &item.ListID, &item.EntityType, &item.EntityID, &item.Name, &item.PosterURL); err != nil {
			return l, err
		}
		l.Items = append(l.Items, item)
	}
	return l, nil
}

func (db *DB) UpdateList(id int64, name string, isPublic bool) error {
	_, err := db.conn.Exec(`UPDATE lists SET name=?, is_public=? WHERE id=?`, name, isPublic, id)
	return err
}

func (db *DB) DeleteList(id int64) error {
	db.conn.Exec(`DELETE FROM list_items WHERE list_id = ?`, id)
	_, err := db.conn.Exec(`DELETE FROM lists WHERE id = ?`, id)
	return err
}

func (db *DB) AddListItem(item models.ListItem) error {
	_, err := db.conn.Exec(`INSERT INTO list_items (list_id, entity_type, entity_id, name) VALUES (?, ?, ?, ?)`,
		item.ListID, item.EntityType, item.EntityID, item.Name)
	return err
}

func (db *DB) RemoveListItem(itemID int64) error {
	_, err := db.conn.Exec(`DELETE FROM list_items WHERE id = ?`, itemID)
	return err
}

func (db *DB) AddShowToList(listID, showID int64) error {
	var name string
	db.conn.QueryRow("SELECT name FROM shows WHERE id = ?", showID).Scan(&name)
	_, err := db.conn.Exec(`INSERT INTO list_items (list_id, entity_type, entity_id, name) VALUES (?, 'series', ?, ?)`, listID, showID, name)
	return err
}

func (db *DB) AddMovieToList(listID, movieID int64) error {
	var name string
	db.conn.QueryRow("SELECT name FROM movies WHERE id = ?", movieID).Scan(&name)
	_, err := db.conn.Exec(`INSERT INTO list_items (list_id, entity_type, entity_id, name) VALUES (?, 'movie', ?, ?)`, listID, movieID, name)
	return err
}

// --- Watch Stats (per-user) ---

func (db *DB) UpsertWatchStats(userID int64, s models.WatchStats) error {
	_, err := db.conn.Exec(`
		INSERT INTO watch_stats (user_id, period, count, runtime)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, period) DO UPDATE SET count=excluded.count, runtime=excluded.runtime`,
		userID, s.Period, s.Count, s.Runtime)
	return err
}

func (db *DB) IncrementWatchStats(userID int64, count int) {
	period := fmt.Sprintf("month-%s", time.Now().Format("2006-01"))
	db.conn.Exec(`
		INSERT INTO watch_stats (user_id, period, count, runtime)
		VALUES (?, ?, ?, 0)
		ON CONFLICT(user_id, period) DO UPDATE SET count = count + ?`,
		userID, period, count, count)
}

// RecalcWatchStats rebuilds watch_stats from actual episodes and movies in the DB.
// It merges with existing stats (from CSV import), using the higher count for each period.
func (db *DB) RecalcWatchStats(userID int64) error {
	// Recalc from episodes (group by month) — use higher of CSV vs DB count
	// Note: watched_at is stored in Go format "2006-01-02 15:04:05 ..." so we use substr
	_, err := db.conn.Exec(`
		INSERT INTO watch_stats (user_id, period, count, runtime)
		SELECT ?, 'month-' || substr(watched_at, 1, 7), COUNT(*), COALESCE(SUM(runtime), 0)
		FROM episodes
		WHERE user_id = ? AND watched_at IS NOT NULL
		GROUP BY substr(watched_at, 1, 7)
		ON CONFLICT(user_id, period) DO UPDATE SET
			count = MAX(count, excluded.count),
			runtime = MAX(runtime, excluded.runtime)`,
		userID, userID)
	if err != nil {
		return err
	}

	// Add movies (always additive on top of episode counts)
	// Read all rows first to avoid holding connection during Exec (MaxOpenConns=1)
	type movieStat struct {
		period  string
		count   int
		runtime int
	}
	rows, err := db.conn.Query(`
		SELECT 'month-' || substr(um.watched_at, 1, 7) AS period, COUNT(*) AS cnt, COALESCE(SUM(m.runtime), 0) AS rt
		FROM user_movies um
		JOIN movies m ON m.id = um.movie_id
		WHERE um.user_id = ? AND um.watched_at IS NOT NULL
		GROUP BY substr(um.watched_at, 1, 7)`,
		userID)
	if err != nil {
		return err
	}
	var movieStats []movieStat
	for rows.Next() {
		var ms movieStat
		if err := rows.Scan(&ms.period, &ms.count, &ms.runtime); err != nil {
			continue
		}
		movieStats = append(movieStats, ms)
	}
	rows.Close()

	for _, ms := range movieStats {
		db.conn.Exec(`
			INSERT INTO watch_stats (user_id, period, count, runtime)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(user_id, period) DO UPDATE SET count = count + ?, runtime = runtime + ?`,
			userID, ms.period, ms.count, ms.runtime, ms.count, ms.runtime)
	}

	return nil
}

func (db *DB) GetUserWatchStats(userID int64) ([]models.WatchStats, error) {
	rows, err := db.conn.Query("SELECT id, user_id, period, count, runtime FROM watch_stats WHERE user_id = ? ORDER BY period", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []models.WatchStats
	for rows.Next() {
		var s models.WatchStats
		if err := rows.Scan(&s.ID, &s.UserID, &s.Period, &s.Count, &s.Runtime); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

// --- Show Progress (per-user) ---

func (db *DB) UpsertShowProgress(userID int64, p models.ShowProgress) error {
	_, err := db.conn.Exec(`
		INSERT INTO show_progress (user_id, show_id, show_name, last_season_number, last_episode_number, last_episode_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, show_id) DO UPDATE SET
			last_season_number=excluded.last_season_number,
			last_episode_number=excluded.last_episode_number,
			last_episode_id=excluded.last_episode_id,
			updated_at=excluded.updated_at`,
		userID, p.ShowID, p.ShowName, p.LastSeasonNumber, p.LastEpisodeNumber, p.LastEpisodeID, p.UpdatedAt)
	return err
}

// --- Dashboard Stats (per-user) ---

type DashboardStats struct {
	TotalShows    int `json:"total_shows"`
	FollowedShows int `json:"followed_shows"`
	TotalEpisodes int `json:"total_episodes"`
	TotalMovies   int `json:"total_movies"`
	TotalRuntime  int `json:"total_runtime"`
}

func (db *DB) GetDashboardStats(userID int64) (DashboardStats, error) {
	var s DashboardStats
	db.conn.QueryRow("SELECT COUNT(*) FROM user_shows WHERE user_id = ?", userID).Scan(&s.TotalShows)
	db.conn.QueryRow("SELECT COUNT(*) FROM user_shows WHERE user_id = ? AND is_followed = 1", userID).Scan(&s.FollowedShows)
	db.conn.QueryRow("SELECT COUNT(*) FROM episodes WHERE user_id = ?", userID).Scan(&s.TotalEpisodes)
	db.conn.QueryRow("SELECT COUNT(*) FROM user_movies WHERE user_id = ?", userID).Scan(&s.TotalMovies)
	db.conn.QueryRow("SELECT COALESCE(SUM(runtime), 0) FROM watch_stats WHERE user_id = ?", userID).Scan(&s.TotalRuntime)
	return s, nil
}

// --- Upcoming Cache (shared between users) ---

type CachedUpcoming struct {
	ShowID        int64  `json:"show_id"`
	ShowName      string `json:"show_name"`
	PosterURL     string `json:"poster_url"`
	EpisodeName   string `json:"episode_name"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	AirDate       string `json:"air_date"`
	Overview      string `json:"overview"`
}

func (db *DB) UpsertUpcomingCache(showID int64, showName, posterURL, episodeName string, season, episode int, airDate, overview string) error {
	_, err := db.conn.Exec(`
		INSERT INTO upcoming_cache (show_id, show_name, poster_url, episode_name, season_number, episode_number, air_date, overview, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(show_id) DO UPDATE SET
			show_name=excluded.show_name, poster_url=excluded.poster_url, episode_name=excluded.episode_name,
			season_number=excluded.season_number, episode_number=excluded.episode_number,
			air_date=excluded.air_date, overview=excluded.overview, fetched_at=CURRENT_TIMESTAMP`,
		showID, showName, posterURL, episodeName, season, episode, airDate, overview)
	return err
}

func (db *DB) DeleteUpcomingCache(showID int64) error {
	_, err := db.conn.Exec("DELETE FROM upcoming_cache WHERE show_id = ?", showID)
	return err
}

func (db *DB) GetUpcomingCacheForUser(userID int64) ([]CachedUpcoming, error) {
	rows, err := db.conn.Query(`
		SELECT uc.show_id, uc.show_name, uc.poster_url, uc.episode_name, uc.season_number, uc.episode_number, uc.air_date, uc.overview
		FROM upcoming_cache uc
		JOIN user_shows us ON us.show_id = uc.show_id AND us.user_id = ? AND us.is_followed = 1
		WHERE uc.air_date != ''
		ORDER BY uc.air_date ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []CachedUpcoming
	for rows.Next() {
		var c CachedUpcoming
		if err := rows.Scan(&c.ShowID, &c.ShowName, &c.PosterURL, &c.EpisodeName, &c.SeasonNumber, &c.EpisodeNumber, &c.AirDate, &c.Overview); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, nil
}

func (db *DB) GetAllShowsWithTMDB() ([]models.Show, error) {
	rows, err := db.conn.Query("SELECT id, external_id, name, tmdb_id, poster_url, backdrop_url, overview, genres, status, total_seasons FROM shows WHERE tmdb_id > 0 ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shows []models.Show
	for rows.Next() {
		var s models.Show
		if err := rows.Scan(&s.ID, &s.ExternalID, &s.Name, &s.TMDBID, &s.PosterURL, &s.BackdropURL, &s.Overview, &s.Genres, &s.Status, &s.TotalSeasons); err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, nil
}

func (db *DB) GetAllMoviesWithTMDB() ([]models.Movie, error) {
	rows, err := db.conn.Query("SELECT id, external_id, name, tmdb_id, poster_url, overview, genres, runtime FROM movies WHERE tmdb_id > 0 ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var movies []models.Movie
	for rows.Next() {
		var m models.Movie
		if err := rows.Scan(&m.ID, &m.ExternalID, &m.Name, &m.TMDBID, &m.PosterURL, &m.Overview, &m.Genres, &m.Runtime); err != nil {
			return nil, err
		}
		movies = append(movies, m)
	}
	return movies, nil
}

// --- Season Episodes Cache ---

func (db *DB) UpsertSeasonEpisodes(showID int64, seasonNumber, episodeCount int) error {
	_, err := db.conn.Exec(`INSERT INTO season_episodes (show_id, season_number, episode_count) VALUES (?, ?, ?)
		ON CONFLICT(show_id, season_number) DO UPDATE SET episode_count = excluded.episode_count`,
		showID, seasonNumber, episodeCount)
	return err
}

func (db *DB) GetSeasonEpisodes(showID int64) (map[int]int, error) {
	rows, err := db.conn.Query("SELECT season_number, episode_count FROM season_episodes WHERE show_id = ? ORDER BY season_number", showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int]int)
	for rows.Next() {
		var season, count int
		if err := rows.Scan(&season, &count); err != nil {
			return nil, err
		}
		result[season] = count
	}
	return result, nil
}

func (db *DB) GetActiveShowsWithTMDB() ([]models.Show, error) {
	rows, err := db.conn.Query("SELECT id, external_id, name, tmdb_id, poster_url, backdrop_url, overview, genres, status, total_seasons FROM shows WHERE tmdb_id > 0 AND status != 'Ended' AND status != 'Canceled' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shows []models.Show
	for rows.Next() {
		var s models.Show
		if err := rows.Scan(&s.ID, &s.ExternalID, &s.Name, &s.TMDBID, &s.PosterURL, &s.BackdropURL, &s.Overview, &s.Genres, &s.Status, &s.TotalSeasons); err != nil {
			return nil, err
		}
		shows = append(shows, s)
	}
	return shows, nil
}

// MovieStats holds aggregate movie statistics for a user.
type MovieStats struct {
	TotalMovies  int
	TotalRuntime int
}

func (db *DB) GetMovieStats(userID int64) (MovieStats, error) {
	var s MovieStats
	db.conn.QueryRow(`SELECT COUNT(*), COALESCE(SUM(m.runtime), 0) FROM user_movies um JOIN movies m ON m.id = um.movie_id WHERE um.user_id = ? AND um.watched_at IS NOT NULL`, userID).Scan(&s.TotalMovies, &s.TotalRuntime)
	return s, nil
}

// --- Sessions ---

func (db *DB) CreateSession(token string, userID int64, expiresAt time.Time) error {
	_, err := db.conn.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, expiresAt)
	return err
}

func (db *DB) GetSession(token string) (int64, bool) {
	var userID int64
	err := db.conn.QueryRow(`SELECT user_id FROM sessions WHERE token = ? AND expires_at > ?`, token, time.Now()).Scan(&userID)
	if err != nil {
		return 0, false
	}
	return userID, true
}

func (db *DB) DeleteSession(token string) error {
	_, err := db.conn.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (db *DB) CleanExpiredSessions() error {
	_, err := db.conn.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now())
	return err
}

// --- Users: Email & Password ---

func (db *DB) GetUserByEmail(email string) (models.User, error) {
	var u models.User
	err := db.conn.QueryRow("SELECT id, username, email, password_hash, created_at FROM users WHERE email = ?", email).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, err
}

func (db *DB) UpdateUserEmail(userID int64, email string) error {
	_, err := db.conn.Exec("UPDATE users SET email = ? WHERE id = ?", email, userID)
	return err
}

func (db *DB) UpdateUserPassword(userID int64, passwordHash string) error {
	_, err := db.conn.Exec("UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, userID)
	return err
}

// --- Magic Links ---

func (db *DB) CreateMagicLink(token string, userID int64, purpose string, expiresAt time.Time) error {
	_, err := db.conn.Exec(`INSERT INTO magic_links (token, user_id, purpose, expires_at) VALUES (?, ?, ?, ?)`, token, userID, purpose, expiresAt)
	return err
}

func (db *DB) GetMagicLink(token string) (int64, string, bool) {
	var userID int64
	var purpose string
	err := db.conn.QueryRow(`SELECT user_id, purpose FROM magic_links WHERE token = ? AND expires_at > ?`, token, time.Now()).Scan(&userID, &purpose)
	if err != nil {
		return 0, "", false
	}
	return userID, purpose, true
}

func (db *DB) DeleteMagicLink(token string) error {
	_, err := db.conn.Exec(`DELETE FROM magic_links WHERE token = ?`, token)
	return err
}

func (db *DB) CleanExpiredMagicLinks() error {
	_, err := db.conn.Exec(`DELETE FROM magic_links WHERE expires_at <= ?`, time.Now())
	return err
}

// --- Episode Details ---

// EpisodeDetail holds metadata for a single episode from TMDB.
type EpisodeDetail struct {
	ShowID        int64
	SeasonNumber  int
	EpisodeNumber int
	Name          string
	NameEN        string
	Overview      string
	OverviewEN    string
	AirDate       string
	Runtime       int
	StillURL      string
}

func (db *DB) UpsertEpisodeDetail(d EpisodeDetail) error {
	_, err := db.conn.Exec(`INSERT INTO episode_details (show_id, season_number, episode_number, name, name_en, overview, overview_en, air_date, runtime, still_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(show_id, season_number, episode_number) DO UPDATE SET
			name=excluded.name, name_en=excluded.name_en,
			overview=excluded.overview, overview_en=excluded.overview_en,
			air_date=excluded.air_date, runtime=excluded.runtime, still_url=excluded.still_url`,
		d.ShowID, d.SeasonNumber, d.EpisodeNumber, d.Name, d.NameEN, d.Overview, d.OverviewEN, d.AirDate, d.Runtime, d.StillURL)
	return err
}

func (db *DB) GetEpisodeDetails(showID int64) ([]EpisodeDetail, error) {
	rows, err := db.conn.Query(`SELECT show_id, season_number, episode_number, name, name_en, overview, overview_en, air_date, runtime, still_url
		FROM episode_details WHERE show_id = ? ORDER BY season_number, episode_number`, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var details []EpisodeDetail
	for rows.Next() {
		var d EpisodeDetail
		if err := rows.Scan(&d.ShowID, &d.SeasonNumber, &d.EpisodeNumber, &d.Name, &d.NameEN, &d.Overview, &d.OverviewEN, &d.AirDate, &d.Runtime, &d.StillURL); err != nil {
			return nil, err
		}
		details = append(details, d)
	}
	return details, nil
}

// ContinueWatchingItem represents the next unwatched episode for a show.
type ContinueWatchingItem struct {
	ShowID        int64
	ShowName      string
	ShowNameES    string
	ShowNameEN    string
	PosterURL     string
	SeasonNumber  int
	EpisodeNumber int
	EpName        string
	EpNameEN      string
	EpOverview    string
	EpOverviewEN  string
	StillURL      string
	DaysSinceLast int
}

// GetContinueWatching returns the next unwatched episode for shows the user is mid-season on.
// It excludes shows where the next episode is in a new (unwatched) season — those go to GetNewSeasons.
func (db *DB) GetContinueWatching(userID int64, limit int, offset ...int) ([]ContinueWatchingItem, error) {
	off := 0
	if len(offset) > 0 {
		off = offset[0]
	}
	all, err := db.getContinueWatchingCandidates(userID)
	if err != nil {
		return nil, err
	}

	var items []ContinueWatchingItem
	skipped := 0
	for _, item := range all {
		if len(items) >= limit {
			break
		}
		if item.isNewSeason {
			continue
		}
		if skipped < off {
			skipped++
			continue
		}
		items = append(items, item.ContinueWatchingItem)
	}
	return items, nil
}

// GetNewSeasons returns shows where the user completed previous seasons and a new season is now available.
func (db *DB) GetNewSeasons(userID int64) ([]ContinueWatchingItem, error) {
	all, err := db.getContinueWatchingCandidates(userID)
	if err != nil {
		return nil, err
	}

	var items []ContinueWatchingItem
	for _, item := range all {
		if item.isNewSeason {
			items = append(items, item.ContinueWatchingItem)
		}
	}
	return items, nil
}

type candidateItem struct {
	ContinueWatchingItem
	isNewSeason bool
}

func (db *DB) getContinueWatchingCandidates(userID int64) ([]candidateItem, error) {
	rows, err := db.conn.Query(`
		SELECT us.show_id, s.name, s.name_es, s.name_en, s.poster_url,
			COALESCE((SELECT MAX(e.watched_at) FROM episodes e WHERE e.user_id = us.user_id AND e.show_id = us.show_id), us.followed_at) as last_activity
		FROM user_shows us
		JOIN shows s ON s.id = us.show_id
		WHERE us.user_id = ? AND us.is_followed = 1 AND us.is_archived = 0
			AND (us.snoozed_until IS NULL OR us.snoozed_until < ?)
		ORDER BY last_activity DESC`, userID, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidateShow struct {
		id           int64
		name         string
		nameES       string
		nameEN       string
		poster       string
		lastActivity string
	}
	var candidates []candidateShow
	for rows.Next() {
		var c candidateShow
		if err := rows.Scan(&c.id, &c.name, &c.nameES, &c.nameEN, &c.poster, &c.lastActivity); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}

	var items []candidateItem
	for _, c := range candidates {
		// Get all season/episode counts for this show
		seasons, _ := db.GetSeasonEpisodes(c.id)
		if len(seasons) == 0 {
			continue
		}

		// Get watched episodes for this user/show
		watchedSet := make(map[string]bool)
		maxWatchedSeason := 0
		wRows, err := db.conn.Query("SELECT season_number, episode_number FROM episodes WHERE user_id = ? AND show_id = ?", userID, c.id)
		if err != nil {
			continue
		}
		for wRows.Next() {
			var sn, en int
			wRows.Scan(&sn, &en)
			watchedSet[fmt.Sprintf("%d-%d", sn, en)] = true
			if sn > maxWatchedSeason {
				maxWatchedSeason = sn
			}
		}
		wRows.Close()

		// Find first unwatched episode
		var foundSeason, foundEpisode int
		found := false
		sortedSeasons := make([]int, 0, len(seasons))
		for sn := range seasons {
			sortedSeasons = append(sortedSeasons, sn)
		}
		sortInts(sortedSeasons)
		for _, sn := range sortedSeasons {
			count := seasons[sn]
			for ep := 1; ep <= count; ep++ {
				if !watchedSet[fmt.Sprintf("%d-%d", sn, ep)] {
					foundSeason = sn
					foundEpisode = ep
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			continue // fully watched
		}

		// Get episode details
		var item candidateItem
		item.ShowID = c.id
		item.ShowName = c.name
		item.ShowNameES = c.nameES
		item.ShowNameEN = c.nameEN
		item.PosterURL = c.poster
		item.SeasonNumber = foundSeason
		item.EpisodeNumber = foundEpisode

		var airDate string
		err = db.conn.QueryRow(`SELECT name, name_en, overview, overview_en, still_url, air_date
			FROM episode_details WHERE show_id = ? AND season_number = ? AND episode_number = ?`,
			c.id, foundSeason, foundEpisode).Scan(&item.EpName, &item.EpNameEN, &item.EpOverview, &item.EpOverviewEN, &item.StillURL, &airDate)
		if err != nil {
			item.EpName = fmt.Sprintf("S%02dE%02d", foundSeason, foundEpisode)
		} else if airDate == "" || airDate > time.Now().Format("2006-01-02") {
			// Episode not yet aired — skip
			continue
		}

		// Calculate days since last activity
		if len(c.lastActivity) >= 19 {
			if t, err := time.Parse("2006-01-02 15:04:05", c.lastActivity[:19]); err == nil {
				item.DaysSinceLast = int(time.Since(t).Hours() / 24)
			}
		}

		// Determine if this is a new season: first unwatched is in a season beyond what user has watched
		item.isNewSeason = foundSeason > maxWatchedSeason && maxWatchedSeason > 0

		items = append(items, item)
	}

	return items, nil
}

// sortInts sorts a slice of ints in ascending order.
func sortInts(s []int) {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}


// --- API Keys ---

// APIKey represents a user's API key metadata.
type APIKey struct {
	ID         int64
	Name       string
	Scopes     string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

func (db *DB) CreateAPIKey(userID int64, keyHash, name, scopes string) (int64, error) {
	res, err := db.conn.Exec(`INSERT INTO api_keys (user_id, key_hash, name, scopes) VALUES (?, ?, ?, ?)`,
		userID, keyHash, name, scopes)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) ValidateAPIKey(keyHash string) (int64, string, bool) {
	var userID int64
	var scopes string
	err := db.conn.QueryRow(`SELECT user_id, scopes FROM api_keys WHERE key_hash = ?`, keyHash).Scan(&userID, &scopes)
	if err != nil {
		return 0, "", false
	}
	// Update last_used_at
	db.conn.Exec(`UPDATE api_keys SET last_used_at = ? WHERE key_hash = ?`, time.Now(), keyHash)
	return userID, scopes, true
}

func (db *DB) GetUserAPIKeys(userID int64) ([]APIKey, error) {
	rows, err := db.conn.Query(`SELECT id, name, scopes, created_at, last_used_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var lastUsed sql.NullTime
		if err := rows.Scan(&k.ID, &k.Name, &k.Scopes, &k.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			k.LastUsedAt = lastUsed.Time
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (db *DB) DeleteAPIKey(userID, keyID int64) error {
	_, err := db.conn.Exec(`DELETE FROM api_keys WHERE id = ? AND user_id = ?`, keyID, userID)
	return err
}

// --- Timeline ---

// TimelineItem represents a watched episode or movie in the timeline.
type TimelineItem struct {
	Type          string // "episode" or "movie"
	ShowID        int64
	ShowName      string
	ShowNameES    string
	ShowNameEN    string
	PosterURL     string
	SeasonNumber  int
	EpisodeNumber int
	EpName        string
	EpNameEN      string
	MovieName     string
	WatchedAt     string
	Date          string // YYYY-MM-DD
}

// GetTimelineItems returns watched items (episodes + movies) ordered by date, paginated.
func (db *DB) GetTimelineItems(userID int64, before string, limit int) ([]TimelineItem, error) {
	var beforeClause string
	var args []any
	args = append(args, userID, userID)
	if before != "" {
		beforeClause = "HAVING date < ?"
		args = append(args, before)
	}

	query := `
		SELECT type, show_id, show_name, show_name_es, show_name_en, poster_url, season_number, episode_number, ep_name, ep_name_en, date FROM (
			SELECT 'episode' as type, e.show_id, s.name as show_name, s.name_es as show_name_es, s.name_en as show_name_en, s.poster_url,
				e.season_number, e.episode_number,
				COALESCE((SELECT ed.name FROM episode_details ed WHERE ed.show_id = e.show_id AND ed.season_number = e.season_number AND ed.episode_number = e.episode_number), '') as ep_name,
				COALESCE((SELECT ed.name_en FROM episode_details ed WHERE ed.show_id = e.show_id AND ed.season_number = e.season_number AND ed.episode_number = e.episode_number), '') as ep_name_en,
				substr(e.watched_at, 1, 10) as date
			FROM episodes e JOIN shows s ON s.id = e.show_id
			WHERE e.user_id = ? AND e.watched_at != ''
			UNION ALL
			SELECT 'movie' as type, m.id as show_id, m.name as show_name, m.name_es as show_name_es, m.name_en as show_name_en, m.poster_url,
				0 as season_number, 0 as episode_number, '' as ep_name, '' as ep_name_en,
				substr(um.watched_at, 1, 10) as date
			FROM user_movies um JOIN movies m ON m.id = um.movie_id
			WHERE um.user_id = ? AND um.watched_at IS NOT NULL
		) items
		GROUP BY type, show_id, season_number, episode_number
		` + beforeClause + `
		ORDER BY date DESC, show_name ASC
		LIMIT ?`
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TimelineItem
	for rows.Next() {
		var item TimelineItem
		if err := rows.Scan(&item.Type, &item.ShowID, &item.ShowName, &item.ShowNameES, &item.ShowNameEN, &item.PosterURL, &item.SeasonNumber, &item.EpisodeNumber, &item.EpName, &item.EpNameEN, &item.Date); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// TimelinePeriod represents a collapsed time period with item count.
type TimelinePeriod struct {
	Type  string // "week", "month", "year"
	Label string
	From  string // YYYY-MM-DD
	To    string // YYYY-MM-DD
	Count int
}

// GetTimelinePeriods returns grouped periods (weeks, months, years) for items older than a cutoff date.
func (db *DB) GetTimelinePeriods(userID int64, olderThan string) ([]TimelinePeriod, error) {
	// Get all dates with activity older than cutoff
	rows, err := db.conn.Query(`
		SELECT date, cnt FROM (
			SELECT substr(watched_at, 1, 10) as date, COUNT(*) as cnt
			FROM episodes WHERE user_id = ? AND watched_at != '' AND substr(watched_at, 1, 10) < ?
			GROUP BY date
			UNION ALL
			SELECT substr(watched_at, 1, 10) as date, COUNT(*) as cnt
			FROM user_movies WHERE user_id = ? AND watched_at IS NOT NULL AND substr(watched_at, 1, 10) < ?
			GROUP BY date
		) ORDER BY date DESC`, userID, olderThan, userID, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type dayCount struct {
		date  string
		count int
	}
	var days []dayCount
	for rows.Next() {
		var d dayCount
		rows.Scan(&d.date, &d.count)
		days = append(days, d)
	}

	if len(days) == 0 {
		return nil, nil
	}

	// Group into weeks (for first ~8 weeks), then months, then years
	var periods []TimelinePeriod
	weekCount := 0
	monthCount := 0
	currentWeek := ""
	currentMonth := ""
	currentYear := ""

	for _, d := range days {
		year := d.date[:4]
		month := d.date[:7]
		// Simple week grouping: by ISO week approximation (group by 7-day blocks)
		// For simplicity, group by month when > 8 weeks old

		if weekCount < 8 {
			// Group by week (use the Monday of the week)
			week := getWeekLabel(d.date)
			if week != currentWeek {
				if currentWeek != "" {
					weekCount++
				}
				currentWeek = week
				if weekCount < 8 {
					periods = append(periods, TimelinePeriod{
						Type:  "week",
						Label: week,
						From:  d.date,
						To:    d.date,
						Count: d.count,
					})
				} else {
					// Switch to months
					currentMonth = month
					periods = append(periods, TimelinePeriod{
						Type:  "month",
						Label: month,
						From:  d.date,
						To:    d.date,
						Count: d.count,
					})
				}
			} else {
				// Add to current week
				last := &periods[len(periods)-1]
				last.Count += d.count
				last.From = d.date
			}
		} else if monthCount < 12 {
			// Group by month
			if month != currentMonth {
				currentMonth = month
				monthCount++
				periods = append(periods, TimelinePeriod{
					Type:  "month",
					Label: month,
					From:  d.date,
					To:    d.date,
					Count: d.count,
				})
			} else {
				last := &periods[len(periods)-1]
				last.Count += d.count
				last.From = d.date
			}
		} else {
			// Group by year
			if year != currentYear {
				currentYear = year
				periods = append(periods, TimelinePeriod{
					Type:  "year",
					Label: year,
					From:  d.date,
					To:    d.date,
					Count: d.count,
				})
			} else {
				last := &periods[len(periods)-1]
				last.Count += d.count
				last.From = d.date
			}
		}
	}

	return periods, nil
}

// GetTimelineItemsForRange returns items between two dates.
func (db *DB) GetTimelineItemsForRange(userID int64, from, to string) ([]TimelineItem, error) {
	query := `
		SELECT type, show_id, show_name, show_name_es, show_name_en, poster_url, season_number, episode_number, ep_name, ep_name_en, date FROM (
			SELECT 'episode' as type, e.show_id, s.name as show_name, s.name_es as show_name_es, s.name_en as show_name_en, s.poster_url,
				e.season_number, e.episode_number,
				COALESCE((SELECT ed.name FROM episode_details ed WHERE ed.show_id = e.show_id AND ed.season_number = e.season_number AND ed.episode_number = e.episode_number), '') as ep_name,
				COALESCE((SELECT ed.name_en FROM episode_details ed WHERE ed.show_id = e.show_id AND ed.season_number = e.season_number AND ed.episode_number = e.episode_number), '') as ep_name_en,
				substr(e.watched_at, 1, 10) as date
			FROM episodes e JOIN shows s ON s.id = e.show_id
			WHERE e.user_id = ? AND e.watched_at != '' AND substr(e.watched_at, 1, 10) >= ? AND substr(e.watched_at, 1, 10) <= ?
			UNION ALL
			SELECT 'movie' as type, m.id as show_id, m.name as show_name, m.name_es as show_name_es, m.name_en as show_name_en, m.poster_url,
				0 as season_number, 0 as episode_number, '' as ep_name, '' as ep_name_en,
				substr(um.watched_at, 1, 10) as date
			FROM user_movies um JOIN movies m ON m.id = um.movie_id
			WHERE um.user_id = ? AND um.watched_at IS NOT NULL AND substr(um.watched_at, 1, 10) >= ? AND substr(um.watched_at, 1, 10) <= ?
		) items
		ORDER BY date DESC, show_name ASC`

	rows, err := db.conn.Query(query, userID, from, to, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TimelineItem
	for rows.Next() {
		var item TimelineItem
		if err := rows.Scan(&item.Type, &item.ShowID, &item.ShowName, &item.ShowNameES, &item.ShowNameEN, &item.PosterURL, &item.SeasonNumber, &item.EpisodeNumber, &item.EpName, &item.EpNameEN, &item.Date); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func getWeekLabel(date string) string {
	// Return YYYY-Www format for grouping
	if len(date) < 10 {
		return date
	}
	// Simple: group by the date's year + week-of-month approximation
	// Use the date truncated to the start of the week (Monday)
	// For simplicity, just use YYYY-MM-WN where N is day/7
	day := 1
	if len(date) >= 10 {
		fmt.Sscanf(date[8:10], "%d", &day)
	}
	weekNum := (day - 1) / 7
	return fmt.Sprintf("%s-w%d", date[:7], weekNum)
}

// GetTimelineYears returns distinct years that have watched activity.
func (db *DB) GetTimelineYears(userID int64) ([]string, error) {
	rows, err := db.conn.Query(`
		SELECT DISTINCT year FROM (
			SELECT substr(watched_at, 1, 4) as year FROM episodes WHERE user_id = ? AND watched_at != ''
			UNION
			SELECT substr(watched_at, 1, 4) as year FROM user_movies WHERE user_id = ? AND watched_at IS NOT NULL
		) ORDER BY year DESC`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var years []string
	for rows.Next() {
		var y string
		rows.Scan(&y)
		if y != "" {
			years = append(years, y)
		}
	}
	return years, nil
}
