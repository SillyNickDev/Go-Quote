package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// Quote holds the quote details.
type Quote struct {
	ID        int
	Text      string
	Author    string
	CreatedAt time.Time
}

// ErrNoQuotes is returned when the database does not contain any quotes that
// satisfy the requested operation.
var ErrNoQuotes = errors.New("no quotes available")

// QuoteStore provides database backed persistence for quotes.
type QuoteStore struct {
	db       *sql.DB
	random   *rand.Rand
	randomMu sync.Mutex
}

// Returns a non-nil error if the database cannot be opened, pinged, or initialized (table creation).
func NewQuoteStore(ctx context.Context, dbPath string) (*QuoteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(0)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging db: %w", err)
	}

	// Create table if it doesn't exist.
	const query = `CREATE TABLE IF NOT EXISTS quotes (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                text TEXT NOT NULL,
                author TEXT NOT NULL,
                created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );`
	if _, err := db.ExecContext(ctx, query); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating table: %w", err)
	}

	return &QuoteStore{
		db:     db,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// Close releases database resources.
func (s *QuoteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

type rowScanner interface {
	Scan(dest ...any) error
}

// parseSQLiteTime parses a timestamp string using several known layouts in the local time zone.
// Supported layouts are "2006-01-02 15:04:05", time.RFC3339Nano, and time.RFC3339; it returns the parsed time or an error if the format is unsupported.
func parseSQLiteTime(value string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp format: %q", value)
}

// scanQuote scans a rowScanner into a Quote and parses its created timestamp.
// It returns the populated Quote on success or an error if scanning the row or parsing the creation time fails.
func scanQuote(scanner rowScanner) (Quote, error) {
	var q Quote
	var created string
	if err := scanner.Scan(&q.ID, &q.Text, &q.Author, &created); err != nil {
		return Quote{}, err
	}
	parsedTime, err := parseSQLiteTime(created)
	if err != nil {
		return Quote{}, err
	}
	q.CreatedAt = parsedTime
	return q, nil
}

// Add inserts a new quote into the database.
func (s *QuoteStore) Add(ctx context.Context, text, author string) (int64, error) {
	if s == nil {
		return 0, errors.New("quote store is not initialized")
	}
	text = strings.TrimSpace(text)
	author = strings.TrimSpace(author)

	if text == "" {
		return 0, fmt.Errorf("quote text cannot be empty")
	}
	if author == "" {
		return 0, fmt.Errorf("author cannot be empty")
	}

	stmt, err := s.db.PrepareContext(ctx, "INSERT INTO quotes(text, author) VALUES(?, ?)")
	if err != nil {
		return 0, fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	res, err := stmt.ExecContext(ctx, text, author)
	if err != nil {
		return 0, fmt.Errorf("executing insert: %w", err)
	}
	return res.LastInsertId()
}

// Random returns a random quote.
func (s *QuoteStore) Random(ctx context.Context) (*Quote, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM quotes").Scan(&count); err != nil {
		return nil, fmt.Errorf("counting quotes: %w", err)
	}
	if count == 0 {
		return nil, ErrNoQuotes
	}

	s.randomMu.Lock()
	offset := s.random.Intn(count)
	s.randomMu.Unlock()
	row := s.db.QueryRowContext(ctx, "SELECT id, text, author, created_at FROM quotes ORDER BY id LIMIT 1 OFFSET ?", offset)
	q, err := scanQuote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoQuotes
		}
		return nil, fmt.Errorf("scanning random quote: %w", err)
	}
	return &q, nil
}

// Search returns quotes that match the given search term.
func (s *QuoteStore) Search(ctx context.Context, term string) ([]Quote, error) {
	if term == "" {
		return nil, ErrNoQuotes
	}
	likeTerm := "%" + term + "%"
	rows, err := s.db.QueryContext(ctx, "SELECT id, text, author, created_at FROM quotes WHERE text LIKE ? OR author LIKE ?", likeTerm, likeTerm)
	if err != nil {
		return nil, fmt.Errorf("querying quotes: %w", err)
	}
	defer rows.Close()

	var quotes []Quote
	for rows.Next() {
		q, err := scanQuote(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning quote: %w", err)
		}
		quotes = append(quotes, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating quotes: %w", err)
	}
	if len(quotes) == 0 {
		return nil, ErrNoQuotes
	}
	return quotes, nil
}

// List retrieves all quotes (ordered by ID).
func (s *QuoteStore) List(ctx context.Context) ([]Quote, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, text, author, created_at FROM quotes ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("listing quotes: %w", err)
	}
	defer rows.Close()

	var quotes []Quote
	for rows.Next() {
		q, err := scanQuote(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning quote: %w", err)
		}
		quotes = append(quotes, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating quotes: %w", err)
	}
	if len(quotes) == 0 {
		return nil, ErrNoQuotes
	}
	return quotes, nil
}

// Delete removes a quote with the given ID.
func (s *QuoteStore) Delete(ctx context.Context, id int) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM quotes WHERE id = ?", id)
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

// UpdateText replaces the text of a quote while leaving the author unchanged.
func (s *QuoteStore) UpdateText(ctx context.Context, id int, newText string) error {
	newText = strings.TrimSpace(newText)
	if newText == "" {
		return fmt.Errorf("quote text cannot be empty")
	}
	res, err := s.db.ExecContext(ctx, "UPDATE quotes SET text = ? WHERE id = ?", newText, id)
	if err != nil {
		return fmt.Errorf("updating quote text: %w", err)
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

// UpdateAuthor replaces the author of a quote while leaving the text unchanged.
func (s *QuoteStore) UpdateAuthor(ctx context.Context, id int, newAuthor string) error {
	newAuthor = strings.TrimSpace(newAuthor)
	if newAuthor == "" {
		return fmt.Errorf("author cannot be empty")
	}
	res, err := s.db.ExecContext(ctx, "UPDATE quotes SET author = ? WHERE id = ?", newAuthor, id)
	if err != nil {
		return fmt.Errorf("updating quote author: %w", err)
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

// GetByID retrieves a quote using its ID.
func (s *QuoteStore) GetByID(ctx context.Context, id int) (*Quote, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, text, author, created_at FROM quotes WHERE id = ?", id)
	q, err := scanQuote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoQuotes
		}
		return nil, fmt.Errorf("fetching quote by id: %w", err)
	}
	return &q, nil
}

// Latest returns the most recently added quote.
func (s *QuoteStore) Latest(ctx context.Context) (*Quote, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, text, author, created_at FROM quotes ORDER BY id DESC LIMIT 1")
	q, err := scanQuote(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoQuotes
		}
		return nil, fmt.Errorf("fetching latest quote: %w", err)
	}
	return &q, nil
}

// Count returns the total number of quotes stored.
func (s *QuoteStore) Count(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM quotes")
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("counting quotes: %w", err)
	}
	return total, nil
}
