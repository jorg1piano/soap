package main

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Config represents the soap.yaml configuration
type Config struct {
	// Worktree commands
	CreateWorktree    string `yaml:"createWorktree"`    // Template for creating a new worktree
	DuplicateWorktree string `yaml:"duplicateWorktree"` // Template for duplicating current worktree
	Setup             string `yaml:"setup"`             // Commands to run after worktree creation

	// Ticket integration
	OpenTicket   string `yaml:"openTicket"`   // URL template for opening tickets
	CopyTicket   string `yaml:"copyTicket"`   // Command for copying ticket links
	ListTickets  string `yaml:"listTickets"`  // Command to list external tickets (JSON output)
	CreateTicket string `yaml:"createTicket"` // Command to create ticket in external system
}

// ExternalTicket represents a ticket from an external system
type ExternalTicket struct {
	Kind  string      `json:"kind,omitempty"`  // Optional kind field
	ID    interface{} `json:"id"`              // Can be string or int
	Title string      `json:"title"`
	State string      `json:"state,omitempty"` // Optional state
	Type  string      `json:"type,omitempty"`  // Optional type
	URL   string      `json:"url,omitempty"`   // Optional URL
}

// CreateTicketData holds data for rendering createTicket template
type CreateTicketData struct {
	Type  string // Ticket type (story, bug, feature, etc.)
	Title string // Ticket title
}

// TemplateData holds data for template rendering
type TemplateData struct {
	ID       string // Ticket ID
	Title    string // Ticket title
	Worktree string // Worktree path
	Index    int    // Worktree index (for multiple worktrees)
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// RenderTemplate renders a Go template string with the given data
func RenderTemplate(tmpl string, data TemplateData) (string, error) {
	t, err := template.New("tmpl").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// RenderCreateTicketTemplate renders a createTicket template with the given data
func RenderCreateTicketTemplate(tmpl string, data CreateTicketData) (string, error) {
	t, err := template.New("createTicket").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// FindConfigPath looks for soap.yaml in standard locations
func FindConfigPath() string {
	// Look next to the binary
	if self, err := os.Executable(); err == nil {
		p := self[:len(self)-len("soap")] + "soap.yaml"
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// Look in current directory
	if _, err := os.Stat("soap.yaml"); err == nil {
		return "soap.yaml"
	}
	return ""
}
