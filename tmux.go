package main

import (
	"os"
	"os/exec"
	"strings"
)

const tmuxSession = "soap"

// TmuxManager manages tmux windows
type TmuxManager struct {
	sessionID string
	selfPath  string
	selfPane  string
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
		selfPane:  os.Getenv("TMUX_PANE"),
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

// ListPanes queries tmux for all panes in the current session and returns terminals
func (tm *TmuxManager) ListPanes() []Terminal {
	out, err := tmuxRun("list-panes", "-s", "-t", tm.sessionID, "-F", "#{pane_id}\t#{pane_current_path}\t#{pane_pid}")
	if err != nil {
		return nil
	}

	var terminals []Terminal
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		paneID := parts[0]
		// Skip our own pane
		if paneID == tm.selfPane {
			continue
		}
		term := Terminal{
			PaneID:      paneID,
			CurrentPath: parts[1],
		}
		if len(parts) >= 3 {
			term.PID = parts[2]
		}
		terminals = append(terminals, term)
	}
	return terminals
}
