---
name: ccc-team
description: CCC Team Session Management and Inter-Pane Communication for 3-pane tmux team sessions (planner/executor/reviewer). Use when creating, managing, or communicating in a team session. Auto-loads when CCC_ROLE is set at session start.
---

# CCC Team Skill

Comprehensive skill for managing 3-pane Claude Code team sessions and inter-pane communication.

## Architecture

```
Session (tmux session)
  └─ Window (tmux window) - "ccc-team:<session-name>"
       ├─ Pane 1: Planner  (CCC_ROLE=planner)
       ├─ Pane 2: Executor (CCC_ROLE=executor)
       └─ Pane 3: Reviewer (CCC_ROLE=reviewer)
```

**NOTE**: tmux uses 1-based pane indexing. Panes are:
- Pane 1 = Planner (@planner)
- Pane 2 = Executor (@executor)
- Pane 3 = Reviewer (@reviewer)

## Role Responsibilities

| Role | Responsibility | Communication |
|------|---------------|--------------|
| **Planner (Pane 1)** | Task decomposition, delegation | `@executor` for tasks, `@reviewer` for reviews |
| **Executor (Pane 2)** | Code implementation, testing | Reports to `@planner` only |
| **Reviewer (Pane 3)** | Code review, approval | Reports to `@planner` only |

**Communication Rule**: Only the Planner communicates directly with Executor and Reviewer. Executor and Reviewer report back to the Planner, who coordinates next steps.

---

## Creating a Team Session

### Using CCC CLI

```bash
# Create a new team session with 3 panes
ccc team new myproject

# Resume an existing team session
ccc team attach myproject

# List all team sessions
ccc team list

# Start Claude in all panes
ccc team start myproject

# Stop a team session
ccc team stop myproject
```

### Manual Setup

```bash
# Create tmux window with 3 panes (horizontal layout)
tmux new-window -n "ccc-team:myproject"
tmux split-window -h -t "ccc-team:myproject"   # Split for panes 1 and 2
tmux split-window -h -t "ccc-team:myproject.2" # Split for pane 3

# Name each pane by its role
tmux select-pane -t "ccc-team:myproject.1" -T "Planner"
tmux select-pane -t "ccc-team:myproject.2" -T "Executor"
tmux select-pane -t "ccc-team:myproject.3" -T "Reviewer"

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

```bash
# Pane 1 - Planner
cd /path/to/project
CCC_ROLE=planner claude

# Pane 2 - Executor
cd /path/to/project
CCC_ROLE=executor claude

# Pane 3 - Reviewer
cd /path/to/project
CCC_ROLE=reviewer claude
```

---

## Pane Layouts

### Equal 3-Pane Layout

```
┌─────────┬─────────┬─────────┐
│Planner  │Executor │Reviewer │
│  33%    │  33%    │  33%    │
└─────────┴─────────┴─────────┘
```

### Planner-Focused Layout (60/20/20)

```
┌────────────────┬────────┬────────┐
│    Planner     │Executor│Reviewer│
│      60%       │  20%   │  20%   │
└────────────────┴────────┴────────┘
```

```bash
tmux select-layout -t "ccc-team:myproject" main-horizontal
```

---

## Inter-Pane Communication

### Quick Reference

| To Role    | Use @mention | tmux target |
|------------|--------------|-------------|
| @planner   | `@planner`   | :.1         |
| @executor  | `@executor`  | :.2         |
| @reviewer  | `@reviewer`  | :.3         |

### Sending Messages

When you mention another role, prepend the @mention and execute:

```bash
@executor please implement the REST API
```

The skill will detect the @mention and send via tmux.

### Step-by-Step Message Send

```bash
# Step 1: Create message file (safe heredoc — no shell injection)
cat > /tmp/ccc-msg.txt << 'EOF'
your message here
EOF

# Step 2: Load into tmux buffer
tmux load-buffer -b ccc-msg /tmp/ccc-msg.txt

# Step 3: Paste to target pane (double Enter required by Claude Code 2.1.84+)
tmux paste-buffer -d -p -t :.{target_pane} -b ccc-msg
sleep 0.1
tmux send-keys -t :.{target_pane} Escape  # Dismiss autocomplete popup
sleep 0.05
tmux send-keys -t :.{target_pane} Enter   # Submit (first Enter)
sleep 0.05
tmux send-keys -t :.{target_pane} Enter   # Submit (second Enter required)
```

### Pane Index Mapping

| Role      | Pane Index | Buffer Name   |
|-----------|------------|---------------|
| @planner  | :.1        | ccc-interpane-1 |
| @executor | :.2        | ccc-interpane-2 |
| @reviewer | :.3        | ccc-interpane-3 |

---

## Receiving Messages

### MANDATORY: Submit Received Messages Immediately

**CRITICAL**: When a message is sent via tmux paste-buffer, it appears as typed text. Claude Code does NOT automatically process it.

**You MUST run the submit commands immediately when you see new text appear:**

```bash
sleep 0.2
tmux send-keys -t :. Escape  # REQUIRED: Dismiss autocomplete popup
tmux send-keys -t :. Enter   # REQUIRED: First Enter
tmux send-keys -t :. Enter   # REQUIRED: Second Enter
```

---

## ACK Protocol

| Response | Meaning |
|----------|---------|
| `@{sender} ACK` | Message received, processing |
| `@{sender} Done` | Task completed |
| `@{sender} NACK` | Message rejected, cannot process |

### Example Conversation

```
[Planner - Pane 1]:
User: "Please implement the REST API endpoints and get them reviewed"
Planner: Creates task plan, then sends to Executor:
  tmux load-buffer -b ccc-msg /tmp/ccc-msg.txt
  tmux paste-buffer -d -p -t :.2 -b ccc-msg
  tmux send-keys -t :.2 Escape; tmux send-keys -t :.2 Enter; tmux send-keys -t :.2 Enter
  → Message: "@executor Please implement the REST API endpoints"

[Executor - Pane 2]:
(receives message) "Please implement the REST API endpoints"
Claude: "@planner ACK - working on it"
  → tmux sends ACK to pane 1
(implements endpoints, runs tests)
Claude: "@planner Done - REST API implemented and tested"

[Planner - Pane 1]:
(receives done, sends to Reviewer):
  → Message: "@reviewer Please review the REST API implementation"

[Reviewer - Pane 3]:
(receives message) "Please review..."
Claude: "@planner LGTM - approved"

[Planner - Pane 1]:
(receives approval, relays to Executor):
  → Message: "@executor The REST API has been approved by Reviewer"
```

### Communication Rules

| From | To | When |
|------|-----|------|
| Planner | @executor | Delegate tasks |
| Planner | @reviewer | Request reviews |
| Executor | @planner | Report completion or blockers |
| Reviewer | @planner | Report approval or request changes |

---

## Verifying Setup

### Check Pane Status

```bash
# List all panes with titles
tmux list-panes -t "ccc-team:myproject" -F "#{pane_index}: #{pane_title} [#{pane_active}]"

# Example output:
# 1: Planner [1]   <- active
# 2: Executor [0]
# 3: Reviewer [0]
```

### Verify CCC_ROLE

```bash
echo $CCC_ROLE  # Outputs: planner, executor, reviewer, or empty
```

---

## Session Lifecycle

```
Create → Start → Work ←→ Pause ←→ Resume → Close
              ↓
         [3 panes running]
```

- **Pause**: `tmux detach` (Ctrl+b d)
- **Resume**: `tmux attach -t ccc-team:<name>`
- **Close**: `tmux kill-session -t ccc-team:<name>`

---

## Auto-Load Setup

CCC automatically adds a SessionStart hook when you run `ccc install`. This hook detects `CCC_ROLE` at session startup and exports it to `CLAUDE_ENV_FILE` for session persistence.

No manual configuration needed - the hook is managed by CCC.

---

## Troubleshooting

| Issue | Cause | Solution |
|-------|-------|----------|
| Pane title empty | Not set during creation | `tmux select-pane -t :.1 -T "Planner"` |
| CCC_ROLE empty | Not set before claude start | Restart pane with env var |
| Cannot send to pane | Wrong pane index | Check with `tmux list-panes` |
| ACK timeout | No response from target | Retry message or notify user |

---

## Security Notes

- **Current window only**: Messages only sent within current tmux window
- **Heredoc approach**: All messages use `<< 'EOF'` to prevent shell injection
- **No cross-session communication**: Panes in different tmux sessions cannot communicate
- **Role verification**: Always verify target pane index before sending

---

## Verification Commands

```bash
# 1. Verify tmux is available
tmux -V

# 2. Check your current role
echo $CCC_ROLE

# 3. List panes in current window
tmux list-panes -F '#{pane_index}: #{pane_title}'

# 4. Test buffer functionality
cat > /tmp/ccc-test.txt << 'EOF'
test
EOF
tmux load-buffer -b test-buffer /tmp/ccc-test.txt
tmux delete-buffer -b test-buffer 2>/dev/null || true
```
