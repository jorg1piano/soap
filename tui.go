package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

//go:embed ascii.txt
var asciiArt string

// TUI model and update loop

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Select     key.Binding
	New        key.Binding
	Delete     key.Binding
	Editor     key.Binding
	Shell      key.Binding
	FreeClaude key.Binding
	OpenTicket key.Binding
	CopyTicket key.Binding
	Import     key.Binding
	Quit       key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Select:     key.NewBinding(key.WithKeys("enter", "c"), key.WithHelp("enter/c", "select")),
		New:        key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		Delete:     key.NewBinding(key.WithKeys("d", "backspace"), key.WithHelp("d", "delete")),
		Editor:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "vscode")),
		Shell:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shell")),
		FreeClaude: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "free claude")),
		OpenTicket: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open ticket")),
		CopyTicket: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy link")),
		Import:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "import")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.New, k.Import, k.Editor, k.Shell, k.FreeClaude, k.OpenTicket, k.CopyTicket, k.Delete, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return nil
}

type model struct {
	store  *Store
	tmux   *TmuxManager
	cfg    *Config
	tickets []Ticket
	cursor  int
	keys    keyMap
	help    help.Model
	status  string
	err     error
	// Create mode
	createMode       bool
	createTypeSelect int // 0=story, 1=bug, 2=feature, 3=chore, 4=task
	createTitleInput textinput.Model
	createStep       int // 0=select type, 1=enter title
	// Import mode
	importMode   bool
	importList   []ExternalTicket
	importSelect int
}

type tickMsg struct{}
type statusMsg struct{ msg string }
type listTicketsMsg struct {
	tickets []ExternalTicket
	err     error
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func initialModel(store *Store, tmux *TmuxManager, cfg *Config) model {
	createInput := textinput.New()
	createInput.Placeholder = "Ticket title..."
	createInput.CharLimit = 100
	createInput.Width = 60
	createInput.Blur()

	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	m := model{
		store:            store,
		tmux:             tmux,
		cfg:              cfg,
		createTitleInput: createInput,
		keys:             newKeyMap(),
		help:             h,
		status:           "Ready",
	}
	m.reload()
	return m
}

func (m *model) reload() {
	tickets, err := m.store.AllTickets()
	if err != nil {
		m.err = err
		return
	}
	m.tickets = tickets
	if m.cursor >= len(m.tickets) {
		m.cursor = max(0, len(m.tickets)-1)
	}
}

func (m *model) selectedTicket() *Ticket {
	if m.cursor < len(m.tickets) {
		return &m.tickets[m.cursor]
	}
	return nil
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.status = msg.msg
		m.reload()
		return m, nil

	case listTicketsMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Import failed: %v", msg.err)
			return m, nil
		}
		if len(msg.tickets) == 0 {
			m.status = "No tickets found"
			return m, nil
		}
		m.importMode = true
		m.importList = msg.tickets
		m.importSelect = 0
		m.status = "Select ticket to import (↑/↓ navigate, Enter select, Esc cancel)"
		return m, nil

	case tickMsg:
		if !m.createMode && !m.importMode {
			m.reload()
		}
		return m, tickCmd()

	case tea.KeyMsg:
		if m.createMode {
			return m.handleCreateInput(msg)
		}
		if m.importMode {
			return m.handleImportInput(msg)
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.tickets)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Select):
			return m.handleSelect()
		case key.Matches(msg, m.keys.New):
			return m.handleNew()
		case key.Matches(msg, m.keys.Delete):
			return m.handleDelete()
		case key.Matches(msg, m.keys.Editor):
			return m.handleEditor()
		case key.Matches(msg, m.keys.Shell):
			return m.handleShell()
		case key.Matches(msg, m.keys.FreeClaude):
			return m.handleFreeClaude()
		case key.Matches(msg, m.keys.OpenTicket):
			return m.handleOpenTicket()
		case key.Matches(msg, m.keys.CopyTicket):
			return m.handleCopyTicket()
		case key.Matches(msg, m.keys.Import):
			return m.handleImport()
		}
	}

	if m.createMode && m.createStep == 1 {
		var cmd tea.Cmd
		m.createTitleInput, cmd = m.createTitleInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) handleNew() (tea.Model, tea.Cmd) {
	m.createMode = true
	m.createStep = 0
	m.createTypeSelect = 0
	m.status = "Select ticket type (↑/↓ to navigate, Enter to select, Esc to cancel)"
	return m, nil
}

func (m model) handleCreateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ticketTypes := []string{"story", "bug", "feature", "chore", "task"}

	if m.createStep == 0 {
		// Step 0: Select ticket type
		switch msg.Type {
		case tea.KeyUp, tea.KeyCtrlK:
			if m.createTypeSelect > 0 {
				m.createTypeSelect--
			}
			return m, nil
		case tea.KeyDown, tea.KeyCtrlJ:
			if m.createTypeSelect < len(ticketTypes)-1 {
				m.createTypeSelect++
			}
			return m, nil
		case tea.KeyEnter:
			m.createStep = 1
			m.createTitleInput.SetValue("")
			m.createTitleInput.Focus()
			m.status = fmt.Sprintf("Creating %s ticket — enter title and press Enter (Esc to cancel)", ticketTypes[m.createTypeSelect])
			return m, m.createTitleInput.Cursor.BlinkCmd()
		case tea.KeyEsc:
			m.createMode = false
			m.createStep = 0
			m.createTypeSelect = 0
			m.status = "Ticket creation cancelled"
			return m, nil
		}
	} else {
		// Step 1: Enter title
		switch msg.Type {
		case tea.KeyEnter:
			title := m.createTitleInput.Value()
			if title == "" {
				m.status = "Title cannot be empty"
				return m, nil
			}
			ticketType := ticketTypes[m.createTypeSelect]

			// Use createTicket command if configured, otherwise generate random ID
			ticketID := generateTicketID()
			if m.cfg.CreateTicket != "" {
				cmdStr, err := RenderCreateTicketTemplate(m.cfg.CreateTicket, CreateTicketData{
					Type:  ticketType,
					Title: title,
				})
				if err != nil {
					m.status = fmt.Sprintf("Template error: %v", err)
					return m, nil
				}
				cmd := exec.Command("bash", "-c", strings.TrimSpace(cmdStr))
				out, err := cmd.Output()
				if err != nil {
					m.status = fmt.Sprintf("createTicket failed: %v", err)
					return m, nil
				}
				externalID := strings.TrimSpace(string(out))
				if externalID != "" {
					ticketID = externalID
				}
			}

			ticket := Ticket{
				ID:    ticketID,
				Title: fmt.Sprintf("[%s] %s", ticketType, title),
			}
			if err := m.store.PutTicket(ticket); err != nil {
				m.err = err
			}
			m.reload()
			m.createMode = false
			m.createStep = 0
			m.createTypeSelect = 0
			m.createTitleInput.SetValue("")
			m.status = fmt.Sprintf("Created %s: [%s] %s", ticket.ID, ticketType, title)
			return m, nil

		case tea.KeyEsc:
			m.createMode = false
			m.createStep = 0
			m.createTypeSelect = 0
			m.createTitleInput.SetValue("")
			m.status = "Ticket creation cancelled"
			return m, nil

		default:
			var cmd tea.Cmd
			m.createTitleInput, cmd = m.createTitleInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m model) handleImport() (tea.Model, tea.Cmd) {
	if m.cfg.ListTickets == "" {
		m.status = "No listTickets configured in soap.yaml"
		return m, nil
	}

	m.status = "Fetching tickets..."
	cmdStr := m.cfg.ListTickets

	return m, func() tea.Msg {
		cmd := exec.Command("bash", "-c", strings.TrimSpace(cmdStr))
		out, err := cmd.Output()
		if err != nil {
			return listTicketsMsg{err: err}
		}
		var tickets []ExternalTicket
		if err := json.Unmarshal(out, &tickets); err != nil {
			return listTicketsMsg{err: fmt.Errorf("parse JSON: %w", err)}
		}
		// Filter to ticket kind
		var filtered []ExternalTicket
		for _, t := range tickets {
			if t.Kind == "" || t.Kind == "ticket" {
				filtered = append(filtered, t)
			}
		}
		return listTicketsMsg{tickets: filtered}
	}
}

func (m model) handleImportInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlK:
		if m.importSelect > 0 {
			m.importSelect--
		}
		return m, nil
	case tea.KeyDown, tea.KeyCtrlJ:
		if m.importSelect < len(m.importList)-1 {
			m.importSelect++
		}
		return m, nil
	case tea.KeyEnter:
		ext := m.importList[m.importSelect]
		ticket := Ticket{
			ID:    fmt.Sprintf("%d", ext.ID),
			Title: ext.Title,
		}
		if err := m.store.PutTicket(ticket); err != nil {
			m.err = err
		}
		m.reload()
		m.importMode = false
		m.importList = nil
		m.status = fmt.Sprintf("Imported #%d: %s", ext.ID, ext.Title)
		return m, nil
	case tea.KeyEsc:
		m.importMode = false
		m.importList = nil
		m.status = "Import cancelled"
		return m, nil
	}
	return m, nil
}

func (m model) handleSelect() (tea.Model, tea.Cmd) {
	t := m.selectedTicket()
	if t == nil {
		m.status = "No ticket selected"
		return m, nil
	}

	ticket := *t
	cfg := m.cfg
	store := m.store
	tmux := m.tmux

	m.status = fmt.Sprintf("Selecting %s...", ticket.ID)

	return m, func() tea.Msg {
		var worktreePath string

		if ticket.Worktree != "" {
			worktreePath = ticket.Worktree
		} else {
			if cfg.CreateWorktree == "" {
				return statusMsg{msg: "Error: createWorktree not configured"}
			}

			cmdStr, err := RenderTemplate(cfg.CreateWorktree, TemplateData{
				ID:    ticket.ID,
				Title: ticket.Title,
				Index: 0,
			})
			if err != nil {
				return statusMsg{msg: fmt.Sprintf("Error: %v", err)}
			}

			out, err := runBashCommand(cmdStr)
			if err != nil {
				return statusMsg{msg: fmt.Sprintf("Error creating worktree: %v", err)}
			}

			worktreePath = out
			ticket.Worktree = worktreePath
			store.PutTicket(ticket)

			if cfg.Setup != "" {
				setupCmd, _ := RenderTemplate(cfg.Setup, TemplateData{
					ID:       ticket.ID,
					Title:    ticket.Title,
					Worktree: worktreePath,
					Index:    0,
				})
				runBashCommandInDir(setupCmd, worktreePath)
			}
		}

		if err := tmux.OpenClaudeSession(ticket, worktreePath); err != nil {
			return statusMsg{msg: fmt.Sprintf("Error: %v", err)}
		}

		return statusMsg{msg: fmt.Sprintf("Opened Claude session for %s", ticket.ID)}
	}
}

func (m model) handleDelete() (tea.Model, tea.Cmd) {
	t := m.selectedTicket()
	if t == nil {
		return m, nil
	}

	ticket := *t
	m.tmux.CleanupTicket(ticket.ID)

	if ticket.Worktree != "" {
		cleanupWorktree(ticket.Worktree)
	}

	if err := m.store.DeleteTicket(ticket.ID); err != nil {
		m.err = err
	}

	m.reload()
	m.status = fmt.Sprintf("Deleted %s", ticket.ID)
	return m, nil
}

func (m model) handleEditor() (tea.Model, tea.Cmd) {
	dir := "."
	if t := m.selectedTicket(); t != nil && t.Worktree != "" {
		dir = t.Worktree
	}
	cmd := exec.Command("open", "-a", "Visual Studio Code", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		m.status = fmt.Sprintf("VS Code error: %v (%s)", err, strings.TrimSpace(string(out)))
	} else {
		m.status = fmt.Sprintf("Opened VS Code in %s", dir)
	}
	return m, nil
}

func (m model) handleShell() (tea.Model, tea.Cmd) {
	m.status = "Opening shell tab..."
	tmux := m.tmux

	return m, func() tea.Msg {
		windowName := "shell"
		var err error
		if !tmux.windowExists(windowName) {
			_, err = tmuxRun("new-window", "-d", "-t", tmux.sessionID+":", "-n", windowName)
		} else {
			_, err = tmuxRun("select-window", "-t", tmux.target(windowName))
		}
		if err != nil {
			return statusMsg{msg: fmt.Sprintf("Error: %v", err)}
		}
		return statusMsg{msg: "Opened shell tab"}
	}
}

func (m model) handleFreeClaude() (tea.Model, tea.Cmd) {
	m.status = "Opening free Claude session..."
	tmux := m.tmux

	return m, func() tea.Msg {
		windowName := "claude-free"
		_, err := tmuxRun("select-window", "-t", tmux.target(windowName))
		if err != nil {
			_, err = tmuxRun("new-window", "-t", tmux.sessionID+":", "-n", windowName, "claude")
		}
		if err != nil {
			return statusMsg{msg: fmt.Sprintf("Error: %v", err)}
		}
		return statusMsg{msg: "Opened free Claude session"}
	}
}

func (m model) handleOpenTicket() (tea.Model, tea.Cmd) {
	if m.cfg.OpenTicket == "" {
		m.status = "No openTicket configured in soap.yaml"
		return m, nil
	}
	t := m.selectedTicket()
	if t == nil {
		m.status = "No ticket selected"
		return m, nil
	}
	url, err := RenderTemplate(m.cfg.OpenTicket, TemplateData{
		ID:    t.ID,
		Title: t.Title,
	})
	if err != nil {
		m.status = fmt.Sprintf("Template error: %v", err)
		return m, nil
	}
	url = strings.TrimSpace(url)
	cmd := exec.Command("open", url)
	if err := cmd.Run(); err != nil {
		m.status = fmt.Sprintf("Failed to open: %v", err)
	} else {
		m.status = fmt.Sprintf("Opened %s", url)
	}
	return m, nil
}

func (m model) handleCopyTicket() (tea.Model, tea.Cmd) {
	if m.cfg.CopyTicket == "" {
		m.status = "No copyTicket configured in soap.yaml"
		return m, nil
	}
	t := m.selectedTicket()
	if t == nil {
		m.status = "No ticket selected"
		return m, nil
	}
	rendered, err := RenderTemplate(m.cfg.CopyTicket, TemplateData{
		ID:    t.ID,
		Title: t.Title,
	})
	if err != nil {
		m.status = fmt.Sprintf("Template error: %v", err)
		return m, nil
	}
	rendered = strings.TrimSpace(rendered)
	cmd := exec.Command("sh", "-c", rendered)
	if err := cmd.Run(); err != nil {
		m.status = fmt.Sprintf("Copy failed: %v", err)
	} else {
		m.status = fmt.Sprintf("Copied link for #%s", t.ID)
	}
	return m, nil
}

func (m model) View() string {
	var s strings.Builder

	// ASCII art on the left
	artStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Padding(0, 2, 0, 0)
	artRendered := artStyle.Render(asciiArt)

	// Build right side content
	var rightSide strings.Builder
	rightSide.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("SOAP - Ticket Manager"))
	rightSide.WriteString("\n\n")

	// Ticket list
	if len(m.tickets) == 0 && !m.createMode && !m.importMode {
		rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true).Render("  No tickets"))
		rightSide.WriteString("\n")
	} else if !m.createMode && !m.importMode {
		for i, t := range m.tickets {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			style := lipgloss.NewStyle()
			if i == m.cursor {
				style = style.Foreground(lipgloss.Color("33")).Bold(true)
			}

			line := fmt.Sprintf("%s%-10s %s", cursor, t.ID, t.Title)
			if t.Worktree != "" {
				line += " ✓"
			}

			rightSide.WriteString(style.Render(line))
			rightSide.WriteString("\n")
		}
	}

	rightSide.WriteString("\n")

	// Create mode
	if m.createMode {
		ticketTypes := []string{"story", "bug", "feature", "chore", "task"}
		if m.createStep == 0 {
			rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true).Render("  Create ticket — select type:"))
			rightSide.WriteString("\n")
			for i, t := range ticketTypes {
				prefix := "    "
				if i == m.createTypeSelect {
					prefix = "  > "
				}
				style := lipgloss.NewStyle().Padding(0, 1)
				if i == m.createTypeSelect {
					style = style.Foreground(lipgloss.Color("33")).Bold(true)
				}
				rightSide.WriteString(style.Render(fmt.Sprintf("%s%s", prefix, t)))
				rightSide.WriteString("\n")
			}
		} else {
			ticketType := ticketTypes[m.createTypeSelect]
			rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true).Render(fmt.Sprintf("  Create %s ticket — title: ", ticketType)))
			rightSide.WriteString(m.createTitleInput.View())
			rightSide.WriteString("\n")
		}
		rightSide.WriteString("\n")
	}

	// Import mode
	if m.importMode && len(m.importList) > 0 {
		rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true).Render("  Import ticket:"))
		rightSide.WriteString("\n")
		visibleCount := 8
		start := m.importSelect - visibleCount/2
		if start < 0 {
			start = 0
		}
		end := start + visibleCount
		if end > len(m.importList) {
			end = len(m.importList)
			start = end - visibleCount
			if start < 0 {
				start = 0
			}
		}
		for i := start; i < end; i++ {
			t := m.importList[i]
			prefix := "    "
			if i == m.importSelect {
				prefix = "  > "
			}
			style := lipgloss.NewStyle().Padding(0, 1)
			if i == m.importSelect {
				style = style.Foreground(lipgloss.Color("33")).Bold(true)
			}
			label := fmt.Sprintf("%s#%d %s", prefix, t.ID, t.Title)
			if len(label) > 80 {
				label = label[:77] + "..."
			}
			rightSide.WriteString(style.Render(label))
			rightSide.WriteString("\n")
		}
		if len(m.importList) > visibleCount {
			rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 3).Render(fmt.Sprintf("  %d/%d tickets", m.importSelect+1, len(m.importList))))
			rightSide.WriteString("\n")
		}
		rightSide.WriteString("\n")
	}

	// Status bar
	if m.err != nil {
		rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("  Error: %v", m.err)))
		rightSide.WriteString("\n")
	}
	rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true).Render(fmt.Sprintf("  %s", m.status)))
	rightSide.WriteString("\n\n")

	// Help
	rightSide.WriteString(lipgloss.NewStyle().Padding(0, 1).Render(m.help.View(m.keys)))
	rightSide.WriteString("\n")

	// Join ASCII art on left with content on right
	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, artRendered, rightSide.String()))

	return s.String()
}
