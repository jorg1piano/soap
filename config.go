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
	// Ticket integration
	ListTickets string `yaml:"listTickets"` // Command to list tickets (must output JSON array)
	OnSelect    string `yaml:"onSelect"`    // Command to run when selecting a ticket
	OnDelete    string `yaml:"onDelete"`    // Command to run when deleting a ticket
	OpenTicket  string `yaml:"openTicket"`  // URL template for opening tickets in browser
	CopyTicket  string `yaml:"copyTicket"`  // Command for copying ticket links
	LoadTicket  string `yaml:"loadTicket"`  // Command to load ticket details by ID
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

// TicketID returns the ID as a string regardless of underlying type
func (t ExternalTicket) TicketID() string {
	switch v := t.ID.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// TemplateData holds data for template rendering
type TemplateData struct {
	ID    string // Ticket ID
	Title string // Ticket title
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
