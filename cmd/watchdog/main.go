package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mdaguete/watchlog/internal/auth"
	"github.com/mdaguete/watchlog/internal/db"
)

const usage = `watchdog - WatchLog administration CLI

Usage: watchdog --datadir <path> <command> [args...]

Options:
  --datadir <path>          Path to WatchLog data directory (required)

Commands:
  users                     List all users
  user-create <name> <pass> Create a new user
  user-passwd <name> <pass> Change user password
  user-block <name>         Block a user (disable login)
  user-unblock <name>       Unblock a user
  user-delete <name>        Delete a user and all their data
  user-email <name> <email> Set user email

  config                    Show all settings
  config-get <key>          Get a specific setting value
  config-set <key> <value>  Set a configuration value
  config-del <key>          Delete a configuration value

  db-info                   Show database info (version, size, users)
  db-vacuum                 Compact the database (VACUUM)

  netflix-dates <csv> [uid] Adjust episode watched dates from Netflix history
                            (dry-run by default; add --apply to write)
  sync-stats [uid]          Recalculate watch stats from the DB (all users if
                            no id); use after importing viewing history
  fill-aired <show> [uid]   Mark a show's aired episodes watched by air date
                            (fills gaps from numbering mismatches; --apply)

Examples:
  watchdog --datadir /data users
  watchdog --datadir /data user-create admin mypassword
  watchdog --datadir /data config-set auth_registration disabled
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	// Parse --datadir flag
	var dataDir string
	var cmdArgs []string
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--datadir" && i+1 < len(os.Args) {
			dataDir = os.Args[i+1]
			i++
		} else {
			cmdArgs = append(cmdArgs, os.Args[i])
		}
	}

	if dataDir == "" {
		fmt.Fprintln(os.Stderr, "Error: --datadir is required")
		fmt.Print(usage)
		os.Exit(1)
	}

	if len(cmdArgs) == 0 {
		fmt.Print(usage)
		os.Exit(1)
	}

	command := cmdArgs[0]
	args := cmdArgs[1:]

	dbPath := filepath.Join(dataDir, "watchlog.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: database not found at %s\n", dbPath)
		os.Exit(1)
	}

	database, err := db.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	switch command {
	case "users":
		cmdUsers(database)
	case "user-create":
		requireArgs(args, 2, "user-create <name> <password>")
		cmdUserCreate(database, args[0], args[1])
	case "user-passwd":
		requireArgs(args, 2, "user-passwd <name> <password>")
		cmdUserPasswd(database, args[0], args[1])
	case "user-block":
		requireArgs(args, 1, "user-block <name>")
		cmdUserBlock(database, args[0], true)
	case "user-unblock":
		requireArgs(args, 1, "user-unblock <name>")
		cmdUserBlock(database, args[0], false)
	case "user-delete":
		requireArgs(args, 1, "user-delete <name>")
		cmdUserDelete(database, args[0])
	case "user-email":
		requireArgs(args, 2, "user-email <name> <email>")
		cmdUserEmail(database, args[0], args[1])
	case "config":
		cmdConfig(database)
	case "config-get":
		requireArgs(args, 1, "config-get <key>")
		cmdConfigGet(database, args[0])
	case "config-set":
		requireArgs(args, 2, "config-set <key> <value>")
		cmdConfigSet(database, args[0], args[1])
	case "config-del":
		requireArgs(args, 1, "config-del <key>")
		cmdConfigSet(database, args[0], "")
	case "db-info":
		cmdDBInfo(database, dbPath)
	case "db-vacuum":
		cmdDBVacuum(database)
	case "netflix-dates":
		requireArgs(args, 1, "netflix-dates <csv-path> [user-id] [--apply]")
		csvPath := args[0]
		uid := int64(1)
		apply := false
		for _, a := range args[1:] {
			if a == "--apply" {
				apply = true
			} else if n, err := strconv.ParseInt(a, 10, 64); err == nil {
				uid = n
			}
		}
		cmdNetflixDates(database, csvPath, uid, apply)
	case "sync-stats":
		uid := int64(0) // 0 = all users
		if len(args) > 0 {
			if n, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				uid = n
			}
		}
		cmdSyncStats(database, uid)
	case "fill-aired":
		requireArgs(args, 1, "fill-aired <show-id> [user-id] [--apply]")
		showID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid show id: %s\n", args[0])
			os.Exit(1)
		}
		uid := int64(1)
		apply := false
		for _, a := range args[1:] {
			if a == "--apply" {
				apply = true
			} else if n, err := strconv.ParseInt(a, 10, 64); err == nil {
				uid = n
			}
		}
		cmdFillAired(database, showID, uid, apply)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		fmt.Print(usage)
		os.Exit(1)
	}
}

func requireArgs(args []string, n int, usage string) {
	if len(args) < n {
		fmt.Fprintf(os.Stderr, "Usage: watchdog <datadir> %s\n", usage)
		os.Exit(1)
	}
}

func cmdUsers(database *db.DB) {
	users, err := database.ListAllUsers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-4s %-20s %-30s %-8s\n", "ID", "Username", "Email", "Blocked")
	fmt.Println(strings.Repeat("-", 70))
	for _, u := range users {
		blocked := ""
		if u.Blocked {
			blocked = "YES"
		}
		fmt.Printf("%-4d %-20s %-30s %-8s\n", u.ID, u.Username, u.Email, blocked)
	}
}

func cmdUserCreate(database *db.DB, username, password string) {
	if len(password) < 8 {
		fmt.Fprintln(os.Stderr, "Error: password must be at least 8 characters")
		os.Exit(1)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	id, err := database.CreateUser(username, hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("User created: %s (id=%d)\n", username, id)
}

func cmdUserPasswd(database *db.DB, username, password string) {
	if len(password) < 8 {
		fmt.Fprintln(os.Stderr, "Error: password must be at least 8 characters")
		os.Exit(1)
	}
	user, err := database.GetUserByUsername(username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: user %q not found\n", username)
		os.Exit(1)
	}
	hash, _ := auth.HashPassword(password)
	database.UpdateUserPassword(user.ID, hash)
	fmt.Printf("Password updated for %s\n", username)
}

func cmdUserBlock(database *db.DB, username string, block bool) {
	user, err := database.GetUserByUsername(username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: user %q not found\n", username)
		os.Exit(1)
	}
	database.SetUserBlocked(user.ID, block)
	if block {
		fmt.Printf("User %s blocked\n", username)
	} else {
		fmt.Printf("User %s unblocked\n", username)
	}
}

func cmdUserDelete(database *db.DB, username string) {
	user, err := database.GetUserByUsername(username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: user %q not found\n", username)
		os.Exit(1)
	}
	database.DeleteUser(user.ID)
	fmt.Printf("User %s deleted (id=%d)\n", username, user.ID)
}

func cmdUserEmail(database *db.DB, username, email string) {
	user, err := database.GetUserByUsername(username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: user %q not found\n", username)
		os.Exit(1)
	}
	database.UpdateUserEmail(user.ID, email)
	fmt.Printf("Email updated for %s: %s\n", username, email)
}

func cmdConfig(database *db.DB) {
	settings, err := database.ListSettings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-30s %s\n", "Key", "Value")
	fmt.Println(strings.Repeat("-", 60))
	for _, s := range settings {
		// Mask sensitive values
		val := s.Value
		if strings.Contains(s.Key, "key") || strings.Contains(s.Key, "password") || strings.Contains(s.Key, "smtp") {
			if len(val) > 8 {
				val = val[:4] + "..." + val[len(val)-4:]
			}
		}
		fmt.Printf("%-30s %s\n", s.Key, val)
	}
}

func cmdConfigSet(database *db.DB, key, value string) {
	database.SetSetting(key, value)
	fmt.Printf("Set %s = %s\n", key, value)
}

func cmdConfigGet(database *db.DB, key string) {
	value := database.GetSetting(key)
	if value == "" {
		fmt.Printf("%s: (not set)\n", key)
	} else {
		fmt.Printf("%s = %s\n", key, value)
	}
}

func cmdDBInfo(database *db.DB, dbPath string) {
	info, _ := os.Stat(dbPath)
	version := database.CurrentVersion()
	users, _ := database.ListAllUsers()
	fmt.Printf("Database: %s\n", dbPath)
	fmt.Printf("Size:     %.2f MB\n", float64(info.Size())/(1024*1024))
	fmt.Printf("Version:  %d\n", version)
	fmt.Printf("Users:    %d\n", len(users))
}

func cmdDBVacuum(database *db.DB) {
	database.Vacuum()
	fmt.Println("Database vacuumed successfully")
}

func init() {
	// Suppress migration logs when using CLI
	_ = strconv.Atoi // ensure strconv is used
}
