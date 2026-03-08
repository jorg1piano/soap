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

func printUsage() {
	fmt.Println("Usage: soap <command> [args...]")
	fmt.Println("\nCommands:")
	fmt.Println("  list [--json]          List tickets from external system")
	fmt.Println("  select <id>            Run onSelect hook for ticket")
	fmt.Println("  delete <id>            Run onDelete hook for ticket")
	fmt.Println("  tick                   Re-evaluate all tmux pane state")
	fmt.Println("  register               Register terminal with TUI (used by Claude hooks)")
	fmt.Println("  unregister             Unregister terminal from TUI (used by Claude hooks)")
	fmt.Println("  subscribe [subject]    Subscribe to NATS messages")
	fmt.Println("  add-key <key>          Add a key to current pane (e.g. claude)")
	fmt.Println("  remove-key <key>       Remove a key from current pane")
	fmt.Println("  install-hooks          Install Claude Code hooks")
	fmt.Println("  install-tmux-hooks     Install tmux hooks to ~/.tmux.conf")
}

// runCLI handles all CLI commands
func runCLI(args []string, cfg *Config, store *Store, tmux *TmuxManager) {
	if len(args) == 0 {
		printUsage()
		return
	}

	cmd := args[0]

	switch cmd {
	case "list":
		handleList(args[1:], cfg)

	case "select":
		handleSelect(args[1:], cfg)

	case "delete":
		handleDelete(args[1:], cfg)

	case "register":
		handleRegister(args[1:], store)

	case "unregister":
		handleUnregister(args[1:], store)

	case "tick":
		handleTick(store)

	case "subscribe":
		handleSubscribe(args[1:], store)

	case "install-hooks":
		global := len(args) > 1 && args[1] == "--global"
		installHooks(global)

	case "install-tmux-hooks":
		installTmuxHooks()

	case "add-key":
		handleAddKey(args[1:], store)

	case "remove-key":
		handleRemoveKey(args[1:], store)

	case "help", "--help", "-h":
		runCLI(nil, cfg, store, tmux)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

// fetchTickets runs the listTickets command and returns parsed tickets
func fetchTickets(cfg *Config) ([]ExternalTicket, error) {
	if cfg.ListTickets == "" {
		return nil, fmt.Errorf("listTickets not configured in soap.yaml")
	}

	cmd := exec.Command("bash", "-c", strings.TrimSpace(cfg.ListTickets))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fetching tickets: %w", err)
	}

	var tickets []ExternalTicket
	if err := json.Unmarshal(out, &tickets); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	// Filter to ticket kind
	var filtered []ExternalTicket
	for _, t := range tickets {
		if t.Kind == "" || t.Kind == "ticket" {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

func handleList(args []string, cfg *Config) {
	tickets, err := fetchTickets(cfg)
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
		fmt.Printf("%-10s %s\n", t.TicketID(), t.Title)
	}
}

func handleSelect(args []string, cfg *Config) {
	if len(args) < 1 {
		fmt.Println("Usage: soap select <id>")
		os.Exit(1)
	}

	if cfg.OnSelect == "" {
		fmt.Fprintf(os.Stderr, "Error: onSelect not configured in soap.yaml\n")
		os.Exit(1)
	}

	ticketID := args[0]

	// Find ticket title from list (best effort)
	title := ""
	if tickets, err := fetchTickets(cfg); err == nil {
		for _, t := range tickets {
			if t.TicketID() == ticketID {
				title = t.Title
				break
			}
		}
	}

	cmdStr, err := RenderTemplate(cfg.OnSelect, TemplateData{
		ID:    ticketID,
		Title: title,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering template: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command("bash", "-c", strings.TrimSpace(cmdStr))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running onSelect: %v\n", err)
		os.Exit(1)
	}
}

func handleDelete(args []string, cfg *Config) {
	if len(args) < 1 {
		fmt.Println("Usage: soap delete <id>")
		os.Exit(1)
	}

	if cfg.OnDelete == "" {
		fmt.Fprintf(os.Stderr, "Error: onDelete not configured in soap.yaml\n")
		os.Exit(1)
	}

	ticketID := args[0]

	cmdStr, err := RenderTemplate(cfg.OnDelete, TemplateData{
		ID: ticketID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering template: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command("bash", "-c", strings.TrimSpace(cmdStr))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running onDelete: %v\n", err)
		os.Exit(1)
	}
}

func handleRegister(args []string, store *Store) {
	logFile, _ := os.OpenFile("/tmp/soap-register.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if logFile != nil {
		defer logFile.Close()
		fmt.Fprintf(logFile, "[%s] register called, pid=%d, args=%v\n", time.Now().Format(time.RFC3339), os.Getpid(), args)
	}

	var paneID string
	if len(args) > 0 {
		paneID = args[0]
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] using pane ID from argument: %s\n", time.Now().Format(time.RFC3339), paneID)
		}
	} else {
		paneID = os.Getenv("TMUX_PANE")
		if paneID == "" {
			cmd := exec.Command("tmux", "display-message", "-p", "#{pane_id}")
			if out, err := cmd.Output(); err == nil {
				paneID = strings.TrimSpace(string(out))
			} else if logFile != nil {
				fmt.Fprintf(logFile, "[%s] failed to detect pane: %v\n", time.Now().Format(time.RFC3339), err)
			}
		}
	}

	if paneID == "" {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] no pane ID, exiting\n", time.Now().Format(time.RFC3339))
		}
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] failed to get cwd: %v\n", time.Now().Format(time.RFC3339), err)
		}
		return
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] registering pane=%s path=%s\n", time.Now().Format(time.RFC3339), paneID, cwd)
	}

	metadata := map[string]string{
		"pane_id":      paneID,
		"current_path": cwd,
		"pid":          fmt.Sprintf("%d", os.Getpid()),
	}
	data, _ := json.Marshal(metadata)
	if err := store.Publish("soap.register", data); err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] failed to publish: %v\n", time.Now().Format(time.RFC3339), err)
		}
	} else if logFile != nil {
		fmt.Fprintf(logFile, "[%s] successfully published registration\n", time.Now().Format(time.RFC3339))
	}
}

func handleUnregister(args []string, store *Store) {
	logFile, _ := os.OpenFile("/tmp/soap-register.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if logFile != nil {
		defer logFile.Close()
		fmt.Fprintf(logFile, "[%s] unregister called, pid=%d, args=%v\n", time.Now().Format(time.RFC3339), os.Getpid(), args)
	}

	var paneID string
	if len(args) > 0 {
		paneID = args[0]
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] using pane ID from argument: %s\n", time.Now().Format(time.RFC3339), paneID)
		}
	} else {
		paneID = os.Getenv("TMUX_PANE")
		if paneID == "" {
			cmd := exec.Command("tmux", "display-message", "-p", "#{pane_id}")
			if out, err := cmd.Output(); err == nil {
				paneID = strings.TrimSpace(string(out))
			} else if logFile != nil {
				fmt.Fprintf(logFile, "[%s] failed to detect pane: %v\n", time.Now().Format(time.RFC3339), err)
			}
		}
	}

	if paneID == "" {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] no pane ID, exiting\n", time.Now().Format(time.RFC3339))
		}
		return
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] unregistering pane=%s\n", time.Now().Format(time.RFC3339), paneID)
	}

	metadata := map[string]string{
		"pane_id": paneID,
		"pid":     fmt.Sprintf("%d", os.Getpid()),
	}
	data, _ := json.Marshal(metadata)
	if err := store.Publish("soap.unregister", data); err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] failed to publish: %v\n", time.Now().Format(time.RFC3339), err)
		}
	} else if logFile != nil {
		fmt.Fprintf(logFile, "[%s] successfully published unregistration\n", time.Now().Format(time.RFC3339))
	}
}

func handleAddKey(args []string, store *Store) {
	if len(args) < 1 {
		fmt.Println("Usage: soap add-key <key>")
		os.Exit(1)
	}
	keyName := args[0]

	paneID := os.Getenv("TMUX_PANE")
	if paneID == "" {
		cmd := exec.Command("tmux", "display-message", "-p", "#{pane_id}")
		if out, err := cmd.Output(); err == nil {
			paneID = strings.TrimSpace(string(out))
		}
	}
	if paneID == "" {
		return
	}

	os.MkdirAll(keysDir, 0755)

	keyFile := filepath.Join(keysDir, paneID+"."+keyName)
	os.WriteFile(keyFile, []byte{}, 0644)

	metadata := map[string]string{
		"pane_id": paneID,
		"key":     keyName,
	}
	data, _ := json.Marshal(metadata)
	store.Publish("soap.add-key", data)

	fmt.Printf("Added key %s to pane %s\n", keyName, paneID)
}

func handleRemoveKey(args []string, store *Store) {
	if len(args) < 1 {
		fmt.Println("Usage: soap remove-key <key>")
		os.Exit(1)
	}
	keyName := args[0]

	paneID := os.Getenv("TMUX_PANE")
	if paneID == "" {
		cmd := exec.Command("tmux", "display-message", "-p", "#{pane_id}")
		if out, err := cmd.Output(); err == nil {
			paneID = strings.TrimSpace(string(out))
		}
	}
	if paneID == "" {
		return
	}

	keyFile := filepath.Join(keysDir, paneID+"."+keyName)
	os.Remove(keyFile)

	metadata := map[string]string{
		"pane_id": paneID,
		"key":     keyName,
	}
	data, _ := json.Marshal(metadata)
	store.Publish("soap.remove-key", data)

	fmt.Printf("Removed key %s from pane %s\n", keyName, paneID)
}

func handleTick(store *Store) {
	tmux := NewTmuxManager()
	panes := tmux.ListPanes()

	data, _ := json.Marshal(panes)
	store.Publish("soap.tick", data)
	fmt.Printf("tick: %d panes\n", len(panes))
}

func handleSubscribe(args []string, store *Store) {
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

	if err := store.nc.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "Error flushing: %v\n", err)
	}
	fmt.Println("Subscription active, waiting for messages...")

	select {}
}

//go:embed hooks-template.json
var hooksTemplate string

func installHooks(global bool) {
	if global {
		installGlobalHooks()
		return
	}

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

func installHooksInDir(dir string) error {
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}

	settingsFile := filepath.Join(claudeDir, "settings.local.json")

	soapPath, err := os.Executable()
	if err != nil {
		soapPath = "~/.soap/soap"
	}
	template := strings.ReplaceAll(hooksTemplate, "~/.soap/soap", soapPath)

	cmd := exec.Command("jq", "-s", ".[0] * .[1]", settingsFile, "-")
	cmd.Stdin = strings.NewReader(template)

	output, err := cmd.CombinedOutput()
	if err != nil {
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	settingsFile := filepath.Join(homeDir, ".claude.json")

	soapPath, err := os.Executable()
	if err != nil {
		soapPath = "soap"
	}
	template := strings.ReplaceAll(hooksTemplate, "~/.soap/soap", soapPath)

	cmd := exec.Command("jq", "-s", ".[0] * .[1]", settingsFile, "-")
	cmd.Stdin = strings.NewReader(template)

	output, err := cmd.CombinedOutput()
	if err != nil {
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

func installTmuxHooks() {
	soapPath, err := os.Executable()
	if err != nil {
		soapPath = "soap"
	}

	if strings.HasPrefix(soapPath, "~") {
		homeDir, _ := os.UserHomeDir()
		soapPath = strings.Replace(soapPath, "~", homeDir, 1)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	tmuxConfPath := filepath.Join(homeDir, ".tmux.conf")

	hookConfig := fmt.Sprintf(`# SOAP START - Auto-register/unregister terminals
# Register when panes/windows are created
set-hook -g after-split-window "run-shell -b 'sleep 1; %s register #{pane_id}'"
set-hook -g after-new-window "run-shell -b 'sleep 1; %s register #{pane_id}'"
# Unregister when panes exit
set-hook -g pane-exited "run-shell -b '%s unregister #{hook_pane}'"
set-hook -g after-kill-pane "run-shell -b '%s unregister #{hook_pane}'"
# SOAP END - Auto-register/unregister terminals
`, soapPath, soapPath, soapPath, soapPath)

	var existingContent string
	if content, err := os.ReadFile(tmuxConfPath); err == nil {
		existingContent = string(content)
	}

	startMarker := "# SOAP START - Auto-register/unregister terminals"
	endMarker := "# SOAP END - Auto-register/unregister terminals"

	if strings.Contains(existingContent, startMarker) {
		startIdx := strings.Index(existingContent, startMarker)
		endIdx := strings.Index(existingContent, endMarker)
		if startIdx >= 0 && endIdx > startIdx {
			endIdx = strings.Index(existingContent[endIdx:], "\n")
			if endIdx >= 0 {
				endIdx += strings.Index(existingContent, endMarker)
				existingContent = existingContent[:startIdx] + existingContent[endIdx+1:]
			} else {
				existingContent = existingContent[:startIdx]
			}
		}
	}

	newContent := strings.TrimRight(existingContent, "\n") + "\n\n" + hookConfig

	if err := os.WriteFile(tmuxConfPath, []byte(newContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to ~/.tmux.conf: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Installed tmux hooks in ~/.tmux.conf")
	fmt.Println("  Run 'tmux source-file ~/.tmux.conf' to apply changes to existing sessions")
}
