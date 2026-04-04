---
name: ccc-team-session
description: CCC Team Session Management. Use when creating, starting, or managing a 3-pane team session (planner/executor/reviewer). Covers tmux window setup, pane naming, and CCC_ROLE environment variable configuration.
---

# CCC Team Session Management

Manage 3-pane team sessions where each pane runs a separate Claude Code instance with a specialized role.

## Architecture

```text
Session (tmux session)
  └─ Window (tmux window) - "ccc-team:<session-name>"
       ├─ Pane 0: Planner  (CCC_ROLE=planner)
       ├─ Pane 1: Executor (CCC_ROLE=executor)
       └─ Pane 2: Reviewer (CCC_ROLE=reviewer)
```

## Role Responsibilities

| Role | Responsibility | Key Commands |
|------|---------------|--------------|
| **Planner** | Task decomposition, delegation | `@executor`, `@reviewer` mentions |
| **Executor** | Code implementation, testing | `@planner` for questions, `@reviewer` for review |
| **Reviewer** | Code review, approval | `@executor` for revisions, `@planner` for sign-off |

---

## Creating a Team Session

### Using CCC CLI

```bash
# Create a new team session with 3 panes
ccc team create myproject

# Resume an existing team session
ccc team attach myproject

# List all team sessions
ccc team list
```

### Manual Setup (if needed)

```bash
# Create tmux session with window (Planner | Executor | Reviewer)
tmux new-session -d -s "ccc-team" -n "myproject"
tmux split-window -h -t "ccc-team:myproject"   # Split horizontally for panes 0 and 1
tmux split-window -h -t "ccc-team:myproject.1" # Split horizontally for pane 2

# Name each pane by its role
tmux select-pane -t "ccc-team:myproject.0" -T "Planner"
tmux select-pane -t "ccc-team:myproject.1" -T "Executor"
tmux select-pane -t "ccc-team:myproject.2" -T "Reviewer"

# Verify pane names
tmux list-panes -t "ccc-team:myproject" -F "#{pane_index}: #{pane_title}"
```

---

## Starting Claude in Each Pane

### With CCC (Recommended)

```bash
# Start all 3 panes with roles
ccc team start myproject

# Start specific pane only
ccc team start myproject --role executor
```

### Manual with Environment Variables

Each pane needs `CCC_ROLE` set before starting Claude:

```bash
# Pane 0 - Planner
cd /path/to/project
CCC_ROLE=planner claude

# Pane 1 - Executor
cd /path/to/project
CCC_ROLE=executor claude

# Pane 2 - Reviewer
cd /path/to/project
CCC_ROLE=reviewer claude
```

### Using tmux send-keys

```bash
# Send command to pane 0 (Planner)
tmux send-keys -t "ccc-team:myproject.0" "CCC_ROLE=planner claude" Enter

# Send command to pane 1 (Executor)
tmux send-keys -t "ccc-team:myproject.1" "CCC_ROLE=executor claude" Enter

# Send command to pane 2 (Reviewer)
tmux send-keys -t "ccc-team:myproject.2" "CCC_ROLE=reviewer claude" Enter
```

---

## Pane Layouts

### Equal 3-Pane Layout

```text
┌─────────┬─────────┬─────────┐
│Planner  │Executor │Reviewer │
│  33%    │  33%    │  33%    │
└─────────┴─────────┴─────────┘
```

### Planner-Focused Layout (60/20/20)

```text
┌────────────────┬────────┬────────┐
│    Planner     │Executor│Reviewer│
│      60%       │  20%   │  20%   │
└────────────────┴────────┴────────┘
```

Set layout:
```bash
tmux select-layout -t "ccc-team:myproject" main-horizontal
```

---

## Verifying Setup

### Check Pane Status

```bash
# List all panes with titles
tmux list-panes -t "ccc-team:myproject" -F "#{pane_index}: #{pane_title} [#{pane_active}]"

# Example output:
# 0: Planner [1]   <- active (current pane)
# 1: Executor [0]
# 2: Reviewer [0]
```

### Verify CCC_ROLE in Each Pane

```bash
# In each pane, run:
echo $CCC_ROLE

# Expected outputs:
# Pane 0: planner
# Pane 1: executor
# Pane 2: reviewer
```

### Test Inter-Pane Communication

```bash
# From Planner, send test message to Executor
tmux send-keys -t "ccc-team:myproject.1" "ping" Enter

# Check Executor pane for the message
tmux capture-pane -t "ccc-team:myproject.1" -p
```

---

## Attaching to a Team Session

### Attach All Panes (Terminal Multiplexer)

```bash
# Attach to the tmux session (shows all panes)
tmux attach -t "ccc-team:myproject"

# Or use ccc attach
ccc attach myproject
```

### Attach to Specific Pane Only

```bash
# Send target pane to background and attach
tmux select-pane -t "ccc-team:myproject.1"
tmux attach -t "ccc-team:myproject"
```

---

## Session Lifecycle

### Create → Start → Work → Pause → Resume → Close

```text
Create → Start → Work ←→ Pause ←→ Resume → Close
              ↓
         [3 panes running]
```

### Pause (Detach Without Closing)

```bash
tmux detach  # Ctrl+b d
```

### Resume (Reattach)

```bash
tmux attach -t "ccc-team:myproject"
```

### Close (Kill Session)

```bash
tmux kill-session -t "ccc-team:myproject"
```

---

## Troubleshooting

| Issue | Cause | Solution |
|-------|-------|----------|
| Pane title empty | Not set during creation | `tmux select-pane -t :.0 -T "Planner"` |
| CCC_ROLE empty | Not set before claude start | Restart pane with env var |
| Cannot send to pane | Wrong pane index | Check with `tmux list-panes` |
| Session not found | Wrong session name | Check `tmux list-sessions` |

### Common Commands

```bash
# List all tmux sessions
tmux list-sessions

# List all windows in a session
tmux list-windows -t "ccc-team:myproject"

# List all panes in a window
tmux list-panes -t "ccc-team:myproject"

# Rename a pane title
tmux select-pane -t "ccc-team:myproject.0" -T "Planner"

# Resize a pane
tmux resize-pane -t "ccc-team:myproject.0" -x 80 -y 24
```

---

## Auto-Load Setup

CCC automatically adds a SessionStart hook when you run `ccc install`. This hook detects `CCC_ROLE` at session startup and exports it to `CLAUDE_ENV_FILE` for session persistence. This enables the ccc-interpane skill to auto-load based on the role.

No manual configuration needed - the hook is managed by CCC.

---

## Integration with CCC Interpane Skill

The team-session skill works with the `ccc-interpane` skill for messaging:

1. Team session creates the 3-pane layout
2. Each pane gets its `CCC_ROLE` set
3. The SessionStart hook triggers interpane skill auto-load
4. Panes communicate via @mentions using tmux send-keys

See also: `ccc-interpane` skill for inter-pane messaging.
