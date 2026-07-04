package mcpserver

import (
	"context"
	"fmt"
	"time"

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
}

// --- Read tools ---

type emptyArgs struct{}

type showIDArgs struct {
	ShowID int64 `json:"show_id"`
}

func (s *Server) toolGetContinueWatching(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	if !hasScope(ctx, "read") {
		return errNoScope("read")
	}
	userID := getUserID(ctx)
	items, _ := s.db.GetContinueWatching(userID, 10)
	newSeasons, _ := s.db.GetNewSeasons(userID)

	result := map[string]any{
		"continue_watching": items,
		"new_seasons":       newSeasons,
	}
	return jsonText(result), nil, nil
}

func (s *Server) toolGetShows(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	if !hasScope(ctx, "read") {
		return errNoScope("read")
	}
	userID := getUserID(ctx)
	shows, _ := s.db.GetUserShowsFiltered(userID, "name", "")

	type showSummary struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Status   string `json:"status"`
		Episodes int    `json:"episodes_seen"`
		Favorite bool   `json:"favorite"`
		Archived bool   `json:"archived"`
	}
	var summaries []showSummary
	for _, show := range shows {
		summaries = append(summaries, showSummary{
			ID: show.ID, Name: show.Name, Status: show.Status,
			Episodes: show.EpisodesSeen, Favorite: show.IsFavorited, Archived: show.IsArchived,
		})
	}
	return jsonText(summaries), nil, nil
}

func (s *Server) toolGetShow(ctx context.Context, req *mcp.CallToolRequest, args showIDArgs) (*mcp.CallToolResult, any, error) {
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
	if !hasScope(ctx, "read") {
		return errNoScope("read")
	}
	userID := getUserID(ctx)
	movies, _ := s.db.GetUserMoviesSorted(userID, "name")

	type movieSummary struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Genres  string `json:"genres"`
		Runtime int    `json:"runtime"`
	}
	var summaries []movieSummary
	for _, m := range movies {
		summaries = append(summaries, movieSummary{ID: m.ID, Name: m.Name, Genres: m.Genres, Runtime: m.Runtime})
	}
	return jsonText(summaries), nil, nil
}

func (s *Server) toolGetStats(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
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
	if !hasScope(ctx, "mark") {
		return errNoScope("mark")
	}
	userID := getUserID(ctx)
	s.db.SnoozeShow(userID, args.ShowID, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Show snoozed"}},
	}, nil, nil
}
