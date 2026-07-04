package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mdaguete/watchlog/internal/db"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server with WatchLog integration.
type Server struct {
	db     *db.DB
	server *mcp.Server
}

// New creates a new MCP server with all available tools.
func New(database *db.DB) *Server {
	s := &Server{db: database}
	s.server = mcp.NewServer(
		&mcp.Implementation{Name: "watchlog", Version: "0.12.0"},
		nil,
	)
	s.registerTools()
	return s
}

// Handler returns an http.Handler that serves the MCP protocol.
// It wraps the streamable HTTP handler with API key authentication.
func (s *Server) Handler() http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		// The server is shared; auth is checked in the middleware wrapper
		return s.server
	}, nil)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Authenticate via Bearer token (API key)
		key := extractAPIKey(r)
		if key == "" {
			http.Error(w, `{"error":"missing api key"}`, http.StatusUnauthorized)
			return
		}
		userID, scopes, ok := s.db.ValidateAPIKey(key)
		if !ok {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}
		// Store auth context for tool handlers
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		ctx = context.WithValue(ctx, ctxScopes, scopes)
		mcpHandler.ServeHTTP(w, r.WithContext(ctx))
	})
}

type contextKey string

const (
	ctxUserID contextKey = "mcp_user_id"
	ctxScopes contextKey = "mcp_scopes"
)

func getUserID(ctx context.Context) int64 {
	if v, ok := ctx.Value(ctxUserID).(int64); ok {
		return v
	}
	return 0
}

func getScopes(ctx context.Context) string {
	if v, ok := ctx.Value(ctxScopes).(string); ok {
		return v
	}
	return ""
}

func hasScope(ctx context.Context, scope string) bool {
	scopes := getScopes(ctx)
	for _, s := range strings.Split(scopes, ",") {
		if strings.TrimSpace(s) == scope {
			return true
		}
	}
	return false
}

func extractAPIKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func errNoScope(scope string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("error: missing scope '%s'", scope)}},
		IsError: true,
	}, nil, nil
}

func jsonText(v any) *mcp.CallToolResult {
	data, _ := json.MarshalIndent(v, "", "  ")
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}
}
