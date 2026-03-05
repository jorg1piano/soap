# soap

A tmux-based ticket management system with git worktree integration and Claude Code support.

## Features

- **Ticket Management**: Create, list, and manage development tickets
- **Git Worktree Integration**: Automatically create and manage git worktrees for each ticket
- **Tmux Integration**: Launch Claude Code sessions in dedicated tmux windows
- **TUI Mode**: Interactive terminal UI for browsing and managing tickets
- **CLI Mode**: Full command-line interface for scripting and automation
- **NATS-based Storage**: Embedded NATS server for ticket persistence across sessions

## Installation

```bash
go build -o soap .
```

## Configuration

Create a `soap.yaml` file in your project directory:

```yaml
# Create a new worktree for a ticket
# Should output the path to the new worktree
createWorktree: |
  (git worktree add -b ticket/{{.ID}} .worktrees/{{.ID}} || git worktree add .worktrees/{{.ID}} ticket/{{.ID}}) 1>&2 && echo .worktrees/{{.ID}}

# Duplicate the current worktree (for working on same ticket in parallel)
duplicateWorktree: |
  git worktree add .worktrees/{{.ID}}-{{.Index}} ticket/{{.ID}} && echo .worktrees/{{.ID}}-{{.Index}}

# Optional setup commands to run after creating a worktree
setup: |
  echo "Working on ticket {{.ID}}: {{.Title}}" > README.ticket.md

# Optional: URL template for opening tickets in browser
openTicket: 'https://your-issue-tracker.com/{{.ID}}'

# Optional: Command for copying ticket links to clipboard
copyTicket: |
  echo "https://your-issue-tracker.com/{{.ID}}" | pbcopy
```

### Template Variables

Commands use Go templates with these variables:
- `{{.ID}}` - Ticket ID
- `{{.Title}}` - Ticket title
- `{{.Worktree}}` - Worktree path (for duplicate command)
- `{{.Index}}` - Duplicate index (for duplicate command)

## Usage

### TUI Mode

Run without arguments to launch the interactive TUI:

```bash
./soap
```

If not already in tmux, soap will automatically start a tmux session.

### Server Mode

Start the embedded NATS server for persistent ticket storage:

```bash
./soap server
```

### CLI Commands

```bash
# Create a new ticket
./soap create "Fix login bug"

# Import ticket from external system (requires listTickets config)
./soap add

# List all tickets
./soap list
./soap list --json

# Select a ticket (creates worktree and opens Claude session)
./soap select T-001

# Duplicate worktree for parallel work
./soap duplicate T-001

# Show ticket details
./soap status T-001

# Delete a ticket (removes worktree and tmux window)
./soap delete T-001

# Find ticket ID from current worktree
./soap whoami

# Install Claude Code hooks (for automatic ping/idle tracking)
./soap install-hooks
./soap install-hooks --global

# Monitor NATS events (debugging)
./soap subscribe
./soap subscribe "soap.ping"
```

## Workflow

1. **Install hooks** (one-time setup):
   ```bash
   ./soap install-hooks
   # Installs Claude Code hooks for automatic activity tracking
   ```

2. **Start the server** (optional, TUI mode starts it automatically):
   ```bash
   ./soap server &
   ```

3. **Create or import a ticket**:
   ```bash
   # Option 1: Create manually
   ./soap create "Implement new feature"
   # Output: Created T-001: Implement new feature

   # Option 2: Import from external system (if listTickets configured)
   ./soap add
   # Shows interactive menu to select ticket
   ```

4. **Select the ticket**:
   ```bash
   ./soap select T-001
   # Creates worktree at .worktrees/T-001
   # Opens Claude Code in tmux window
   ```

5. **Work in parallel** (optional):
   ```bash
   ./soap duplicate T-001
   # Creates additional worktree and Claude session
   ```

6. **Check your context**:
   ```bash
   cd .worktrees/T-001
   ./soap whoami
   # Output: T-001
   ```

7. **Clean up when done**:
   ```bash
   ./soap delete T-001
   # Removes worktree and tmux window
   ```

## Requirements

- Go 1.24.2+
- tmux
- git with worktree support
- Claude Code CLI (`claude`)

## License

MIT
