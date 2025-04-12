package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gempir/go-twitch-irc/v2"
	_ "github.com/mattn/go-sqlite3"
)

// Quote holds the quote details.
type Quote struct {
	ID        int
	Text      string
	Author    string
	CreatedAt time.Time
}

var db *sql.DB

// initDB opens (or creates) the SQLite database and creates the quotes table if needed.
func initDB(dbPath string) error {
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("opening db: %w", err)
	}
	// Limit open connections (SQLite is not highly concurrent)
	db.SetMaxOpenConns(1)

	// Create table if it doesn't exist.
	query := `CREATE TABLE IF NOT EXISTS quotes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		text TEXT NOT NULL,
		author TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("creating table: %w", err)
	}
	return nil
}

// addQuote inserts a new quote into the database.
func addQuote(text, author string) (int64, error) {
	// Validate that the quote text is not empty.
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("quote text cannot be empty")
	}
	stmt, err := db.Prepare("INSERT INTO quotes(text, author) VALUES(?, ?)")
	if err != nil {
		return 0, fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	res, err := stmt.Exec(text, author)
	if err != nil {
		return 0, fmt.Errorf("executing insert: %w", err)
	}
	return res.LastInsertId()
}

// getRandomQuote returns a random quote.
func getRandomQuote() (*Quote, error) {
	row := db.QueryRow("SELECT id, text, author, created_at FROM quotes ORDER BY RANDOM() LIMIT 1")
	var q Quote
	var created string
	if err := row.Scan(&q.ID, &q.Text, &q.Author, &created); err != nil {
		return nil, fmt.Errorf("scanning random quote: %w", err)
	}
	parsedTime, err := time.Parse("2006-01-02 15:04:05", created)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}
	q.CreatedAt = parsedTime
	return &q, nil
}

// searchQuotes returns quotes that match the given search term.
func searchQuotes(term string) ([]Quote, error) {
	term = "%" + term + "%"
	rows, err := db.Query("SELECT id, text, author, created_at FROM quotes WHERE text LIKE ? OR author LIKE ?", term, term)
	if err != nil {
		return nil, fmt.Errorf("querying quotes: %w", err)
	}
	defer rows.Close()

	var quotes []Quote
	for rows.Next() {
		var q Quote
		var created string
		if err := rows.Scan(&q.ID, &q.Text, &q.Author, &created); err != nil {
			continue // Skip bad rows but log if necessary.
		}
		parsedTime, err := time.Parse("2006-01-02 15:04:05", created)
		if err != nil {
			continue
		}
		q.CreatedAt = parsedTime
		quotes = append(quotes, q)
	}
	return quotes, nil
}

// listQuotes retrieves all quotes (ordered by ID).
func listQuotes() ([]Quote, error) {
	rows, err := db.Query("SELECT id, text, author, created_at FROM quotes ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("listing quotes: %w", err)
	}
	defer rows.Close()

	var quotes []Quote
	for rows.Next() {
		var q Quote
		var created string
		if err := rows.Scan(&q.ID, &q.Text, &q.Author, &created); err != nil {
			continue
		}
		parsedTime, err := time.Parse("2006-01-02 15:04:05", created)
		if err != nil {
			continue
		}
		q.CreatedAt = parsedTime
		quotes = append(quotes, q)
	}
	return quotes, nil
}

// deleteQuote removes a quote with the given ID.
func deleteQuote(id int) error {
	res, err := db.Exec("DELETE FROM quotes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting quote: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("fetching affected rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("no quote with id %d found", id)
	}
	return nil
}

// printHelp returns the help message.
func printHelp() string {
	return `Usage:
!quote              - Return a random quote.
!quote add <quote>  - Add a new quote (author will be the sender).
!quote search <term> - Search for a quote.
!quote list         - List the first 5 quotes.
!quote delete <id>  - Delete a quote (moderator only).
!quote help        - Show this help message.`
}

// handleTwitchCommand processes Twitch chat messages that start with !quote.
func handleTwitchCommand(message, user, channel string, client *twitch.Client) {
	// Only handle messages starting with "!quote"
	if !strings.HasPrefix(message, "!quote") {
		return
	}
	parts := strings.Fields(message)
	if len(parts) < 1 {
		return
	}
	// If only "!quote" is sent, return a random quote if available,
	// but if there are no quotes, show the help message.
	if len(parts) == 1 {
		q, err := getRandomQuote()
		if err != nil {
			client.Say(channel, printHelp())
		} else {
			response := fmt.Sprintf("#%d: \"%s\" - %s", q.ID, q.Text, q.Author)
			client.Say(channel, response)
		}
		return
	}

	// Handle subcommands.
	subcmd := strings.ToLower(parts[1])
	switch subcmd {
	case "help":
		client.Say(channel, printHelp())
	case "add":
		// Command: !quote add <quote text>
		if len(parts) < 3 {
			client.Say(channel, "Usage: !quote add <quote text>")
			return
		}
		quoteText := strings.Join(parts[2:], " ")
		id, err := addQuote(quoteText, user)
		if err != nil {
			client.Say(channel, fmt.Sprintf("Error adding quote: %v", err))
		} else {
			client.Say(channel, fmt.Sprintf("Quote added with ID #%d.", id))
		}
	case "search":
		// Command: !quote search <term>
		if len(parts) < 3 {
			client.Say(channel, "Usage: !quote search <term>")
			return
		}
		term := strings.Join(parts[2:], " ")
		results, err := searchQuotes(term)
		if err != nil || len(results) == 0 {
			client.Say(channel, "No matching quotes found.")
			return
		}
		q := results[0]
		response := fmt.Sprintf("#%d: \"%s\" - %s", q.ID, q.Text, q.Author)
		client.Say(channel, response)
	case "list":
		// Command: !quote list – list the first few quotes.
		quotes, err := listQuotes()
		if err != nil || len(quotes) == 0 {
			client.Say(channel, "No quotes found.")
			return
		}
		var respParts []string
		for i, q := range quotes {
			if i >= 5 {
				break
			}
			respParts = append(respParts, fmt.Sprintf("#%d: \"%s\" - %s", q.ID, q.Text, q.Author))
		}
		client.Say(channel, strings.Join(respParts, " | "))
	case "delete":
		// Command: !quote delete <id>
		if len(parts) < 3 {
			client.Say(channel, "Usage: !quote delete <id>")
			return
		}
		id, err := strconv.Atoi(parts[2])
		if err != nil {
			client.Say(channel, "Invalid quote ID.")
			return
		}
		// NOTE: In a real bot, ensure the user has moderator permissions.
		err = deleteQuote(id)
		if err != nil {
			client.Say(channel, fmt.Sprintf("Error deleting quote #%d: %v", id, err))
		} else {
			client.Say(channel, fmt.Sprintf("Quote #%d deleted.", id))
		}
	default:
		// Fallback: if subcommand is unknown, show help.
		client.Say(channel, printHelp())
	}
}

// runCLI provides an interactive command-line shell for managing quotes.
func runCLI() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("Enter command (add, random, search, list, delete, help, exit):")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		switch strings.ToLower(input) {
		case "help":
			fmt.Println(printHelp())
		case "add":
			fmt.Println("Enter quote text:")
			quoteText, _ := reader.ReadString('\n')
			quoteText = strings.TrimSpace(quoteText)
			if quoteText == "" {
				fmt.Println("Quote text cannot be empty.")
				continue
			}
			fmt.Println("Enter author:")
			author, _ := reader.ReadString('\n')
			author = strings.TrimSpace(author)
			id, err := addQuote(quoteText, author)
			if err != nil {
				fmt.Println("Error adding quote:", err)
			} else {
				fmt.Printf("Quote added with ID #%d.\n", id)
			}
		case "random":
			q, err := getRandomQuote()
			if err != nil {
				fmt.Println("No quotes available.")
			} else {
				fmt.Printf("#%d: \"%s\" - %s\n", q.ID, q.Text, q.Author)
			}
		case "search":
			fmt.Println("Enter search term:")
			term, _ := reader.ReadString('\n')
			term = strings.TrimSpace(term)
			results, err := searchQuotes(term)
			if err != nil || len(results) == 0 {
				fmt.Println("No matching quotes found.")
			} else {
				for _, q := range results {
					fmt.Printf("#%d: \"%s\" - %s\n", q.ID, q.Text, q.Author)
				}
			}
		case "list":
			quotes, err := listQuotes()
			if err != nil || len(quotes) == 0 {
				fmt.Println("No quotes found.")
			} else {
				for _, q := range quotes {
					fmt.Printf("#%d: \"%s\" - %s\n", q.ID, q.Text, q.Author)
				}
			}
		case "delete":
			fmt.Println("Enter quote ID to delete:")
			idStr, _ := reader.ReadString('\n')
			idStr = strings.TrimSpace(idStr)
			id, err := strconv.Atoi(idStr)
			if err != nil {
				fmt.Println("Invalid ID")
				continue
			}
			err = deleteQuote(id)
			if err != nil {
				fmt.Printf("Error deleting quote #%d: %v\n", id, err)
			} else {
				fmt.Printf("Quote #%d deleted.\n", id)
			}
		case "exit":
			return
		default:
			fmt.Println("Unknown command. Type 'help' for instructions.")
		}
	}
}

func main() {
	// Command-line flags for configuration.
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

	// Initialize the SQLite database.
	if err := initDB(dbPath); err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	defer db.Close()

	// Support CLI mode for easy testing.
	if strings.ToLower(mode) == "cli" {
		runCLI()
	} else if strings.ToLower(mode) == "twitch" {
		// Check for required Twitch credentials.
		if twitchUser == "" || twitchOAuth == "" || twitchChannel == "" {
			log.Fatal("Twitch credentials (user, oauth, channel) must be provided in twitch mode.")
		}

		// Initialize Twitch IRC client.
		client := twitch.NewClient(twitchUser, twitchOAuth)
		client.OnPrivateMessage(func(message twitch.PrivateMessage) {
			// Handle each message in a separate goroutine.
			go handleTwitchCommand(message.Message, message.User.Name, message.Channel, client)
		})
		client.Join(twitchChannel)
		log.Printf("Connecting to Twitch channel #%s as %s...", twitchChannel, twitchUser)
		if err := client.Connect(); err != nil {
			log.Fatalf("Error connecting to Twitch: %v", err)
		}
	} else {
		log.Fatalf("Unknown mode: %s. Use 'twitch' or 'cli'.", mode)
	}
}
