# soap

A tmux-based ticket dashboard with Claude Code integration.

## Features

- **Ticket List**: Fetches tickets from any external system via configurable command
- **Configurable Hooks**: `onSelect` and `onDelete` run your own scripts
- **Tmux Integration**: TUI runs in tmux, tracks terminal panes and Claude sessions
- **Claude Code Hooks**: Tracks which panes have active Claude sessions via key files
- **NATS Pub/Sub**: Embedded NATS server for real-time event communication

## Installation

```bash
go build -o soap .
```

## Configuration

Create a `soap.yaml` file next to the binary or in your project directory:

```yaml
# Command to list tickets (must output JSON array with id and title fields)
listTickets: |
  gh issue list --json number,title --jq '[.[] | {id: .number, title: .title}]'

# Command to run when selecting a ticket
onSelect: |
  WORKTREE=".worktrees/{{.ID}}"
  git worktree add -b ticket/{{.ID}} "$WORKTREE" 2>/dev/null || git worktree add "$WORKTREE" ticket/{{.ID}}
  cd "$WORKTREE" && claude

# Command to run when deleting a ticket
onDelete: |
  git worktree remove --force ".worktrees/{{.ID}}" 2>/dev/null

# Optional: URL template for opening tickets in browser
openTicket: 'https://your-tracker.com/{{.ID}}'

# Optional: Command for copying ticket links
copyTicket: |
  echo "https://your-tracker.com/{{.ID}}" | pbcopy

# Optional: Command to load ticket details by ID
loadTicket: 'echo Ticket: {{.ID}}'
```

### Template Variables

- `{{.ID}}` - Ticket ID
- `{{.Title}}` - Ticket title

## Usage

### TUI Mode

```bash
./soap
```

Launches the interactive TUI. Auto-starts a tmux session if not already inside one.

### CLI Commands

```bash
# List tickets from external system
./soap list
./soap list --json

# Run onSelect hook for a ticket
./soap select 12345

# Run onDelete hook for a ticket
./soap delete 12345

# Manage pane keys (used by Claude hooks)
./soap add-key claude
./soap remove-key claude

# Install Claude Code hooks
./soap install-hooks
./soap install-hooks --global

# Monitor NATS events (debugging)
./soap subscribe
```

## Requirements

- Go 1.24.2+
- tmux
- Claude Code CLI (`claude`)

## License

MIT
