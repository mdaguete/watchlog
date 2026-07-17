# watchdog - WatchLog Admin CLI

Command-line tool for administering a WatchLog instance. Operates directly on the SQLite database — no running server required.

## Installation

The `watchdog` binary is included in:
- Docker image (at `/usr/local/bin/watchdog`)
- GoReleaser builds (`make` or `make watchdog`)

## Usage

```
watchdog --datadir <path> <command> [args...]
```

The `--datadir` flag points to the directory containing `watchlog.db`.

## Commands

### User Management

```bash
# List all users
watchdog --datadir /data users

# Create a new user
watchdog --datadir /data user-create <username> <password>

# Change password (min 8 characters)
watchdog --datadir /data user-passwd <username> <new-password>

# Set user email
watchdog --datadir /data user-email <username> <email>

# Block a user (prevents login)
watchdog --datadir /data user-block <username>

# Unblock a user
watchdog --datadir /data user-unblock <username>

# Delete a user and ALL their data (episodes, movies, lists, etc.)
watchdog --datadir /data user-delete <username>
```

### Configuration

```bash
# Show all settings
watchdog --datadir /data config

# Get a specific setting
watchdog --datadir /data config-get <key>

# Set a value
watchdog --datadir /data config-set <key> <value>

# Delete a value (set to empty)
watchdog --datadir /data config-del <key>
```

#### Common Settings

| Key | Values | Description |
|-----|--------|-------------|
| `auth_registration` | `enabled` / `disabled` | Allow new user registration |
| `auth_password` | `enabled` / `disabled` | Allow password login |
| `auth_magic_link` | `enabled` / `disabled` | Allow magic link login |
| `auth_default_login` | `password` / `magic` | Default login form |
| `setup_complete` | `true` / `false` | Setup wizard completed |
| `tmdb_api_key` | API key | TMDB API key |
| `smtp_url` | URL | SMTP connection string |
| `watchlog_url` | URL | Public URL for email links |

### Database

```bash
# Show database info (path, size, schema version, user count)
watchdog --datadir /data db-info

# Compact the database (reclaim space after deletions)
watchdog --datadir /data db-vacuum
```

### Viewing History

```bash
# Adjust episode/movie watched dates from a Netflix ViewingHistory.csv.
# Dry-run by default (shows what would change); add --apply to write.
# A database backup is created before the first change when applying.
watchdog --datadir /data netflix-dates /path/to/NetflixViewingHistory.csv
watchdog --datadir /data netflix-dates /path/to/NetflixViewingHistory.csv 1 --apply

# Recalculate watch stats from the database (all users, or a single user id).
# Idempotent; useful after importing viewing history.
watchdog --datadir /data sync-stats
watchdog --datadir /data sync-stats 1

# Mark a show's already-aired episodes as watched, dated by air date, filling
# gaps caused by season/episode numbering mismatches (e.g. TMDB parts vs
# TVTime). Never overwrites episodes already watched. Dry-run by default;
# --apply backs up the DB and recomputes stats.
watchdog --datadir /data fill-aired 21
watchdog --datadir /data fill-aired 21 1 --apply
```

## Docker Usage

```bash
# Run watchdog inside the container
docker exec watchlog watchdog --datadir /data users

# Or with a one-off container
docker run --rm -v watchlog-data:/data watchlog watchdog --datadir /data db-info
```

## Common Scenarios

### Forgot admin password
```bash
watchdog --datadir /data user-passwd admin newpassword123
```

### Disable registration after setup
```bash
watchdog --datadir /data config-set auth_registration disabled
```

### Unblock a locked-out user
```bash
watchdog --datadir /data user-unblock username
```

### Check database health
```bash
watchdog --datadir /data db-info
watchdog --datadir /data db-vacuum
```
