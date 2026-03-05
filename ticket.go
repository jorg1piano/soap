package main

import (
	"crypto/rand"
	"encoding/hex"
)

// Ticket represents a work item
type Ticket struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Worktree       string `json:"worktree,omitempty"`
	NeedsAttention bool   `json:"needsAttention,omitempty"`
	LastPingTime   int64  `json:"lastPingTime,omitempty"`
}

// Configuration constants
const (
	natsPort    = 14223
	natsDataDir = "/tmp/soap"
	portFile    = "/tmp/soap.port"
)

// generateTicketID creates a short random ID for tickets
func generateTicketID() string {
	bytes := make([]byte, 4) // 8 character hex string
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
