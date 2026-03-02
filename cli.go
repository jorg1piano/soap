package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runCLI handles all CLI commands
func runCLI(args []string, cfg *Config, store *Store, tmux *TmuxManager) {
	if len(args) == 0 {
		fmt.Println("Usage: soap <command> [args...]")
		fmt.Println("\nCommands:")
		fmt.Println("  create <title>         Create a new ticket")
		fmt.Println("  list [--json]          List all tickets")
		fmt.Println("  select <id>            Select ticket (create worktree, open Claude)")
		fmt.Println("  duplicate <id>         Duplicate current worktree for ticket")
		fmt.Println("  delete <id>            Delete ticket and cleanup worktrees")
		fmt.Println("  status <id>            Show ticket details")
		fmt.Println("  whoami                 Detect ticket from current worktree")
		os.Exit(1)
	}

	cmd := args[0]

	switch cmd {
	case "create":
		handleCreate(args[1:], store)

	case "list":
		handleList(args[1:], store)

	case "select":
		handleSelect(args[1:], cfg, store, tmux)

	case "duplicate":
		handleDuplicate(args[1:], cfg, store, tmux)

	case "delete":
		handleDelete(args[1:], cfg, store, tmux)

	case "status":
		handleStatus(args[1:], store)

	case "whoami":
		handleWhoami(store)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func handleCreate(args []string, store *Store) {
	if len(args) < 1 {
		fmt.Println("Usage: soap create <title>")
		os.Exit(1)
	}

	title := strings.Join(args, " ")
	ticket := Ticket{
		ID:    generateTicketID(),
		Title: title,
	}

	if err := store.PutTicket(ticket); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating ticket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created %s: %s\n", ticket.ID, ticket.Title)
}

func handleList(args []string, store *Store) {
	tickets, err := store.AllTickets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	jsonOutput := len(args) > 0 && args[0] == "--json"

	if jsonOutput {
		data, err := json.MarshalIndent(tickets, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	if len(tickets) == 0 {
		fmt.Println("No tickets")
		return
	}

	for _, t := range tickets {
		line := fmt.Sprintf("%-10s %s", t.ID, t.Title)
		if t.Worktree != "" {
			line += fmt.Sprintf("  [worktree: %s]", t.Worktree)
		}
		fmt.Println(line)
	}
}

func handleSelect(args []string, cfg *Config, store *Store, tmux *TmuxManager) {
	if len(args) < 1 {
		fmt.Println("Usage: soap select <id>")
		os.Exit(1)
	}

	ticketID := args[0]
	ticket, err := store.GetTicket(ticketID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: ticket %s not found: %v\n", ticketID, err)
		os.Exit(1)
	}

	var worktreePath string

	// Check if ticket already has a worktree
	if ticket.Worktree != "" {
		worktreePath = ticket.Worktree
		fmt.Printf("Using existing worktree: %s\n", worktreePath)
	} else {
		// Create new worktree using configured command
		if cfg.CreateWorktree == "" {
			fmt.Fprintf(os.Stderr, "Error: createWorktree not configured in soap.yaml\n")
			os.Exit(1)
		}

		cmdStr, err := RenderTemplate(cfg.CreateWorktree, TemplateData{
			ID:    ticket.ID,
			Title: ticket.Title,
			Index: 0,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering template: %v\n", err)
			os.Exit(1)
		}

		cmd := exec.Command("bash", "-c", cmdStr)
		out, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating worktree: %v\n", err)
			os.Exit(1)
		}

		worktreePath = strings.TrimSpace(string(out))
		if worktreePath == "" {
			fmt.Fprintf(os.Stderr, "Error: worktree command produced no output\n")
			os.Exit(1)
		}

		if !filepath.IsAbs(worktreePath) {
			cwd, _ := os.Getwd()
			worktreePath = filepath.Join(cwd, worktreePath)
		}

		// Update ticket with worktree path
		ticket.Worktree = worktreePath
		if err := store.PutTicket(*ticket); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating ticket: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Created worktree: %s\n", worktreePath)

		// Run setup command if configured
		if cfg.Setup != "" {
			setupCmd, err := RenderTemplate(cfg.Setup, TemplateData{
				ID:       ticket.ID,
				Title:    ticket.Title,
				Worktree: worktreePath,
				Index:    0,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error rendering setup template: %v\n", err)
				os.Exit(1)
			}

			cmd := exec.Command("bash", "-c", setupCmd)
			cmd.Dir = worktreePath
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: setup command failed: %v\n", err)
			}
		}
	}

	// Open Claude session in tmux
	if err := tmux.OpenClaudeSession(*ticket, worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening Claude session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Opened Claude session for %s\n", ticket.ID)
}

func handleDuplicate(args []string, cfg *Config, store *Store, tmux *TmuxManager) {
	if len(args) < 1 {
		fmt.Println("Usage: soap duplicate <id>")
		os.Exit(1)
	}

	if cfg.DuplicateWorktree == "" {
		fmt.Fprintf(os.Stderr, "Error: duplicateWorktree not configured in soap.yaml\n")
		os.Exit(1)
	}

	ticketID := args[0]
	ticket, err := store.GetTicket(ticketID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: ticket %s not found: %v\n", ticketID, err)
		os.Exit(1)
	}

	// Count existing worktrees to determine index
	// In a real implementation, you'd track multiple worktrees per ticket
	index := 1

	cmdStr, err := RenderTemplate(cfg.DuplicateWorktree, TemplateData{
		ID:       ticket.ID,
		Title:    ticket.Title,
		Worktree: ticket.Worktree,
		Index:    index,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering template: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command("bash", "-c", cmdStr)
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error duplicating worktree: %v\n", err)
		os.Exit(1)
	}

	newWorktreePath := strings.TrimSpace(string(out))
	fmt.Printf("Duplicated worktree: %s\n", newWorktreePath)

	// Open new Claude session in the duplicated worktree
	if err := tmux.OpenClaudeSession(*ticket, newWorktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening Claude session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Opened Claude session for %s in duplicated worktree\n", ticket.ID)
}

func handleDelete(args []string, cfg *Config, store *Store, tmux *TmuxManager) {
	if len(args) < 1 {
		fmt.Println("Usage: soap delete <id>")
		os.Exit(1)
	}

	ticketID := args[0]
	ticket, err := store.GetTicket(ticketID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: ticket %s not found: %v\n", ticketID, err)
		os.Exit(1)
	}

	// Cleanup tmux window
	tmux.CleanupTicket(ticketID)

	// Cleanup worktree
	if ticket.Worktree != "" {
		cmd := exec.Command("git", "worktree", "remove", "--force", ticket.Worktree)
		if err := cmd.Run(); err != nil {
			// Fallback to rm -rf
			os.RemoveAll(ticket.Worktree)
		}
		fmt.Printf("Removed worktree: %s\n", ticket.Worktree)
	}

	// Delete ticket
	if err := store.DeleteTicket(ticketID); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting ticket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleted ticket %s\n", ticketID)
}

func handleStatus(args []string, store *Store) {
	if len(args) < 1 {
		fmt.Println("Usage: soap status <id>")
		os.Exit(1)
	}

	ticketID := args[0]
	ticket, err := store.GetTicket(ticketID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: ticket %s not found: %v\n", ticketID, err)
		os.Exit(1)
	}

	fmt.Printf("ID:    %s\n", ticket.ID)
	fmt.Printf("Title: %s\n", ticket.Title)
	if ticket.Worktree != "" {
		fmt.Printf("Worktree: %s\n", ticket.Worktree)
	}
}

func handleWhoami(store *Store) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	tickets, _ := store.AllTickets()
	for _, t := range tickets {
		if t.Worktree != "" && strings.HasPrefix(cwd, t.Worktree) {
			fmt.Println(t.ID)
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Not in a ticket worktree\n")
	os.Exit(1)
}
