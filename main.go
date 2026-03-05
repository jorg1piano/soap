package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Find and load configuration
	configPath := FindConfigPath()
	if configPath == "" {
		log.Fatal("soap.yaml not found (looked next to binary and in cwd)")
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// If no arguments, run TUI mode
	if len(os.Args) < 2 {
		runTUI(cfg)
		return
	}

	cmd := os.Args[1]

	// Commands that don't need NATS connection
	if cmd == "install-hooks" {
		global := len(os.Args) > 2 && os.Args[2] == "--global"
		installHooks(global)
		return
	}

	// Server mode - starts embedded NATS server and keeps it running
	if cmd == "server" {
		fmt.Println("Starting soap server...")
		store, err := NewServerStore()
		if err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
		defer store.Close()

		fmt.Printf("Server running on port %d\n", natsPort)
		fmt.Println("Press Ctrl+C to stop")

		// Block forever
		select {}
	}

	// Client mode - connect to existing server
	store, err := NewClientStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to soap server: %v\n", err)
		fmt.Fprintf(os.Stderr, "Is the soap server running? Start it with: soap server\n")
		os.Exit(1)
	}
	defer store.Close()

	tmux := NewTmuxManager()

	runCLI(os.Args[1:], cfg, store, tmux)
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

	// Connect to server
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
