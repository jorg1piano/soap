# soap

A tmux-based ticket dashboard with Claude Code integration. Manage tickets from any external system, spin up worktrees, and track Claude sessions across tmux panes — all from a single TUI.

## Features

- **Ticket Dashboard**: Fetches tickets from any external system via a configurable shell command. Reorder, label, select, and delete tickets from the TUI.
- **Configurable Hooks**: `onSelect`, `onDelete`, `openTicket`, `copyTicket`, and `loadTicket` hooks let you wire up your own workflows (e.g. create git worktrees, open browsers, copy links).
- **Tmux Integration**: Auto-launches a tmux session, tracks all panes grouped by window, and lets you switch between them.
- **Claude Code Hooks**: Tracks which panes have active Claude sessions and processing state via key files and NATS events. Installs hooks into Claude Code's `settings.json`.
- **NATS Pub/Sub**: Embedded NATS server for real-time event communication between the TUI and CLI commands.
- **Terminal Labels**: Assign free-text labels to terminals for quick identification.
- **Logging**: All CLI command invocations are logged to `/tmp/soap.log`.

## Installation

```bash
go build -o soap .
```

Optionally copy the binary somewhere on your `$PATH` or to `~/.soap/soap`:

```bash
cp soap ~/.soap/soap
```

## Setup

After building, install the Claude Code hooks so soap can track Claude sessions:

```bash
# Install hooks into project-level .claude/settings.json
./soap install-hooks

# Or install globally into ~/.claude/settings.json
./soap install-hooks --global
```

Install tmux hooks for automatic pane registration:

```bash
./soap install-tmux-hooks
```

## Configuration

Create a `soap.yaml` file next to the binary or in your working directory:

```yaml
# Command to list tickets (must output JSON array with id and title fields)
listTickets: |
  gh issue list --json number,title --jq '[.[] | {id: .number, title: .title}]'

# Command to run when selecting a ticket (e.g. create worktree and open Claude)
onSelect: |
  WORKTREE=".worktrees/{{.ID}}"
  git worktree add -b ticket/{{.ID}} "$WORKTREE" 2>/dev/null || git worktree add "$WORKTREE" ticket/{{.ID}}
  cd "$WORKTREE" && claude

# Command to run when deleting a ticket (e.g. cleanup worktree)
onDelete: |
  git worktree remove --force ".worktrees/{{.ID}}" 2>/dev/null

# Optional: URL template for opening tickets in browser
openTicket: 'https://your-tracker.com/{{.ID}}'

# Optional: Command for copying ticket links
copyTicket: |
  echo "https://your-tracker.com/{{.ID}}" | pbcopy

# Optional: Command to load ticket details by ID (shown in terminal info)
loadTicket: |
  gh issue view {{.ID}} --json title,body --jq '{id: "{{.ID}}", title: .title, description: .body}'

# Optional: Directory to open free Claude sessions in
freeclaudeDir: /tmp/freeclaude
```

### Configuration Reference

| Field | Required | Description |
|-------|----------|-------------|
| `listTickets` | Yes | Shell command that outputs a JSON array of `{id, title}` objects |
| `onSelect` | No | Runs when selecting a ticket (`Enter`/`c`). E.g. create worktree, launch Claude |
| `onDelete` | No | Runs when deleting a ticket (`d`/`Backspace`). E.g. remove worktree |
| `openTicket` | No | URL template opened in browser (`o`) |
| `copyTicket` | No | Shell command to copy ticket link (`y`). E.g. pipe to `pbcopy` |
| `loadTicket` | No | Shell command to load ticket details. Output JSON with `id`, `title`, `description`, `status` fields — shown in terminal info |
| `freeclaudeDir` | No | Directory to open free (non-ticket) Claude sessions in (`f`) |

### Template Variables

All hook commands support Go template variables:

- `{{.ID}}` — Ticket ID
- `{{.Title}}` — Ticket title

## Usage

### TUI Mode

```bash
./soap
```

Launches the interactive dashboard. If not already inside tmux, soap auto-creates a `soap` tmux session.

#### Keybindings

| Key | Action |
|-----|--------|
| `↑`/`k`, `↓`/`j` | Navigate |
| `Shift+↑`/`K`, `Shift+↓`/`J` | Reorder items |
| `Enter`/`c` | Select ticket (runs `onSelect`) |
| `d`/`Backspace` | Delete ticket (runs `onDelete`) |
| `r` | Label a terminal or ticket |
| `e` | Open in VS Code |
| `s` | Switch to pane |
| `f` | Free Claude session |
| `o` | Open ticket in browser |
| `y` | Copy ticket link |
| `q`/`Ctrl+C` | Quit |

### CLI Commands

```bash
# List tickets from external system
soap list
soap list --json

# Run hooks for a ticket
soap select <id>
soap delete <id>

# Manage pane keys (used by Claude hooks)
soap add-key <key>
soap remove-key <key>

# Re-evaluate all tmux pane state
soap tick

# Register/unregister terminal with TUI (called by hooks)
soap register [pane-id]
soap unregister [pane-id]

# Install Claude Code hooks
soap install-hooks [--global]

# Install tmux hooks
soap install-tmux-hooks

# Monitor NATS events (debugging)
soap subscribe [subject]
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│  TUI (Bubble Tea)                               │
│  ┌──────────────┐  ┌────────────────────────┐   │
│  │  Ticket List  │  │  Terminal Sidebar      │   │
│  │  (external)   │  │  (tmux panes + keys)   │   │
│  └──────────────┘  └────────────────────────┘   │
│         │                     ▲                  │
│         ▼                     │                  │
│  ┌─────────────────────────────────────────┐    │
│  │  Embedded NATS Server (port 14223)      │    │
│  └─────────────────────────────────────────┘    │
└─────────────────────────────────────────────────┘
          ▲                     ▲
          │                     │
    ┌─────┴─────┐       ┌──────┴──────┐
    │ soap CLI  │       │ Claude Code │
    │ commands  │       │ hooks       │
    └───────────┘       └─────────────┘
```

- The TUI starts an embedded NATS server and subscribes to events.
- CLI commands (`register`, `unregister`, `add-key`, `remove-key`, `tick`) connect as NATS clients and publish events.
- Claude Code hooks call soap CLI commands on session lifecycle events (see below).
- Key files at `/tmp/soap/keys/` track per-pane, per-session state (format: `{paneID}.{keyName}.{sessionID}`).

### Claude Code Hooks

The `install-hooks` command installs the following hooks into Claude Code's settings. These track Claude session state so the TUI can display live status indicators per pane:

| Event | What soap does | Effect in TUI |
|-------|---------------|---------------|
| `SessionStart` | `register` + `add-key claude` | Pane appears in sidebar with `●` indicator |
| `SessionEnd` | `unregister` + `remove-key claude` | Pane loses `●` indicator |
| `UserPromptSubmit` | `add-key claude-processing` | Pane shows spinner animation |
| `Stop` | `remove-key claude-processing` | Spinner stops |
| `PermissionRequest` | `remove-key claude-processing` | Spinner stops (waiting for user) |
| `PreToolUse` (AskUserQuestion) | `remove-key claude-processing` | Spinner stops (waiting for user) |

### Tmux Hooks

The `install-tmux-hooks` command adds hooks to `~/.tmux.conf` for automatic pane tracking:

| Event | What soap does |
|-------|---------------|
| `after-split-window` | `register` the new pane |
| `after-new-window` | `register` the new pane |
| `pane-exited` | `unregister` the closed pane |
| `after-kill-pane` | `unregister` the killed pane |

## Requirements

- Go 1.24+
- tmux
- Claude Code CLI (optional, for hook integration)

## License

MIT
