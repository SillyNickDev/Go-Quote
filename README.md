# Twitch Quote Bot (Go)

A lightweight Twitch IRC bot and CLI for capturing, searching, and managing stream quotes. Quotes are stored in a local SQLite database and can be managed either directly from Twitch chat or through an interactive terminal mode for offline testing.

---

## Features
- Chat-first `!quote` commands for add/search/random/get/list/latest/count, plus moderator-only edit/delete/author updates.
- Interactive CLI mode that mirrors Twitch commands for local management.
- SQLite-backed persistence (single file, no external services) with configurable database path.
- Automatic configuration merge and persistence to `go-quote.config.json`, so credentials only need to be entered once.
- TLS-enabled Twitch IRC client with reconnect and jittered backoff.
- Cross-platform binary; runs anywhere Go and SQLite3 are available.

## Prerequisites
- Go 1.24+ (per `go.mod`).
- SQLite is bundled via `github.com/mattn/go-sqlite3`; no separate install required.

## Install
1) Clone the repository
```bash
git clone https://github.com/yourusername/go-quote
cd go-quote
```

2) Build
```bash
go build .
```
The resulting `go-quote` (or `go-quote.exe` on Windows) binary is self contained.

---

## Configuration
The app merges values from CLI flags, environment variables, and the persisted `go-quote.config.json` file (written after each run):

- Flags: `-mode` (twitch|cli, default `twitch`), `-db` (default `quotes.db`), `-user`, `-oauth` (`oauth:XXXX`), `-channel`.
- Environment (used when flags are empty): `GOQUOTE_MODE`, `GOQUOTE_DB`/`QUOTE_DB`, `GOQUOTE_USER`/`TWITCH_USER`, `GOQUOTE_OAUTH`/`TWITCH_OAUTH`/`TWITCH_TOKEN`/`OAUTH_TOKEN`, `GOQUOTE_CHANNEL`/`TWITCH_CHANNEL`.
- Keep `go-quote.config.json` and your OAuth token private if you commit or share this repository.

---

## Usage
### Twitch mode
Connects to a channel and responds to chat commands.
```bash
./go-quote -mode twitch -user your_bot_username -oauth oauth:yourtoken -channel yourchannel
```
You can generate a Twitch IRC token from providers such as https://antiscuff.com/oauth/.

### CLI mode
Runs an interactive prompt for local quote management.
```bash
./go-quote -mode cli
```
Use the prompts to add, list, search, edit, or delete quotes without joining Twitch chat.

---

## Quote commands (Twitch chat)
- `!quote` — Return a random quote.
- `!quote add <quote>` — Add a quote attributed to the sender.
- `!quote add <author> | <quote>` — Add a quote for another author.
- `!quote search <term>` — Return the first match by text or author.
- `!quote get <id>` — Fetch a specific quote.
- `!quote list` — List the first five quotes.
- `!quote latest` — Show the most recently added quote.
- `!quote count` — Show how many quotes are stored.
- `!quote delete <id>` — Delete a quote (Twitch moderator only).
- `!quote edit <id> | <quote>` — Update quote text (Twitch moderator only).
- `!quote author <id> <author>` — Change quote author (Twitch moderator only).
- `!quote help` — Show command help.

CLI mode exposes the same operations through the interactive menu.

---

## Data files
- `quotes.db` — SQLite database storing all quotes (path configurable via `-db`).
- `go-quote.config.json` — Persisted configuration generated after each run.

## License
GPL — keep it open source and credit the project when used publicly.

## Contact
- Twitch: https://twitch.tv/ItsNickDoberman
- GitHub: https://github.com/sillynickdev5
- Bluesky: https://bsky.app/profile/nickdoberman.xyz
