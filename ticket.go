package main

// Terminal represents a registered terminal/session
type Terminal struct {
	PaneID      string          `json:"pane_id"`
	CurrentPath string          `json:"current_path"`
	PID         string          `json:"pid"`
	WindowID    string          `json:"window_id"`
	WindowName  string          `json:"window_name"`
	Keys        map[string]bool `json:"-"` // Runtime only, populated from key files
}

// Key storage directory
const keysDir = "/tmp/soap/keys"

// Configuration constants
const (
	natsPort = 14223
	portFile = "/tmp/soap.port"
)
