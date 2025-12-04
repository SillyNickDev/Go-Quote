package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
)

// main parses command-line flags, initializes application state (QuoteStore and CommandHandler),
// and runs either the CLI or the Twitch bot based on the -mode flag.
// It creates a context canceled on OS interrupt/SIGTERM to allow graceful shutdown.
func main() {
	var (
		dbPath        string
		twitchUser    string
		twitchOAuth   string
		twitchChannel string
		mode          string
	)
	flag.StringVar(&dbPath, "db", "quotes.db", "Path to SQLite database file")
	flag.StringVar(&twitchUser, "user", "", "Twitch bot username")
	flag.StringVar(&twitchOAuth, "oauth", "", "Twitch OAuth token (format: oauth:xxxx)")
	flag.StringVar(&twitchChannel, "channel", "", "Twitch channel to join")
	flag.StringVar(&mode, "mode", "twitch", "Mode: twitch or cli")
	flag.Parse()
	applyEnvDefaults(&mode, &dbPath, &twitchUser, &twitchOAuth, &twitchChannel)

	config, err := setup(mode, dbPath, twitchUser, twitchOAuth, twitchChannel)
	if err != nil {
		log.Fatalf("Error during setup: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := NewQuoteStore(ctx, config.DBPath)
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	defer store.Close()

	handler := NewCommandHandler(store)

	switch strings.ToLower(config.Mode) {
	case "cli":
		runCLI(ctx, store, handler)
	case "twitch":
		if err := validateTwitchConfig(config.TwitchUser, config.TwitchOAuth, config.TwitchChannel); err != nil {
			log.Fatal(err)
		}
		client := configureTwitchClient(config.TwitchUser, config.TwitchOAuth)
		bot := NewTwitchBot(client, handler, config.TwitchChannel)
		log.Printf("Connecting to Twitch channel #%s as %s...", config.TwitchChannel, config.TwitchUser)
		if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("Error running Twitch bot: %v", err)
		}
	default:
		log.Fatalf("Unknown mode: %s. Use 'twitch' or 'cli'.", config.Mode)
	}
}
