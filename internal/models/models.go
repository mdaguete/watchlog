package models

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Show is the shared catalog entry (TMDB metadata cache)
type Show struct {
	ID           int64  `json:"id"`
	ExternalID   int64  `json:"external_id"`
	Name         string `json:"name"`
	TMDBID       int    `json:"tmdb_id"`
	PosterURL    string `json:"poster_url"`
	BackdropURL  string `json:"backdrop_url"`
	Overview     string `json:"overview"`
	Genres       string `json:"genres"`
	Status       string `json:"status"`
	TotalSeasons int    `json:"total_seasons"`
}

// UserShow is the per-user relationship to a show
type UserShow struct {
	Show
	IsFollowed   bool      `json:"is_followed"`
	IsFavorited  bool      `json:"is_favorited"`
	IsArchived   bool      `json:"is_archived"`
	EpisodesSeen int       `json:"episodes_seen"`
	FollowedAt   time.Time `json:"followed_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Episode struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	ExternalID    int64     `json:"external_id"`
	ShowID        int64     `json:"show_id"`
	SeasonNumber  int       `json:"season_number"`
	EpisodeNumber int       `json:"episode_number"`
	Watched       bool      `json:"watched"`
	WatchedAt     time.Time `json:"watched_at"`
	Runtime       int       `json:"runtime"`
}

// Movie is the shared catalog entry
type Movie struct {
	ID        int64  `json:"id"`
	ExternalID string `json:"external_id"`
	Name      string `json:"name"`
	TMDBID    int    `json:"tmdb_id"`
	PosterURL string `json:"poster_url"`
	Overview  string `json:"overview"`
	Genres    string `json:"genres"`
	Runtime   int    `json:"runtime"`
}

// UserMovie is the per-user watch record
type UserMovie struct {
	Movie
	WatchedAt time.Time `json:"watched_at"`
}

type List struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Name      string     `json:"name"`
	IsPublic  bool       `json:"is_public"`
	CreatedAt time.Time  `json:"created_at"`
	Items     []ListItem `json:"items"`
}

type ListItem struct {
	ID         int64  `json:"id"`
	ListID     int64  `json:"list_id"`
	EntityType string `json:"entity_type"`
	EntityID   int64  `json:"entity_id"`
	Name       string `json:"name"`
	PosterURL  string `json:"poster_url"`
}

type WatchStats struct {
	ID      int64  `json:"id"`
	UserID  int64  `json:"user_id"`
	Period  string `json:"period"`
	Count   int    `json:"count"`
	Runtime int    `json:"runtime"`
}

type ShowProgress struct {
	ShowID            int64     `json:"show_id"`
	ShowName          string    `json:"show_name"`
	LastSeasonNumber  int       `json:"last_season_number"`
	LastEpisodeNumber int       `json:"last_episode_number"`
	LastEpisodeID     int64     `json:"last_episode_id"`
	UpdatedAt         time.Time `json:"updated_at"`
}
