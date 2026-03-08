package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
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
	Delete     key.Binding
	Editor     key.Binding
	Shell      key.Binding
	FreeClaude key.Binding
	OpenTicket key.Binding
	CopyTicket key.Binding
	Quit       key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Select:     key.NewBinding(key.WithKeys("enter", "c"), key.WithHelp("enter/c", "select")),
		Delete:     key.NewBinding(key.WithKeys("d", "backspace"), key.WithHelp("d", "delete")),
		Editor:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "vscode")),
		Shell:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "switch pane")),
		FreeClaude: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "free claude")),
		OpenTicket: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open ticket")),
		CopyTicket: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy link")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Editor, k.Shell, k.FreeClaude, k.OpenTicket, k.CopyTicket, k.Delete, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return nil
}

type model struct {
	store           *Store
	tmux            *TmuxManager
	cfg             *Config
	tickets         []ExternalTicket
	cursor          int
	keys            keyMap
	help            help.Model
	status          string
	err             error
	terminals       map[string]Terminal // pane_id -> terminal info
	terminalPaneIDs []string            // sorted list of pane IDs for selection
	terminalCursor  int                 // cursor position in terminals list
	ticketInfo      map[string]string   // pane_id -> loadTicket output
	animFrame       int                 // animation frame counter for spinners
}

type tickMsg struct{}
type animTickMsg struct{}
type statusMsg struct{ msg string }
type listTicketsMsg struct {
	tickets []ExternalTicket
	err     error
}
type ticketInfoMsg struct {
	info map[string]string
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func animTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return animTickMsg{}
	})
}

func initialModel(store *Store, tmux *TmuxManager, cfg *Config) model {
	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	m := model{
		store:      store,
		tmux:       tmux,
		cfg:        cfg,
		keys:       newKeyMap(),
		help:       h,
		status:     "Loading tickets...",
		terminals:  make(map[string]Terminal),
		ticketInfo: make(map[string]string),
	}
	return m
}

func (m *model) fetchTicketsCmd() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		tickets, err := fetchTickets(cfg)
		return listTicketsMsg{tickets: tickets, err: err}
	}
}

func (m *model) updateTerminalList() {
	var selectedPaneID string
	if m.terminalCursor < len(m.terminalPaneIDs) {
		selectedPaneID = m.terminalPaneIDs[m.terminalCursor]
	}

	m.terminalPaneIDs = make([]string, 0, len(m.terminals))
	for paneID := range m.terminals {
		m.terminalPaneIDs = append(m.terminalPaneIDs, paneID)
	}
	sort.Strings(m.terminalPaneIDs)

	if selectedPaneID != "" {
		for i, id := range m.terminalPaneIDs {
			if id == selectedPaneID {
				m.terminalCursor = i
				return
			}
		}
	}
	if m.terminalCursor >= len(m.terminalPaneIDs) {
		m.terminalCursor = max(0, len(m.terminalPaneIDs)-1)
	}
}

func (m *model) refreshTicketInfoCmd() tea.Cmd {
	loadCmd := m.cfg.LoadTicket
	if loadCmd == "" {
		return nil
	}

	terminals := make(map[string]Terminal, len(m.terminals))
	for k, v := range m.terminals {
		terminals[k] = v
	}

	return func() tea.Msg {
		info := make(map[string]string)
		for paneID, term := range terminals {
			dir := filepath.Base(term.CurrentPath)
			ticketID := extractTicketIDFromDir(dir)
			if ticketID == "" {
				continue
			}
			rendered, err := RenderTemplate(loadCmd, TemplateData{ID: ticketID})
			if err != nil {
				continue
			}
			cmd := exec.Command("sh", "-c", strings.TrimSpace(rendered))
			out, err := cmd.Output()
			if err != nil {
				continue
			}
			info[paneID] = strings.TrimSpace(string(out))
		}
		return ticketInfoMsg{info: info}
	}
}

func (m *model) selectedTicket() *ExternalTicket {
	if m.cursor < len(m.tickets) {
		return &m.tickets[m.cursor]
	}
	return nil
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), animTickCmd(), m.fetchTicketsCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		m.status = msg.msg
		return m, nil

	case listTicketsMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		// Preserve cursor position
		var selectedID string
		if m.cursor < len(m.tickets) {
			selectedID = m.tickets[m.cursor].TicketID()
		}
		m.tickets = msg.tickets
		if selectedID != "" {
			for i, t := range m.tickets {
				if t.TicketID() == selectedID {
					m.cursor = i
					break
				}
			}
		}
		if m.cursor >= len(m.tickets) && m.cursor >= 1 {
			m.cursor = max(0, len(m.tickets)-1)
		}
		if m.status == "Loading tickets..." {
			m.status = "Ready"
		}
		return m, nil

	case animTickMsg:
		m.animFrame = (m.animFrame + 1) % len(spinnerFrames)
		return m, animTickCmd()

	case ticketInfoMsg:
		m.ticketInfo = msg.info
		return m, nil

	case tickMsg:
		// Scan tmux panes to refresh terminal state
		panes := m.tmux.ListPanes()
		m.terminals = make(map[string]Terminal)
		for _, p := range panes {
			p.Keys = loadPaneKeys(p.PaneID)
			m.terminals[p.PaneID] = p
		}
		m.updateTerminalList()
		return m, tea.Batch(tickCmd(), m.fetchTicketsCmd(), m.refreshTicketInfoCmd())

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Up):
			totalItems := len(m.tickets) + len(m.terminalPaneIDs)
			if totalItems == 0 {
				break
			}
			currentPos := m.cursor
			if m.cursor >= len(m.tickets) {
				currentPos = len(m.tickets) + m.terminalCursor
			}
			if currentPos > 0 {
				currentPos--
				if currentPos < len(m.tickets) {
					m.cursor = currentPos
					m.terminalCursor = 0
				} else {
					m.cursor = len(m.tickets)
					m.terminalCursor = currentPos - len(m.tickets)
				}
			}
		case key.Matches(msg, m.keys.Down):
			totalItems := len(m.tickets) + len(m.terminalPaneIDs)
			if totalItems == 0 {
				break
			}
			currentPos := m.cursor
			if m.cursor >= len(m.tickets) {
				currentPos = len(m.tickets) + m.terminalCursor
			}
			if currentPos < totalItems-1 {
				currentPos++
				if currentPos < len(m.tickets) {
					m.cursor = currentPos
					m.terminalCursor = 0
				} else {
					m.cursor = len(m.tickets)
					m.terminalCursor = currentPos - len(m.tickets)
				}
			}
		case key.Matches(msg, m.keys.Select):
			return m.handleSelect()
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
		}
	}

	return m, nil
}

func (m model) handleSelect() (tea.Model, tea.Cmd) {
	t := m.selectedTicket()
	if t == nil {
		m.status = "No ticket selected"
		return m, nil
	}

	if m.cfg.OnSelect == "" {
		m.status = "No onSelect configured in soap.yaml"
		return m, nil
	}

	ticket := *t
	cfg := m.cfg

	m.status = fmt.Sprintf("Selecting %s...", ticket.TicketID())

	return m, func() tea.Msg {
		cmdStr, err := RenderTemplate(cfg.OnSelect, TemplateData{
			ID:    ticket.TicketID(),
			Title: ticket.Title,
		})
		if err != nil {
			return statusMsg{msg: fmt.Sprintf("Error: %v", err)}
		}

		cmd := exec.Command("bash", "-c", strings.TrimSpace(cmdStr))
		if out, err := cmd.CombinedOutput(); err != nil {
			return statusMsg{msg: fmt.Sprintf("onSelect failed: %v (%s)", err, strings.TrimSpace(string(out)))}
		}

		return statusMsg{msg: fmt.Sprintf("Selected %s", ticket.TicketID())}
	}
}

func (m model) handleDelete() (tea.Model, tea.Cmd) {
	// If cursor is in the terminals section, kill the pane
	if m.cursor >= len(m.tickets) && len(m.terminalPaneIDs) > 0 {
		if m.terminalCursor >= len(m.terminalPaneIDs) {
			return m, nil
		}
		paneID := m.terminalPaneIDs[m.terminalCursor]
		_, err := tmuxRun("kill-pane", "-t", paneID)
		if err != nil {
			m.status = fmt.Sprintf("Error killing pane %s: %v", paneID, err)
		} else {
			delete(m.terminals, paneID)
			m.updateTerminalList()
			m.status = fmt.Sprintf("Killed pane %s", paneID)
		}
		return m, nil
	}

	t := m.selectedTicket()
	if t == nil {
		return m, nil
	}

	if m.cfg.OnDelete == "" {
		m.status = "No onDelete configured in soap.yaml"
		return m, nil
	}

	ticket := *t
	cfg := m.cfg

	m.status = fmt.Sprintf("Deleting %s...", ticket.TicketID())

	return m, func() tea.Msg {
		cmdStr, err := RenderTemplate(cfg.OnDelete, TemplateData{
			ID:    ticket.TicketID(),
			Title: ticket.Title,
		})
		if err != nil {
			return statusMsg{msg: fmt.Sprintf("Error: %v", err)}
		}

		cmd := exec.Command("bash", "-c", strings.TrimSpace(cmdStr))
		if out, err := cmd.CombinedOutput(); err != nil {
			return statusMsg{msg: fmt.Sprintf("onDelete failed: %v (%s)", err, strings.TrimSpace(string(out)))}
		}

		return statusMsg{msg: fmt.Sprintf("Deleted %s", ticket.TicketID())}
	}
}

func (m model) handleEditor() (tea.Model, tea.Cmd) {
	cmd := exec.Command("open", "-a", "Visual Studio Code", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		m.status = fmt.Sprintf("VS Code error: %v (%s)", err, strings.TrimSpace(string(out)))
	} else {
		m.status = "Opened VS Code"
	}
	return m, nil
}

func (m model) handleShell() (tea.Model, tea.Cmd) {
	if len(m.terminalPaneIDs) == 0 {
		m.status = "No registered terminals"
		return m, nil
	}

	if m.terminalCursor >= len(m.terminalPaneIDs) {
		m.status = "Invalid terminal selection"
		return m, nil
	}

	paneID := m.terminalPaneIDs[m.terminalCursor]
	m.status = fmt.Sprintf("Switching to pane %s...", paneID)

	return m, func() tea.Msg {
		cmd := exec.Command("tmux", "switch-client", "-t", paneID)
		if err := cmd.Run(); err != nil {
			return statusMsg{msg: fmt.Sprintf("Error switching pane: %v", err)}
		}
		return statusMsg{msg: fmt.Sprintf("Switched to %s", paneID)}
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
		ID:    t.TicketID(),
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
		ID:    t.TicketID(),
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
		m.status = fmt.Sprintf("Copied link for #%s", t.TicketID())
	}
	return m, nil
}

func (m model) View() string {
	var s strings.Builder

	artStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Padding(0, 2, 0, 0)
	artRendered := artStyle.Render(asciiArt)

	var rightSide strings.Builder
	rightSide.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("SOAP - Ticket Manager"))
	rightSide.WriteString("\n\n")

	// Ticket list
	if len(m.tickets) == 0 {
		rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true).Render("  No tickets"))
		rightSide.WriteString("\n")
	} else {
		for i, t := range m.tickets {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			style := lipgloss.NewStyle()
			if i == m.cursor {
				style = style.Foreground(lipgloss.Color("33")).Bold(true)
			}

			line := fmt.Sprintf("%s%-10s %s", cursor, t.TicketID(), t.Title)

			rightSide.WriteString(style.Render(line))
			rightSide.WriteString("\n")
		}
	}

	rightSide.WriteString("\n")

	// Registered terminals section
	if len(m.terminals) > 0 {
		rightSide.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).Render("Terminals"))
		rightSide.WriteString("\n")

		orangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C00"))
		orangeBoldStyle := orangeStyle.Bold(true)
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)

		for i, paneID := range m.terminalPaneIDs {
			term := m.terminals[paneID]
			isSelected := i == m.terminalCursor && m.cursor >= len(m.tickets)
			hasClaude := term.Keys["claude"]
			isProcessing := term.Keys["claude-processing"]

			ticketID := "_____"
			dirName := filepath.Base(term.CurrentPath)
			if id := extractTicketIDFromDir(dirName); id != "" {
				ticketID = id
			}

			indicator := " "
			if isProcessing {
				indicator = spinnerFrames[m.animFrame]
			} else if hasClaude {
				indicator = "●"
			}

			cursorStr := "  "
			if isSelected {
				cursorStr = "> "
			}

			var lineStyle lipgloss.Style
			if isSelected {
				lineStyle = selectedStyle
			} else if hasClaude {
				if isProcessing {
					lineStyle = orangeBoldStyle
				} else {
					lineStyle = orangeStyle
				}
			} else {
				lineStyle = normalStyle
			}

			indicatorStyle := lineStyle
			if isProcessing && !isSelected {
				indicatorStyle = orangeBoldStyle
			}

			ticketIDStyle := lineStyle
			if ticketID == "_____" && !isSelected {
				ticketIDStyle = dimStyle
			}

			loadedInfo := ""
			if info, ok := m.ticketInfo[paneID]; ok {
				loadedInfo = " " + info
			}

			line := fmt.Sprintf("%s%s %s %s%s",
				cursorStr,
				indicatorStyle.Render(indicator),
				ticketIDStyle.Render(fmt.Sprintf("%-7s", ticketID)),
				lineStyle.Render(dirName),
				lineStyle.Render(loadedInfo),
			)
			rightSide.WriteString(line)
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

	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, artRendered, rightSide.String()))

	return s.String()
}

// loadPaneKeys reads key marker files for a given pane ID from /tmp/soap/keys/
func loadPaneKeys(paneID string) map[string]bool {
	keys := make(map[string]bool)
	entries, err := os.ReadDir(keysDir)
	if err != nil {
		return keys
	}
	prefix := paneID + "."
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			key := strings.TrimPrefix(e.Name(), prefix)
			if key != "" {
				keys[key] = true
			}
		}
	}
	return keys
}

// extractTicketIDFromDir tries to extract a ticket ID from a directory name
func extractTicketIDFromDir(dir string) string {
	// Look for patterns like "story-12345-description" or just numeric IDs
	parts := strings.Split(dir, "-")
	for _, p := range parts {
		if len(p) > 0 && p[0] >= '0' && p[0] <= '9' {
			allDigits := true
			for _, c := range p {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return p
			}
		}
	}
	return ""
}
