# Go Quote Bot - Technical Documentation

## Project origin
I started this project on Debian Linux because I could not find a reliable, lightweight quote bot that ran well on Linux without heavy dependencies. Building my own ensured it remained simple, portable, and easy to hack on.

## Overview
Go-Quote is a Twitch IRC bot and companion CLI for capturing, searching, editing, and deleting stream quotes. Quotes are stored in a local SQLite database and can be managed from chat (`!quote` commands) or an interactive terminal mode for offline/local use.

## Architecture
- Entry point: `main.go` wires flags/env/config, creates the `QuoteStore`, and runs either Twitch or CLI mode.
- Configuration: `setup.go` merges defaults, a persisted `go-quote.config.json`, and environment variables, then writes the resolved config back to disk.
- Storage: `store.go` provides SQLite-backed CRUD, random selection, and helper methods. A single `quotes` table holds `id`, `text`, `author`, and `created_at`.
- Command handling: `commands.go` parses `!quote` subcommands and routes to the store. It returns response strings for Twitch or CLI to print.
- Twitch client: `twitch.go` configures the TLS IRC client, handles reconnect/backoff, and relays chat messages through `CommandHandler`.
- CLI mode: `cli.go` offers a prompt-driven interface that mirrors the Twitch commands for local testing or maintenance.

### Data flow
1) Input (Twitch chat or CLI prompt) arrives as a text command.
2) `CommandHandler` parses the subcommand and calls the appropriate `QuoteStore` method.
3) `QuoteStore` performs the SQLite operation and returns data/errors.
4) The caller (Twitch bot or CLI) formats responses for chat/stdout.

### Database schema
The SQLite table is created automatically:
```sql
CREATE TABLE IF NOT EXISTS quotes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  text TEXT NOT NULL,
  author TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## Requirements
- Go 1.24+ (per `go.mod`).
- No external services; SQLite is bundled via `github.com/mattn/go-sqlite3`.

## Setup
1) Clone
```bash
git clone https://github.com/SillyNickDev/Go-Quote
cd go-quote
```
2) Build
```bash
go build .
```
This produces `go-quote` (or `go-quote.exe` on Windows).

## Configuration
Config values are merged in this order: defaults -> config file -> environment -> CLI flags. The resolved config is written to `go-quote.config.json` after each run.
- Flags: `-mode` (twitch|cli, default `twitch`), `-db` (default `quotes.db`), `-user`, `-oauth` (`oauth:XXXX`), `-channel`.
- Environment (used when flags are empty): `GOQUOTE_MODE`, `GOQUOTE_DB`/`QUOTE_DB`, `GOQUOTE_USER`/`TWITCH_USER`, `GOQUOTE_OAUTH`/`TWITCH_OAUTH`/`TWITCH_TOKEN`/`OAUTH_TOKEN`, `GOQUOTE_CHANNEL`/`TWITCH_CHANNEL`.
- Keep `go-quote.config.json` and your OAuth token private.

### Twitch token
Generate an IRC token (e.g., https://antiscuff.com/oauth/) and pass it as `oauth:token` to `-oauth` or `GOQUOTE_OAUTH`.

## Running
### Twitch mode (connect to chat)
```bash
./go-quote -mode twitch -user your_bot_username -oauth oauth:token -channel channel
```
The bot joins the channel, listens for `!quote` commands, and replies in chat.

### CLI mode (local/offline)
```bash
./go-quote -mode cli
```
Follow the prompts to add, list, search, edit, or delete quotes without connecting to Twitch.

## Commands (Twitch chat)
- `!quote` - Return a random quote.
- `!quote add <quote>` - Add a quote attributed to the sender.
- `!quote add <author> | <quote>` - Add a quote for another author.
- `!quote search <term>` - Return the first match by text or author.
- `!quote get <id>` - Fetch a specific quote.
- `!quote list` - List the first five quotes.
- `!quote latest` - Show the most recently added quote.
- `!quote count` - Show how many quotes are stored.
- `!quote delete <id>` - Delete a quote (Twitch moderator only).
- `!quote edit <id> | <quote>` - Update quote text (Twitch moderator only).
- `!quote author <id> <author>` - Change quote author (Twitch moderator only).
- `!quote help` - Show command help.

CLI mode exposes the same operations via its menu.

## Data files
- `quotes.db` - SQLite database (path configurable with `-db`). Back it up to preserve quotes.
- `go-quote.config.json` - Persisted config written after each run; keep it private.

## Modifying the project
### Adding a new chat command
1) Update `commands.go` inside `Handle` with a new subcommand case.
2) Implement the behavior using `QuoteStore` or new logic.
3) Add a help line in `printHelp()`.
4) If the command requires CLI support, mirror it in `cli.go`.

### Changing the database shape
- To add a new field, extend the `Quote` struct in `store.go`, adjust table creation/migrations, and update scan/insert/update queries accordingly.
- Remember to handle existing databases (add migrations or defaults). Consider writing a small migration that checks for missing columns before altering the table.

### Tweaking Twitch behavior
- Rate limiting/retry: see `twitch.go` (`minRetryDelay`, `maxRetryDelay`, jittered backoff logic in `backoffDelay`).
- Moderator detection: `isModerator` treats broadcaster or moderator badges as privileged.

### Configuration defaults
- Adjust defaults in `setup.go` (`defaults` in `setup()` and environment variable names in `applyEnvDefaults`).

### Logging and error handling
- Twitch path logs to stdout via `log.Printf`. Expand or replace with structured logging if needed.
- CLI path prints user-friendly errors to stdout.

## Development tips
- Format: `gofmt` on modified files.
- Quick run: `go run . -mode cli` to exercise commands without Twitch.
- Manual test flow: add a few quotes, list, search, edit, delete, and verify count/latest.
- When touching persistence, test against a fresh `quotes.db` to ensure table creation works and against an existing DB to confirm compatibility.

## Release/packaging ideas
- Build per platform: `GOOS=linux GOARCH=amd64 go build .`, `GOOS=windows GOARCH=amd64 go build .`, etc.
- Distribute the binary with a sample `README.md` and instructions to generate a Twitch token.

## Troubleshooting
- "Quote handler is not configured": ensure `CommandHandler` is initialized with a non-nil `QuoteStore` (see `main.go`).
- "no quotes available": add at least one quote (`!quote add ...` or via CLI `add`).
- Twitch connect issues: verify `-user`, `-oauth` (prefixed with `oauth:`), and `-channel`; check network/firewall and retry.
- Permissions: delete/edit/author commands require Twitch moderator or broadcaster badges.

## Roadmap ideas
- Add pagination for `!quote list`.
- Add export/import to CSV or JSON.
- Add tests around `QuoteStore` and command parsing.
- Add per-channel configuration for multi-channel deployments.
