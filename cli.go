package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// runCLI starts an interactive command-line loop that accepts user commands to manage quotes
// using the provided QuoteStore and delegates unrecognized commands to the provided CommandHandler.
// It prompts on stdin for commands (add, random, search, get, latest, count, list, delete, help, exit),
// performs the corresponding store operations, prints results to stdout, and returns when the user
// issues "exit" or when an input error occurs.
func runCLI(ctx context.Context, store *QuoteStore, handler *CommandHandler) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("Enter command (add, random, search, get, latest, count, list, delete, help, exit):")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
			return
		}
		input = strings.TrimSpace(input)

		switch strings.ToLower(input) {
		case "help":
			fmt.Println(printHelp())
		case "add":
			fmt.Println("Enter quote text:")
			quoteText, _ := reader.ReadString('\n')
			quoteText = strings.TrimSpace(quoteText)
			fmt.Println("Enter author (leave blank to use default):")
			author, _ := reader.ReadString('\n')
			author = strings.TrimSpace(author)
			if author == "" {
				author = "CLI"
			}
			id, err := store.Add(ctx, quoteText, author)
			if err != nil {
				fmt.Println("Error adding quote:", err)
				continue
			}
			fmt.Printf("Quote added with ID #%d.\n", id)
		case "random":
			q, err := store.Random(ctx)
			if err != nil {
				if errors.Is(err, ErrNoQuotes) {
					fmt.Println("No quotes have been added yet.")
				} else {
					fmt.Println("Error fetching quote:", err)
				}
				continue
			}
			fmt.Println(formatQuote(*q))
		case "search":
			fmt.Println("Enter search term:")
			term, _ := reader.ReadString('\n')
			term = strings.TrimSpace(term)
			results, err := store.Search(ctx, term)
			if err != nil {
				if errors.Is(err, ErrNoQuotes) {
					fmt.Println("No matching quotes found.")
				} else {
					fmt.Println("Error searching quotes:", err)
				}
				continue
			}
			for _, q := range results {
				fmt.Println(formatQuote(q))
			}
		case "get":
			fmt.Println("Enter quote ID:")
			idStr, _ := reader.ReadString('\n')
			idStr = strings.TrimSpace(idStr)
			id, err := strconv.Atoi(idStr)
			if err != nil {
				fmt.Println("Invalid ID")
				continue
			}
			q, err := store.GetByID(ctx, id)
			if err != nil {
				if errors.Is(err, ErrNoQuotes) {
					fmt.Printf("No quote with ID #%d found.\n", id)
				} else {
					fmt.Printf("Error fetching quote #%d: %v\n", id, err)
				}
			} else {
				fmt.Println(formatQuote(*q))
			}
		case "latest":
			q, err := store.Latest(ctx)
			if err != nil {
				if errors.Is(err, ErrNoQuotes) {
					fmt.Println("No quotes have been added yet.")
				} else {
					fmt.Println("Error fetching latest quote:", err)
				}
			} else {
				fmt.Printf("Latest is #%d: \"%s\" - %s\n", q.ID, q.Text, q.Author)
			}
		case "count":
			total, err := store.Count(ctx)
			if err != nil {
				fmt.Println("Error counting quotes:", err)
				continue
			}
			fmt.Printf("There %s %d quote%s saved.\n", pluralize("is", "are", total), total, pluralSuffix(total))
		case "list":
			quotes, err := store.List(ctx)
			if err != nil {
				if errors.Is(err, ErrNoQuotes) {
					fmt.Println("No quotes found.")
				} else {
					fmt.Println("Error listing quotes:", err)
				}
			} else {
				for _, q := range quotes {
					fmt.Println(formatQuote(q))
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
			if err := store.Delete(ctx, id); err != nil {
				fmt.Printf("Error deleting quote #%d: %v\n", id, err)
			} else {
				fmt.Printf("Quote #%d deleted.\n", id)
			}
		case "exit":
			return
		default:
			// Fallback to the shared handler for misc commands (e.g. !quote)
			responses := handler.Handle(ctx, input, "CLI", true)
			for _, resp := range responses {
				fmt.Println(resp)
			}
		}
	}
}
