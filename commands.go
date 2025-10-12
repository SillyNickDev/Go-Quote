package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CommandHandler turns incoming messages into responses using a QuoteStore.
type CommandHandler struct {
	store *QuoteStore
}

// NewCommandHandler returns a new CommandHandler that uses the provided QuoteStore.
// Pass a non-nil store to enable quote operations; a nil store will leave the handler misconfigured.
func NewCommandHandler(store *QuoteStore) *CommandHandler {
	return &CommandHandler{store: store}
}

func (h *CommandHandler) Handle(ctx context.Context, message, user string) []string {
	if h == nil || h.store == nil {
		return []string{"Quote handler is not configured"}
	}

	if !strings.HasPrefix(message, "!quote") {
		return nil
	}

	parts := strings.Fields(message)
	if len(parts) == 0 {
		return nil
	}

	if len(parts) == 1 {
		quote, err := h.store.Random(ctx)
		if err != nil {
			if errors.Is(err, ErrNoQuotes) {
				return []string{"No quotes have been added yet. Try !quote add to add one!"}
			}
			return []string{fmt.Sprintf("Error fetching quote: %v", err)}
		}
		return []string{formatQuote(*quote)}
	}

	subcmd := strings.ToLower(parts[1])
	switch subcmd {
	case "help":
		return []string{printHelp()}
	case "add":
		if len(parts) < 3 {
			return []string{"Usage: !quote add <quote text>"}
		}
		quoteText := strings.Join(parts[2:], " ")
		author := user
		if pieces := strings.SplitN(quoteText, "|", 2); len(pieces) == 2 {
			if customAuthor := strings.TrimSpace(pieces[0]); customAuthor != "" {
				author = customAuthor
			}
			quoteText = strings.TrimSpace(pieces[1])
		}
		id, err := h.store.Add(ctx, strings.TrimSpace(quoteText), author)
		if err != nil {
			return []string{fmt.Sprintf("Error adding quote: %v", err)}
		}
		return []string{fmt.Sprintf("Quote added with ID #%d.", id)}
	case "search":
		if len(parts) < 3 {
			return []string{"Usage: !quote search <term>"}
		}
		term := strings.Join(parts[2:], " ")
		results, err := h.store.Search(ctx, term)
		if err != nil {
			if errors.Is(err, ErrNoQuotes) {
				return []string{"No matching quotes found."}
			}
			return []string{fmt.Sprintf("Error searching quotes: %v", err)}
		}
		return []string{formatQuote(results[0])}
	case "get":
		if len(parts) < 3 {
			return []string{"Usage: !quote get <id>"}
		}
		id, err := strconv.Atoi(parts[2])
		if err != nil {
			return []string{"Invalid quote ID."}
		}
		quote, err := h.store.GetByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNoQuotes) {
				return []string{fmt.Sprintf("No quote with ID #%d found.", id)}
			}
			return []string{fmt.Sprintf("Error fetching quote #%d: %v", id, err)}
		}
		return []string{formatQuote(*quote)}
	case "list":
		quotes, err := h.store.List(ctx)
		if err != nil {
			if errors.Is(err, ErrNoQuotes) {
				return []string{"No quotes found."}
			}
			return []string{fmt.Sprintf("Error listing quotes: %v", err)}
		}
		var respParts []string
		for i, q := range quotes {
			if i >= 5 {
				break
			}
			respParts = append(respParts, formatQuote(q))
		}
		return []string{strings.Join(respParts, " | ")}
	case "latest":
		quote, err := h.store.Latest(ctx)
		if err != nil {
			if errors.Is(err, ErrNoQuotes) {
				return []string{"No quotes have been added yet."}
			}
			return []string{fmt.Sprintf("Error fetching latest quote: %v", err)}
		}
		response := fmt.Sprintf("Latest is #%d: \"%s\" - %s (added %s)", quote.ID, quote.Text, quote.Author, quote.CreatedAt.Format(time.RFC822))
		return []string{response}
	case "count":
		total, err := h.store.Count(ctx)
		if err != nil {
			return []string{fmt.Sprintf("Error counting quotes: %v", err)}
		}
		if total == 0 {
			return []string{"No quotes have been added yet."}
		}
		return []string{fmt.Sprintf("There %s %d quote%s saved.", pluralize("is", "are", total), total, pluralSuffix(total))}
	case "delete":
		if len(parts) < 3 {
			return []string{"Usage: !quote delete <id>"}
		}
		id, err := strconv.Atoi(parts[2])
		if err != nil {
			return []string{"Invalid quote ID."}
		}
		if err := h.store.Delete(ctx, id); err != nil {
			return []string{fmt.Sprintf("Error deleting quote #%d: %v", id, err)}
		}
		return []string{fmt.Sprintf("Quote #%d deleted.", id)}
	default:
		return []string{printHelp()}
	}
}

// formatQuote formats a Quote as a single-line string in the form `#<ID>: "<Text>" - <Author>`.
func formatQuote(q Quote) string {
	return fmt.Sprintf("#%d: \"%s\" - %s", q.ID, q.Text, q.Author)
}

// printHelp returns the usage help text for the !quote command and its subcommands.
func printHelp() string {
	return `Usage:
!quote              - Return a random quote.
!quote add <quote>  - Add a new quote (author will be the sender).
!quote add <author> | <quote> - Add a quote for another author.
!quote search <term> - Search for a quote.
!quote get <id>     - Get a specific quote by ID.
!quote list         - List the first 5 quotes.
!quote latest       - Show the most recently added quote.
!quote count        - Show how many quotes are stored.
!quote delete <id>  - Delete a quote (moderator only).
!quote help        - Show this help message.`
}

// pluralize returns the singular form when count is 1 and the plural form otherwise.
func pluralize(single, plural string, count int) string {
	if count == 1 {
		return single
	}
	return plural
}

// pluralSuffix returns the plural suffix for a count: an empty string when count is 1,
// otherwise "s".
func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}