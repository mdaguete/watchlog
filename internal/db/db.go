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
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(1)
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	_, err := db.conn.Exec(schema)
	if err != nil {
		return err
	}
	// Add lang column if missing (existing DBs)
	db.conn.Exec("ALTER TABLE users ADD COLUMN lang TEXT NOT NULL DEFAULT 'es'")
	// Add email column if missing
	db.conn.Exec("ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''")
	// Add i18n columns for shows
	db.conn.Exec("ALTER TABLE shows ADD COLUMN overview_en TEXT NOT NULL DEFAULT ''")
	db.conn.Exec("ALTER TABLE shows ADD COLUMN genres_en TEXT NOT NULL DEFAULT ''")
	db.conn.Exec("ALTER TABLE shows ADD COLUMN name_es TEXT NOT NULL DEFAULT ''")
	db.conn.Exec("ALTER TABLE shows ADD COLUMN name_en TEXT NOT NULL DEFAULT ''")
	// Add i18n columns for movies
	db.conn.Exec("ALTER TABLE movies ADD COLUMN overview_en TEXT NOT NULL DEFAULT ''")
	db.conn.Exec("ALTER TABLE movies ADD COLUMN genres_en TEXT NOT NULL DEFAULT ''")
	db.conn.Exec("ALTER TABLE movies ADD COLUMN name_es TEXT NOT NULL DEFAULT ''")
	db.conn.Exec("ALTER TABLE movies ADD COLUMN name_en TEXT NOT NULL DEFAULT ''")
	return nil
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	lang TEXT NOT NULL DEFAULT 'es',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS shows (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	external_id INTEGER UNIQUE NOT NULL,
	name TEXT NOT NULL,
	tmdb_id INTEGER NOT NULL DEFAULT 0,
	poster_url TEXT NOT NULL DEFAULT '',
	backdrop_url TEXT NOT NULL DEFAULT '',
	overview TEXT NOT NULL DEFAULT '',
	genres TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT '',
	total_seasons INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_shows (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	show_id INTEGER NOT NULL REFERENCES shows(id),
	is_followed INTEGER NOT NULL DEFAULT 1,
	is_favorited INTEGER NOT NULL DEFAULT 0,
	is_archived INTEGER NOT NULL DEFAULT 0,
	episodes_seen INTEGER NOT NULL DEFAULT 0,
	followed_at DATETIME,
	updated_at DATETIME,
	UNIQUE(user_id, show_id)
);

CREATE TABLE IF NOT EXISTS episodes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	external_id INTEGER NOT NULL,
	show_id INTEGER NOT NULL REFERENCES shows(id),
	season_number INTEGER NOT NULL,
	episode_number INTEGER NOT NULL,
	watched INTEGER NOT NULL DEFAULT 1,
	watched_at DATETIME,
	runtime INTEGER NOT NULL DEFAULT 0,
	UNIQUE(user_id, show_id, season_number, episode_number)
);

CREATE TABLE IF NOT EXISTS movies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	external_id TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL,
	tmdb_id INTEGER NOT NULL DEFAULT 0,
	poster_url TEXT NOT NULL DEFAULT '',
	overview TEXT NOT NULL DEFAULT '',
	genres TEXT NOT NULL DEFAULT '',
	runtime INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_movies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	movie_id INTEGER NOT NULL REFERENCES movies(id),
	watched_at DATETIME,
	UNIQUE(user_id, movie_id)
);

CREATE TABLE IF NOT EXISTS lists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	name TEXT NOT NULL,
	is_public INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME
);

CREATE TABLE IF NOT EXISTS list_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	list_id INTEGER NOT NULL REFERENCES lists(id),
	entity_type TEXT NOT NULL,
	entity_id INTEGER NOT NULL DEFAULT 0,
	name TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS watch_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	period TEXT NOT NULL,
	count INTEGER NOT NULL DEFAULT 0,
	runtime INTEGER NOT NULL DEFAULT 0,
	UNIQUE(user_id, period)
);

CREATE TABLE IF NOT EXISTS show_progress (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id),
	show_id INTEGER NOT NULL REFERENCES shows(id),
	show_name TEXT NOT NULL,
	last_season_number INTEGER NOT NULL DEFAULT 0,
	last_episode_number INTEGER NOT NULL DEFAULT 0,
	last_episode_id INTEGER NOT NULL DEFAULT 0,
	updated_at DATETIME,
	UNIQUE(user_id, show_id)
);

CREATE INDEX IF NOT EXISTS idx_user_shows_user ON user_shows(user_id);
CREATE INDEX IF NOT EXISTS idx_user_shows_show ON user_shows(show_id);
CREATE INDEX IF NOT EXISTS idx_episodes_user_show ON episodes(user_id, show_id);
CREATE INDEX IF NOT EXISTS idx_episodes_external_id ON episodes(external_id);
CREATE INDEX IF NOT EXISTS idx_lists_user ON lists(user_id);
CREATE INDEX IF NOT EXISTS idx_watch_stats_user ON watch_stats(user_id);
CREATE INDEX IF NOT EXISTS idx_show_progress_user ON show_progress(user_id, show_id);
CREATE INDEX IF NOT EXISTS idx_user_movies_user ON user_movies(user_id);

CREATE TABLE IF NOT EXISTS upcoming_cache (
	show_id INTEGER PRIMARY KEY REFERENCES shows(id),
	show_name TEXT NOT NULL DEFAULT '',
	poster_url TEXT NOT NULL DEFAULT '',
	episode_name TEXT NOT NULL DEFAULT '',
	season_number INTEGER NOT NULL DEFAULT 0,
	episode_number INTEGER NOT NULL DEFAULT 0,
	air_date TEXT NOT NULL DEFAULT '',
	overview TEXT NOT NULL DEFAULT '',
	fetched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY NOT NULL,
	value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY NOT NULL,
	user_id INTEGER NOT NULL REFERENCES users(id),
	expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS magic_links (
	token TEXT PRIMARY KEY NOT NULL,
	user_id INTEGER NOT NULL REFERENCES users(id),
	purpose TEXT NOT NULL,
	expires_at DATETIME NOT NULL
);
`

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

	query := `SELECT s.id, s.external_id, s.name, s.name_es, s.name_en, s.tmdb_id, s.poster_url, s.backdrop_url, s.overview, s.genres, s.status, s.total_seasons,
		us.is_followed, us.is_favorited, us.is_archived, us.episodes_seen, us.followed_at, us.updated_at
		FROM user_shows us
		JOIN shows s ON s.id = us.show_id
		LEFT JOIN show_progress sp ON sp.show_id = s.id AND sp.user_id = us.user_id
		` + joinClause + `
		WHERE us.user_id = ? AND us.is_followed = 1 ` + orderClause

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
		ON CONFLICT(user_id, movie_id) DO NOTHING`, userID, movieID, watchedAt)
	return err
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
		WHERE um.user_id = ? `+orderClause, userID)
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
