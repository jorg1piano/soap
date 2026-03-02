package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const tmuxSession = "soap"

// TmuxManager manages tmux windows for Claude sessions
type TmuxManager struct {
	sessionID string
	selfPath  string
}

// NewTmuxManager creates a new TmuxManager
func NewTmuxManager() *TmuxManager {
	self, _ := os.Executable()
	if self == "" {
		self = os.Args[0]
	}
	sid, _ := tmuxRun("display-message", "-p", "#{session_id}")
	return &TmuxManager{
		sessionID: sid,
		selfPath:  self,
	}
}

// tmuxRun executes a tmux command and returns the output
func tmuxRun(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// target returns a tmux target for a window name
func (tm *TmuxManager) target(windowName string) string {
	return tm.sessionID + ":" + windowName
}

// windowExists checks if a tmux window exists
func (tm *TmuxManager) windowExists(name string) bool {
	_, err := tmuxRun("display-message", "-t", tm.target(name), "-p", "#{window_id}")
	return err == nil
}

// OpenClaudeSession opens a Claude session in tmux for a ticket
func (tm *TmuxManager) OpenClaudeSession(ticket Ticket, worktree string) error {
	windowName := fmt.Sprintf("ticket-%s", ticket.ID)

	// Check if window already exists
	if tm.windowExists(windowName) {
		// Switch to existing window
		_, err := tmuxRun("select-window", "-t", tm.target(windowName))
		return err
	}

	// Create new window
	args := []string{"new-window", "-t", tm.sessionID + ":", "-n", windowName}
	_, err := tmuxRun(args...)
	if err != nil {
		return fmt.Errorf("new-window: %w", err)
	}

	target := tm.target(windowName)

	// Change to worktree directory
	if worktree != "" {
		tmuxRun("send-keys", "-t", target, fmt.Sprintf("cd %q", worktree), "C-m")
		time.Sleep(200 * time.Millisecond)
	}

	// Export SOAP_TICKET_ID for hooks
	tmuxRun("send-keys", "-t", target, fmt.Sprintf("export SOAP_TICKET_ID=%q", ticket.ID), "C-m")
	time.Sleep(100 * time.Millisecond)

	// Launch Claude
	tmuxRun("send-keys", "-t", target, "claude", "C-m")
	time.Sleep(3 * time.Second)

	// Auto-trust (select option 1)
	tmuxRun("send-keys", "-t", target, "1")
	time.Sleep(500 * time.Millisecond)
	tmuxRun("send-keys", "-t", target, "C-m")

	return nil
}

// CleanupTicket kills any tmux windows for a ticket
func (tm *TmuxManager) CleanupTicket(ticketID string) {
	windowName := fmt.Sprintf("ticket-%s", ticketID)
	if tm.windowExists(windowName) {
		tmuxRun("kill-window", "-t", tm.target(windowName))
	}
}
