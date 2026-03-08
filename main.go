package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// If no arguments, run TUI mode
	if len(os.Args) < 2 {
		configPath := FindConfigPath()
		if configPath == "" {
			log.Fatal("soap.yaml not found (looked next to binary and in cwd)")
		}
		cfg, err := LoadConfig(configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		runTUI(cfg)
		return
	}

	cmd := os.Args[1]

	// Commands that don't need config file or NATS connection
	if cmd == "install-hooks" {
		global := len(os.Args) > 2 && os.Args[2] == "--global"
		installHooks(global)
		return
	}

	if cmd == "install-tmux-hooks" {
		installTmuxHooks()
		return
	}

	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		runCLI(nil, nil, nil, nil)
		return
	}

	// Commands that only need NATS (no config file required)
	if cmd == "register" || cmd == "unregister" || cmd == "tick" || cmd == "add-key" || cmd == "remove-key" || cmd == "subscribe" {
		store, err := NewClientStore()
		if err != nil {
			logFile, _ := os.OpenFile("/tmp/soap-register.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if logFile != nil {
				fmt.Fprintf(logFile, "[%s] NATS connection error: %v\n", time.Now().Format(time.RFC3339), err)
				logFile.Close()
			}
			fmt.Fprintf(os.Stderr, "Error connecting to soap server: %v\n", err)
			os.Exit(1)
		}
		defer store.Close()

		tmux := NewTmuxManager()
		runCLI(os.Args[1:], nil, store, tmux)
		return
	}

	// Commands that need config file (list, select, delete)
	configPath := FindConfigPath()
	if configPath == "" {
		log.Fatal("soap.yaml not found (looked next to binary and in cwd)")
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	tmux := NewTmuxManager()
	runCLI(os.Args[1:], cfg, nil, tmux)
}

func runTUI(cfg *Config) {
	// Auto-launch tmux if not inside it
	if os.Getenv("TMUX") == "" {
		fmt.Println("Not inside tmux. Starting a tmux session...")
		self, _ := os.Executable()
		if self == "" {
			self = os.Args[0]
		}
		cmd := exec.Command("tmux", "new-session", "-s", tmuxSession, self)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("tmux: %v", err)
		}
		return
	}

	// Start embedded NATS server for pub/sub
	store, err := NewServerStore()
	if err != nil {
		log.Fatalf("Failed to start store: %v", err)
	}
	defer store.Close()

	tmux := NewTmuxManager()

	p := tea.NewProgram(initialModel(store, tmux, cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
