# MCP Integration

WatchLog exposes a [Model Context Protocol](https://modelcontextprotocol.io/) server for AI agent integration.

## Setup

1. Go to **Settings** in WatchLog
2. Create an API key with the scopes you need
3. Copy the key (shown once only)
4. Configure your AI agent (see below)

## Endpoint

```
POST https://your-watchlog-url/mcp
Authorization: Bearer wl_your_api_key_here
```

## Scopes

| Scope | Description |
|-------|-------------|
| `read` | Get shows, movies, stats, continue watching |
| `mark` | Mark/unmark episodes, mark seasons, snooze shows |
| `write` | Add shows and movies (future) |
| `lists` | Manage lists (future) |
| `admin` | TMDB refresh, settings (future) |

## Available Tools

### Read (scope: `read`)
| Tool | Description |
|------|-------------|
| `get_continue_watching` | Next episodes to watch + new seasons |
| `get_shows` | All followed TV shows |
| `get_show` | Show details by ID (`show_id`) |
| `get_movies` | All watched movies |
| `get_stats` | Watch statistics |

### Mark (scope: `mark`)
| Tool | Description | Parameters |
|------|-------------|------------|
| `mark_episode` | Mark episode watched | `show_id`, `season`, `episode` |
| `unmark_episode` | Unmark episode | `show_id`, `season`, `episode` |
| `mark_season` | Mark full season | `show_id`, `season`, `episodes` |
| `snooze_show` | Hide from continue watching | `show_id` |

## Agent Configuration

### Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "watchlog": {
      "url": "https://your-watchlog-url/mcp",
      "headers": {
        "Authorization": "Bearer wl_your_api_key_here"
      }
    }
  }
}
```

### Cursor

In Cursor settings (Settings → MCP Servers → Add):

- **Name**: WatchLog
- **Transport**: HTTP
- **URL**: `https://your-watchlog-url/mcp`
- **Headers**: `Authorization: Bearer wl_your_api_key_here`

Or in `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "watchlog": {
      "url": "https://your-watchlog-url/mcp",
      "headers": {
        "Authorization": "Bearer wl_your_api_key_here"
      }
    }
  }
}
```

### Windsurf

In `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "watchlog": {
      "serverUrl": "https://your-watchlog-url/mcp",
      "headers": {
        "Authorization": "Bearer wl_your_api_key_here"
      }
    }
  }
}
```

### Kiro

In `.kiro/settings/mcp.json`:

```json
{
  "mcpServers": {
    "watchlog": {
      "url": "https://your-watchlog-url/mcp",
      "headers": {
        "Authorization": "Bearer wl_your_api_key_here"
      }
    }
  }
}
```

### Generic (any MCP client)

The server uses **Streamable HTTP** transport. Connect to the `/mcp` endpoint with:
- Method: `POST`
- Content-Type: `application/json`
- Accept: `application/json, text/event-stream`
- Authorization: `Bearer <api_key>`

## API Key Management

- **Create**: POST `/api/keys` with `key_name` and `scopes` (comma-separated)
- **Delete**: DELETE `/api/keys/{id}`
- **List**: Available in Settings page

Keys are prefixed with `wl_` for easy identification.
