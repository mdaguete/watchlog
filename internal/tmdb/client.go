package tmdb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.themoviedb.org/3"
const imageBaseURL = "https://image.tmdb.org/t/p"

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// --- Types ---

type ShowResult struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Overview     string   `json:"overview"`
	PosterPath   string   `json:"poster_path"`
	BackdropPath string   `json:"backdrop_path"`
	FirstAirDate string   `json:"first_air_date"`
	GenreIDs     []int    `json:"genre_ids"`
	Genres       []Genre  `json:"genres"`
	VoteAverage  float64  `json:"vote_average"`
	Status       string   `json:"status"`
	Seasons      []Season `json:"seasons"`
}

type MovieResult struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	ReleaseDate  string  `json:"release_date"`
	GenreIDs     []int   `json:"genre_ids"`
	Genres       []Genre `json:"genres"`
	VoteAverage  float64 `json:"vote_average"`
	Runtime      int     `json:"runtime"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Season struct {
	ID           int    `json:"id"`
	SeasonNumber int    `json:"season_number"`
	EpisodeCount int    `json:"episode_count"`
	Name         string `json:"name"`
	PosterPath   string `json:"poster_path"`
	AirDate      string `json:"air_date"`
}

type SeasonDetail struct {
	ID           int       `json:"id"`
	SeasonNumber int       `json:"season_number"`
	Episodes     []Episode `json:"episodes"`
	Name         string    `json:"name"`
}

type Episode struct {
	ID            int     `json:"id"`
	EpisodeNumber int     `json:"episode_number"`
	SeasonNumber  int     `json:"season_number"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	AirDate       string  `json:"air_date"`
	Runtime       int     `json:"runtime"`
	StillPath     string  `json:"still_path"`
	VoteAverage   float64 `json:"vote_average"`
}

type SearchResponse[T any] struct {
	Page         int `json:"page"`
	TotalPages   int `json:"total_pages"`
	TotalResults int `json:"total_results"`
	Results      []T `json:"results"`
}

// --- API Methods ---

func (c *Client) SearchTV(query string) ([]ShowResult, error) {
	var resp SearchResponse[ShowResult]
	err := c.get("/search/tv", map[string]string{"query": query, "language": "es-ES"}, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}

func (c *Client) SearchMovie(query string) ([]MovieResult, error) {
	var resp SearchResponse[MovieResult]
	err := c.get("/search/movie", map[string]string{"query": query, "language": "es-ES"}, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}

func (c *Client) GetTVShow(id int) (*ShowResult, error) {
	var show ShowResult
	err := c.get(fmt.Sprintf("/tv/%d", id), map[string]string{"language": "es-ES"}, &show)
	if err != nil {
		return nil, err
	}
	return &show, nil
}

func (c *Client) GetMovie(id int) (*MovieResult, error) {
	var movie MovieResult
	err := c.get(fmt.Sprintf("/movie/%d", id), map[string]string{"language": "es-ES"}, &movie)
	if err != nil {
		return nil, err
	}
	return &movie, nil
}

func (c *Client) GetSeason(showID, seasonNumber int) (*SeasonDetail, error) {
	var season SeasonDetail
	err := c.get(fmt.Sprintf("/tv/%d/season/%d", showID, seasonNumber), map[string]string{"language": "es-ES"}, &season)
	if err != nil {
		return nil, err
	}
	return &season, nil
}

// FindTVByName searches TMDB and returns the first match
func (c *Client) FindTVByName(name string) (*ShowResult, error) {
	results, err := c.SearchTV(name)
	if err != nil {
		return nil, err
	}
	// If no results, try stripping year suffix like "(2023)"
	if len(results) == 0 {
		cleaned := stripYearSuffix(name)
		if cleaned != name {
			results, err = c.SearchTV(cleaned)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results for %q", name)
	}
	// Return the full show details for the first match
	return c.GetTVShow(results[0].ID)
}

// stripYearSuffix removes trailing " (YYYY)" from a name
func stripYearSuffix(name string) string {
	if len(name) < 7 {
		return name
	}
	// Check for pattern " (XXXX)" at end
	if name[len(name)-1] == ')' {
		idx := len(name) - 7
		if idx >= 0 && name[idx:idx+2] == " (" {
			year := name[idx+2 : len(name)-1]
			allDigits := true
			for _, c := range year {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return name[:idx]
			}
		}
	}
	return name
}

// --- Image URL helpers ---

func PosterURL(path string, size string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "w342"
	}
	return fmt.Sprintf("%s/%s%s", imageBaseURL, size, path)
}

func BackdropURL(path string, size string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "w780"
	}
	return fmt.Sprintf("%s/%s%s", imageBaseURL, size, path)
}

// --- HTTP helper ---

func (c *Client) get(path string, params map[string]string, target any) error {
	u, _ := url.Parse(baseURL + path)
	q := u.Query()
	q.Set("api_key", c.apiKey)
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		return fmt.Errorf("tmdb request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: status %d for %s", resp.StatusCode, path)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// Enabled returns true if the client has an API key configured
func (c *Client) Enabled() bool {
	return c.apiKey != ""
}

// UpcomingEpisode represents the next airing episode for a show
type UpcomingEpisode struct {
	ShowID      int     `json:"show_id"`
	ShowName    string  `json:"show_name"`
	PosterURL   string  `json:"poster_url"`
	EpisodeName string  `json:"episode_name"`
	Season      int     `json:"season"`
	Episode     int     `json:"episode"`
	AirDate     string  `json:"air_date"`
	Overview    string  `json:"overview"`
}

// GetNextEpisodeToAir returns next airing episode info for a show
func (c *Client) GetNextEpisodeToAir(tmdbID int) (*UpcomingEpisode, error) {
	var resp struct {
		Name             string   `json:"name"`
		PosterPath       string   `json:"poster_path"`
		NextEpisodeToAir *Episode `json:"next_episode_to_air"`
	}
	err := c.get(fmt.Sprintf("/tv/%d", tmdbID), map[string]string{"language": "es-ES"}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.NextEpisodeToAir == nil {
		return nil, nil
	}
	ep := resp.NextEpisodeToAir
	return &UpcomingEpisode{
		ShowID:      tmdbID,
		ShowName:    resp.Name,
		PosterURL:   PosterURL(resp.PosterPath, "w154"),
		EpisodeName: ep.Name,
		Season:      ep.SeasonNumber,
		Episode:     ep.EpisodeNumber,
		AirDate:     ep.AirDate,
		Overview:    ep.Overview,
	}, nil
}
