package main

// Terminal represents a registered terminal/session
type Terminal struct {
	PaneID      string          `json:"pane_id"`
	CurrentPath string          `json:"current_path"`
	PID         string          `json:"pid"`
	Keys        map[string]bool `json:"-"` // Runtime only, populated from key files
}

// Key storage directory
const keysDir = "/tmp/soap/keys"

// Configuration constants
const (
	natsPort = 14223
	portFile = "/tmp/soap.port"
)
