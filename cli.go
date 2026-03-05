package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// runCLI handles all CLI commands
func runCLI(args []string, cfg *Config, store *Store, tmux *TmuxManager) {
	if len(args) == 0 {
		fmt.Println("Usage: soap <command> [args...]")
		fmt.Println("\nCommands:")
		fmt.Println("  create <title>         Create a new ticket")
		fmt.Println("  add                    Import tickets from external system")
		fmt.Println("  list [--json]          List all tickets")
		fmt.Println("  select <id>            Select ticket (create worktree, open Claude)")
		fmt.Println("  duplicate <id>         Duplicate current worktree for ticket")
		fmt.Println("  delete <id>            Delete ticket and cleanup worktrees")
		fmt.Println("  status <id>            Show ticket details")
		fmt.Println("  whoami                 Detect ticket from current worktree")
		fmt.Println("  ping                   Signal activity (used by Claude hooks)")
		fmt.Println("  idle                   Signal idle state (used by Claude hooks)")
		fmt.Println("  subscribe [subject]    Subscribe to NATS messages")
		fmt.Println("  install-hooks          Install Claude Code hooks")
		os.Exit(1)
	}

	cmd := args[0]

	switch cmd {
	case "create":
		handleCreate(args[1:], store)

	case "add":
		handleAdd(cfg, store)

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

	case "ping":
		handlePing(store)

	case "idle":
		handleIdle(store)

	case "subscribe":
		handleSubscribe(args[1:], store)

	case "install-hooks":
		global := len(args) > 1 && args[1] == "--global"
		installHooks(global)

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

func handleAdd(cfg *Config, store *Store) {
	if cfg.ListTickets == "" {
		fmt.Fprintf(os.Stderr, "Error: listTickets not configured in soap.yaml\n")
		os.Exit(1)
	}

	// Fetch external tickets
	cmd := exec.Command("bash", "-c", strings.TrimSpace(cfg.ListTickets))
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching tickets: %v\n", err)
		os.Exit(1)
	}

	var tickets []ExternalTicket
	if err := json.Unmarshal(out, &tickets); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	if len(tickets) == 0 {
		fmt.Println("No tickets available")
		return
	}

	// Display tickets for selection
	fmt.Println("Available tickets:")
	for i, t := range tickets {
		fmt.Printf("%d. #%v: %s\n", i+1, t.ID, t.Title)
	}

	fmt.Print("\nSelect ticket number (or 'q' to quit): ")
	var input string
	fmt.Scanln(&input)

	if input == "q" || input == "" {
		fmt.Println("Cancelled")
		return
	}

	var selection int
	if _, err := fmt.Sscanf(input, "%d", &selection); err != nil || selection < 1 || selection > len(tickets) {
		fmt.Fprintf(os.Stderr, "Invalid selection\n")
		os.Exit(1)
	}

	ext := tickets[selection-1]

	// Convert external ticket ID to string
	var ticketID string
	switch v := ext.ID.(type) {
	case string:
		ticketID = v
	case float64:
		ticketID = fmt.Sprintf("%.0f", v)
	case int:
		ticketID = fmt.Sprintf("%d", v)
	default:
		ticketID = fmt.Sprintf("%v", v)
	}

	ticket := Ticket{
		ID:    ticketID,
		Title: ext.Title,
	}

	if err := store.PutTicket(ticket); err != nil {
		fmt.Fprintf(os.Stderr, "Error adding ticket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added %s: %s\n", ticket.ID, ticket.Title)
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

func handlePing(store *Store) {
	// Ping the soap server - used by Claude hooks to signal activity.
	// Detects current ticket and clears its NeedsAttention flag.
	logFile, _ := os.OpenFile("/tmp/soap-ping.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if logFile != nil {
		defer logFile.Close()
		fmt.Fprintf(logFile, "[%s] ping hook called\n", time.Now().Format(time.RFC3339))
	}

	// Check env var first (set by tmux spawn script)
	ticketID := os.Getenv("SOAP_TICKET_ID")

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] SOAP_TICKET_ID=%s\n", time.Now().Format(time.RFC3339), ticketID)
	}

	if ticketID == "" {
		// Fallback to cwd matching for manual launches
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] cwd=%s\n", time.Now().Format(time.RFC3339), cwd)
		}

		// Find ticket ID from worktree
		tickets, err := store.AllTickets()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching tickets: %v\n", err)
			os.Exit(1)
		}

		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] found %d tickets\n", time.Now().Format(time.RFC3339), len(tickets))
			for _, t := range tickets {
				fmt.Fprintf(logFile, "[%s]   - %s: worktree=%s\n", time.Now().Format(time.RFC3339), t.ID, t.Worktree)
			}
		}

		for _, t := range tickets {
			// Try exact path prefix match first
			if t.Worktree != "" && (strings.HasPrefix(cwd, t.Worktree) || strings.HasPrefix(t.Worktree, cwd)) {
				ticketID = t.ID
				if logFile != nil {
					fmt.Fprintf(logFile, "[%s] matched via worktree path\n", time.Now().Format(time.RFC3339))
				}
				break
			}
			// Fallback: match by ticket ID in path (handles worktrees in different repos)
			if strings.Contains(cwd, "/.worktrees/"+t.ID) || strings.Contains(cwd, "\\.worktrees\\"+t.ID) {
				ticketID = t.ID
				if logFile != nil {
					fmt.Fprintf(logFile, "[%s] matched via ticket ID in path\n", time.Now().Format(time.RFC3339))
				}
				break
			}
		}
	}

	if ticketID == "" {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] no matching ticket found\n", time.Now().Format(time.RFC3339))
		}
		// Not in a worktree, just print pong
		fmt.Println("pong")
		return
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] matched ticket %s\n", time.Now().Format(time.RFC3339), ticketID)
	}

	// Get fresh ticket before updating
	ticket, err := store.GetTicket(ticketID)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] error getting ticket: %v\n", time.Now().Format(time.RFC3339), err)
		}
		fmt.Fprintf(os.Stderr, "Error getting ticket: %v\n", err)
		os.Exit(1)
	}

	// Clear NeedsAttention and update LastPingTime
	ticket.NeedsAttention = false
	ticket.LastPingTime = time.Now().Unix()
	if err := store.PutTicket(*ticket); err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] error saving ticket: %v\n", time.Now().Format(time.RFC3339), err)
		}
		fmt.Fprintf(os.Stderr, "Error updating ticket: %v\n", err)
		os.Exit(1)
	}

	// Publish ping event to NATS
	if err := store.Publish("soap.ping", []byte(ticketID)); err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] error publishing ping event: %v\n", time.Now().Format(time.RFC3339), err)
		}
	} else if logFile != nil {
		fmt.Fprintf(logFile, "[%s] published soap.ping event for %s\n", time.Now().Format(time.RFC3339), ticketID)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] cleared NeedsAttention for %s\n", time.Now().Format(time.RFC3339), ticketID)
	}
	fmt.Println("pong")
}

func handleIdle(store *Store) {
	// Called when Claude goes idle - sets NeedsAttention flag.
	// Check env var first (set by tmux spawn script)
	ticketID := os.Getenv("SOAP_TICKET_ID")

	if ticketID == "" {
		// Fallback to cwd matching for manual launches
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Find ticket ID from worktree
		tickets, err := store.AllTickets()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching tickets: %v\n", err)
			os.Exit(1)
		}

		for _, t := range tickets {
			// Try exact path prefix match first
			if t.Worktree != "" && (strings.HasPrefix(cwd, t.Worktree) || strings.HasPrefix(t.Worktree, cwd)) {
				ticketID = t.ID
				break
			}
			// Fallback: match by ticket ID in path (handles worktrees in different repos)
			if strings.Contains(cwd, "/.worktrees/"+t.ID) || strings.Contains(cwd, "\\.worktrees\\"+t.ID) {
				ticketID = t.ID
				break
			}
		}
	}

	if ticketID == "" {
		return
	}

	// Get fresh ticket before updating
	ticket, err := store.GetTicket(ticketID)
	if err != nil {
		return
	}

	// Set NeedsAttention flag - Claude is idle and waiting
	ticket.NeedsAttention = true
	if err := store.PutTicket(*ticket); err != nil {
		return
	}

	// Publish idle event to NATS
	store.Publish("soap.idle", []byte(ticketID))
}

func handleSubscribe(args []string, store *Store) {
	// Subscribe to NATS subject and print messages
	subject := "soap.>"
	if len(args) > 0 {
		subject = args[0]
	}

	fmt.Printf("Subscribing to '%s' on nats://127.0.0.1:%d\n", subject, natsPort)
	fmt.Println("Press Ctrl+C to stop...")

	sub, err := store.nc.Subscribe(subject, func(msg *nats.Msg) {
		fmt.Printf("[%s] Subject: %s\n", time.Now().Format("15:04:05"), msg.Subject)
		if len(msg.Data) > 0 {
			fmt.Printf("  Data: %s\n", string(msg.Data))
		}
		if len(msg.Header) > 0 {
			fmt.Printf("  Headers: %v\n", msg.Header)
		}
		fmt.Println()
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error subscribing: %v\n", err)
		os.Exit(1)
	}
	defer sub.Unsubscribe()

	// Ensure subscription is active
	if err := store.nc.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "Error flushing: %v\n", err)
	}
	fmt.Println("Subscription active, waiting for messages...")

	// Block forever
	select {}
}

//go:embed hooks-template.json
var hooksTemplate string

func installHooks(global bool) {
	if global {
		installGlobalHooks()
		return
	}

	// Install Claude Code hooks for the current project (non-destructive)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := installHooksInDir(cwd); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Installed soap hooks in .claude/settings.local.json")
}

// installHooksInDir installs hooks in a specific directory's .claude/settings.local.json
func installHooksInDir(dir string) error {
	// Find or create .claude directory
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}

	settingsFile := filepath.Join(claudeDir, "settings.local.json")

	// Get soap binary path and replace in template
	soapPath, err := os.Executable()
	if err != nil {
		soapPath = "~/.soap/soap"
	}
	template := strings.ReplaceAll(hooksTemplate, "~/.soap/soap", soapPath)

	// Use jq to merge template with existing settings
	cmd := exec.Command("jq", "-s", ".[0] * .[1]", settingsFile, "-")
	cmd.Stdin = strings.NewReader(template)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If file doesn't exist, jq fails reading it, so just use template
		if _, statErr := os.Stat(settingsFile); os.IsNotExist(statErr) {
			if err := os.WriteFile(settingsFile, []byte(template), 0644); err != nil {
				return fmt.Errorf("writing settings.json: %w", err)
			}
			return nil
		}
		return fmt.Errorf("running jq: %w: %s", err, output)
	}

	if err := os.WriteFile(settingsFile, output, 0644); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}

	return nil
}

func installGlobalHooks() {
	// Install hooks in global Claude Code settings (non-destructive)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	settingsFile := filepath.Join(homeDir, ".claude.json")

	// Get soap binary path and replace in template
	soapPath, err := os.Executable()
	if err != nil {
		soapPath = "soap"
	}
	template := strings.ReplaceAll(hooksTemplate, "~/.soap/soap", soapPath)

	// Use jq to merge template with existing settings
	cmd := exec.Command("jq", "-s", ".[0] * .[1]", settingsFile, "-")
	cmd.Stdin = strings.NewReader(template)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If file doesn't exist, jq fails reading it, so just use template
		if _, statErr := os.Stat(settingsFile); os.IsNotExist(statErr) {
			if err := os.WriteFile(settingsFile, []byte(template), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", settingsFile, err)
				os.Exit(1)
			}
			fmt.Println("✓ Installed soap hooks globally")
			return
		}
		fmt.Fprintf(os.Stderr, "Error running jq: %v: %s\n", err, output)
		os.Exit(1)
	}

	if err := os.WriteFile(settingsFile, output, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", settingsFile, err)
		os.Exit(1)
	}

	fmt.Printf("✓ Installed soap hooks globally in %s\n", settingsFile)
}
