package mcpserver

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mdaguete/watchlog/internal/models"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) registerTools() {
	// Read tools
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_continue_watching",
		Description: "Get the list of next episodes to watch (continue watching + new seasons)",
	}, s.toolGetContinueWatching)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_shows",
		Description: "Get all followed TV shows",
	}, s.toolGetShows)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_show",
		Description: "Get details of a specific TV show by ID",
	}, s.toolGetShow)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_movies",
		Description: "Get all watched movies",
	}, s.toolGetMovies)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_stats",
		Description: "Get watch statistics (shows, episodes, movies, runtime)",
	}, s.toolGetStats)

	// Write tools
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "mark_episode",
		Description: "Mark an episode as watched",
	}, s.toolMarkEpisode)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "unmark_episode",
		Description: "Unmark an episode (mark as not watched)",
	}, s.toolUnmarkEpisode)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "mark_season",
		Description: "Mark all episodes of a season as watched",
	}, s.toolMarkSeason)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "snooze_show",
		Description: "Snooze a show from continue watching until next episode is marked",
	}, s.toolSnoozeShow)

	// Write tools
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search_shows",
		Description: "Search TMDB for TV shows by name. Returns matches with tmdb_id to use with add_show.",
	}, s.toolSearchShows)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "add_show",
		Description: "Add a TV show from TMDB to your library by its tmdb_id",
	}, s.toolAddShow)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "search_movies",
		Description: "Search TMDB for movies by name. Returns matches with tmdb_id to use with add_movie.",
	}, s.toolSearchMovies)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "add_movie",
		Description: "Add a movie from TMDB to your watchlist (does not mark as watched)",
	}, s.toolAddMovie)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "mark_movie_watched",
		Description: "Mark a movie as watched",
	}, s.toolMarkMovieWatched)
}

// --- Read tools ---

type emptyArgs struct{}

type showIDArgs struct {
	ShowID int64 `json:"show_id"`
}

func (s *Server) toolGetContinueWatching(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=get_continue_watching user=%d", getUserID(ctx))
	if !hasScope(ctx, "read") {
		return errNoScope("read")
	}
	userID := getUserID(ctx)
	items, _ := s.db.GetContinueWatching(userID, 5)
	newSeasons, _ := s.db.GetNewSeasons(userID)

	type epSummary struct {
		ShowID   int64  `json:"show_id"`
		ShowName string `json:"show_name"`
		Season   int    `json:"season"`
		Episode  int    `json:"episode"`
		EpName   string `json:"episode_name"`
	}
	var cw []epSummary
	for _, item := range items {
		cw = append(cw, epSummary{ShowID: item.ShowID, ShowName: item.ShowName, Season: item.SeasonNumber, Episode: item.EpisodeNumber, EpName: item.EpName})
	}
	var ns []epSummary
	for _, item := range newSeasons {
		ns = append(ns, epSummary{ShowID: item.ShowID, ShowName: item.ShowName, Season: item.SeasonNumber, Episode: item.EpisodeNumber, EpName: item.EpName})
	}

	result := map[string]any{
		"continue_watching": cw,
		"new_seasons":       ns,
	}
	return jsonText(result), nil, nil
}

func (s *Server) toolGetShows(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=get_shows user=%d", getUserID(ctx))
	if !hasScope(ctx, "read") {
		return errNoScope("read")
	}
	userID := getUserID(ctx)
	shows, _ := s.db.GetUserShowsFiltered(userID, "name", "")

	type showSummary struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	var summaries []showSummary
	for _, show := range shows {
		summaries = append(summaries, showSummary{ID: show.ID, Name: show.Name, Status: show.Status})
	}
	return jsonText(summaries), nil, nil
}

func (s *Server) toolGetShow(ctx context.Context, req *mcp.CallToolRequest, args showIDArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=get_show user=%d show_id=%d", getUserID(ctx), args.ShowID)
	if !hasScope(ctx, "read") {
		return errNoScope("read")
	}
	userID := getUserID(ctx)
	show, err := s.db.GetUserShow(userID, args.ShowID)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "show not found"}},
			IsError: true,
		}, nil, nil
	}
	return jsonText(show), nil, nil
}

func (s *Server) toolGetMovies(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=get_movies user=%d", getUserID(ctx))
	if !hasScope(ctx, "read") {
		return errNoScope("read")
	}
	userID := getUserID(ctx)
	movies, _ := s.db.GetUserMoviesSorted(userID, "name")

	type movieSummary struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	var summaries []movieSummary
	for _, m := range movies {
		summaries = append(summaries, movieSummary{ID: m.ID, Name: m.Name})
	}
	return jsonText(summaries), nil, nil
}

func (s *Server) toolGetStats(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=get_stats user=%d", getUserID(ctx))
	if !hasScope(ctx, "read") {
		return errNoScope("read")
	}
	userID := getUserID(ctx)
	stats, _ := s.db.GetDashboardStats(userID)
	return jsonText(stats), nil, nil
}

// --- Write tools ---

type markEpisodeArgs struct {
	ShowID  int64 `json:"show_id"`
	Season  int   `json:"season"`
	Episode int   `json:"episode"`
}

type markSeasonArgs struct {
	ShowID   int64 `json:"show_id"`
	Season   int   `json:"season"`
	Episodes int   `json:"episodes"`
}

func (s *Server) toolMarkEpisode(ctx context.Context, req *mcp.CallToolRequest, args markEpisodeArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=mark_episode user=%d show=%d S%02dE%02d", getUserID(ctx), args.ShowID, args.Season, args.Episode)
	if !hasScope(ctx, "mark") {
		return errNoScope("mark")
	}
	userID := getUserID(ctx)
	s.db.MarkEpisodeWatched(userID, args.ShowID, args.Season, args.Episode)
	s.db.IncrementWatchStats(userID, 1)
	s.db.UnsnoozeShow(userID, args.ShowID)
	s.db.AutoArchiveIfComplete(userID, args.ShowID)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Marked S%02dE%02d as watched", args.Season, args.Episode)}},
	}, nil, nil
}

func (s *Server) toolUnmarkEpisode(ctx context.Context, req *mcp.CallToolRequest, args markEpisodeArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=unmark_episode user=%d show=%d S%02dE%02d", getUserID(ctx), args.ShowID, args.Season, args.Episode)
	if !hasScope(ctx, "mark") {
		return errNoScope("mark")
	}
	userID := getUserID(ctx)
	s.db.UnmarkEpisodeWatched(userID, args.ShowID, args.Season, args.Episode)
	s.db.AutoUnarchiveIfIncomplete(userID, args.ShowID)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Unmarked S%02dE%02d", args.Season, args.Episode)}},
	}, nil, nil
}

func (s *Server) toolMarkSeason(ctx context.Context, req *mcp.CallToolRequest, args markSeasonArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=mark_season user=%d show=%d S%02d", getUserID(ctx), args.ShowID, args.Season)
	if !hasScope(ctx, "mark") {
		return errNoScope("mark")
	}
	userID := getUserID(ctx)
	marked, _ := s.db.MarkSeasonWatched(userID, args.ShowID, args.Season, args.Episodes)
	if marked > 0 {
		s.db.IncrementWatchStats(userID, marked)
	}
	s.db.AutoArchiveIfComplete(userID, args.ShowID)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Marked %d episodes in season %d", marked, args.Season)}},
	}, nil, nil
}

func (s *Server) toolSnoozeShow(ctx context.Context, req *mcp.CallToolRequest, args showIDArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=snooze_show user=%d show=%d", getUserID(ctx), args.ShowID)
	if !hasScope(ctx, "mark") {
		return errNoScope("mark")
	}
	userID := getUserID(ctx)
	s.db.SnoozeShow(userID, args.ShowID, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Show snoozed"}},
	}, nil, nil
}

// --- Write tools ---

type searchArgs struct {
	Query string `json:"query"`
}

type addShowArgs struct {
	TMDBID int `json:"tmdb_id"`
}

func (s *Server) toolSearchShows(ctx context.Context, req *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=search_shows user=%d query=%q", getUserID(ctx), args.Query)
	if !hasScope(ctx, "write") {
		return errNoScope("write")
	}
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "TMDB not configured"}},
			IsError: true,
		}, nil, nil
	}
	results, err := s.tmdb.SearchTV(args.Query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("search error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	type result struct {
		TMDBID   int    `json:"tmdb_id"`
		Name     string `json:"name"`
		Year     string `json:"year"`
		Overview string `json:"overview"`
	}
	var matches []result
	for i, r := range results {
		if i >= 5 {
			break
		}
		year := ""
		if len(r.FirstAirDate) >= 4 {
			year = r.FirstAirDate[:4]
		}
		overview := r.Overview
		if len(overview) > 100 {
			overview = overview[:100] + "..."
		}
		matches = append(matches, result{TMDBID: r.ID, Name: r.Name, Year: year, Overview: overview})
	}
	return jsonText(matches), nil, nil
}

func (s *Server) toolAddShow(ctx context.Context, req *mcp.CallToolRequest, args addShowArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=add_show user=%d tmdb_id=%d", getUserID(ctx), args.TMDBID)
	if !hasScope(ctx, "write") {
		return errNoScope("write")
	}
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "TMDB not configured"}},
			IsError: true,
		}, nil, nil
	}
	userID := getUserID(ctx)

	// Fetch show details from TMDB
	show, err := s.tmdb.GetTVShow(args.TMDBID)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("TMDB error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	// Upsert show in catalog
	showID, _ := s.db.UpsertShow(models.Show{
		ExternalID: int64(show.ID),
		Name:       show.Name,
		TMDBID:     show.ID,
	})
	if showID == 0 {
		// Already exists, get ID
		s.db.GetShowByTMDBID(show.ID, &showID)
	}
	if showID == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "failed to add show"}},
			IsError: true,
		}, nil, nil
	}

	// Follow the show
	s.db.FollowShow(userID, showID)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Added '%s' (id=%d)", show.Name, showID)}},
	}, nil, nil
}

func (s *Server) toolSearchMovies(ctx context.Context, req *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=search_movies user=%d query=%q", getUserID(ctx), args.Query)
	if !hasScope(ctx, "write") {
		return errNoScope("write")
	}
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "TMDB not configured"}},
			IsError: true,
		}, nil, nil
	}
	results, err := s.tmdb.SearchMovie(args.Query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("search error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	type result struct {
		TMDBID   int    `json:"tmdb_id"`
		Title    string `json:"title"`
		Year     string `json:"year"`
		Overview string `json:"overview"`
	}
	var matches []result
	for i, r := range results {
		if i >= 5 {
			break
		}
		year := ""
		if len(r.ReleaseDate) >= 4 {
			year = r.ReleaseDate[:4]
		}
		overview := r.Overview
		if len(overview) > 100 {
			overview = overview[:100] + "..."
		}
		matches = append(matches, result{TMDBID: r.ID, Title: r.Title, Year: year, Overview: overview})
	}
	return jsonText(matches), nil, nil
}

type addMovieArgs struct {
	TMDBID int `json:"tmdb_id"`
}

func (s *Server) toolAddMovie(ctx context.Context, req *mcp.CallToolRequest, args addMovieArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=add_movie user=%d tmdb_id=%d", getUserID(ctx), args.TMDBID)
	if !hasScope(ctx, "write") {
		return errNoScope("write")
	}
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "TMDB not configured"}},
			IsError: true,
		}, nil, nil
	}
	userID := getUserID(ctx)

	movie, err := s.tmdb.GetMovie(args.TMDBID)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("TMDB error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	genres := make([]string, len(movie.Genres))
	for i, g := range movie.Genres {
		genres[i] = g.Name
	}
	genreStr := ""
	if len(genres) > 0 {
		genreStr = genres[0]
		for _, g := range genres[1:] {
			genreStr += ", " + g
		}
	}

	id, _ := s.db.AddMovieFromTMDB(movie.ID, movie.Title, "", movie.Overview, genreStr, movie.Runtime)
	s.db.AddMovieToLibrary(userID, id)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Added to watchlist: '%s' (id=%d)", movie.Title, id)}},
	}, nil, nil
}

type movieIDArgs struct {
	MovieID int64 `json:"movie_id"`
}

func (s *Server) toolMarkMovieWatched(ctx context.Context, req *mcp.CallToolRequest, args movieIDArgs) (*mcp.CallToolResult, any, error) {
	log.Printf("MCP: tool=mark_movie_watched user=%d movie_id=%d", getUserID(ctx), args.MovieID)
	if !hasScope(ctx, "mark") {
		return errNoScope("mark")
	}
	userID := getUserID(ctx)
	s.db.MarkMovieWatched(userID, args.MovieID, time.Now())
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Movie marked as watched"}},
	}, nil, nil
}
