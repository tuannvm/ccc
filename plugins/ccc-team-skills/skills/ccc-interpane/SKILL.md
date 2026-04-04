---
name: ccc-interpane
description: CCC Inter-Pane Communication for 3-pane tmux team sessions (planner/executor/reviewer). Auto-loads when CCC_ROLE is set at session start. Use when sending messages between panes via @mentions, or when working as Planner, Executor, or Reviewer.
---

# CCC Inter-Pane Communication Skill

Enables communication between Claude Code instances running in different panes of the same tmux window. This skill **auto-loads when CCC_ROLE is set** (via SessionStart hook), and also triggers when you mention @planner, @executor, or @reviewer.

## Pane Layout

You are in a 3-pane tmux window:

```text
┌──────────┬──────────┬──────────┐
│ Pane 0   │ Pane 1   │ Pane 2   │
│ Planner  │ Executor │ Reviewer │
└──────────┴──────────┴──────────┘
```

## Roles

- **Pane 0 - @planner**: Creates plans, breaks down tasks, delegates work
- **Pane 1 - @executor**: Executes code changes, runs commands, runs tests
- **Pane 2 - @reviewer**: Reviews code, provides feedback, approves changes

---

## Auto-Bootstrap Mechanism

### How Your Role Is Detected

When Claude Code starts in a team pane, the `CCC_ROLE` environment variable is set to identify your role:

```bash
# Check your current role
echo $CCC_ROLE  # Outputs: planner, executor, reviewer, or empty

# Check tmux pane title
tmux display-message -p '#{pane_title}'  # Outputs: Planner, Executor, Reviewer, or empty
```

### Session Initialization

This skill triggers when:
1. `CCC_ROLE` environment variable is set (planner/executor/reviewer)
2. Tmux pane title matches a known role name
3. You mention @planner, @executor, or @reviewer in your message

---

## Sending Messages Between Panes

### Quick Reference

| To Role    | Use @mention | tmux target |
|------------|--------------|-------------|
| @planner   | `@planner`   | :.0         |
| @executor  | `@executor`  | :.1         |
| @reviewer  | `@reviewer`  | :.2         |

### Message Format

When you mention another role, prepend the message with the @mention and execute the tmux send commands:

```text
@executor please implement the REST API
```

The skill will automatically detect the @mention and prompt you to send via tmux.

```bash
# Step 1: Verify your current pane and role
tmux display-message -p "Pane #{pane_index}: #{pane_title}"

# Step 2: Create message file (safe heredoc — no shell injection)
cat > /tmp/ccc-interpane-msg.txt << 'EOF'
{your message here}
EOF

# Step 3: Load into tmux buffer
tmux load-buffer -b ccc-interpane-out /tmp/ccc-interpane-msg.txt

# Step 4: Paste to target pane and send Enter (double Enter required by Claude Code 2.1.84+)
tmux paste-buffer -d -p -t :.{target_pane} -b ccc-interpane-out
sleep 0.1
tmux send-keys -t :.{target_pane} Escape  # Dismiss autocomplete popup
sleep 0.05
tmux send-keys -t :.{target_pane} Enter   # Submit (first Enter)
sleep 0.05
tmux send-keys -t :.{target_pane} Enter   # Submit (second Enter required)
```

### Pane Index Mapping

| Role      | Pane Index | Buffer Name       |
|-----------|------------|------------------|
| @planner  | :.0        | ccc-interpane-0  |
| @executor | :.1        | ccc-interpane-1  |
| @reviewer | :.2        | ccc-interpane-2  |

---

## Handling @Mentions

### When You Receive an @mention

When another pane sends you a message, it appears as user input in your pane. Process it naturally, then send an ACK:

```bash
# After processing, send acknowledgment
@planner ACK - message received and processing
```

### When You Send an @mention

1. Send the message using tmux commands (above)
2. Wait for ACK response (timeout: 60 seconds)
3. If no ACK, retry once after 10 seconds

### ACK Protocol

| Response | Meaning |
|----------|---------|
| `@{sender} ACK` | Message received, processing |
| `@{sender} Done` | Task completed |
| `@{sender} NACK` | Message rejected, cannot process |

---

## State Coordination

### Message Tracking

For deduplication and loop prevention, the interpane package tracks:
- **Request ID**: Unique ID for each user prompt (for deduplication)
- **Hop Count**: Prevents infinite routing loops (max 5 hops)
- **Message Hash**: Detects duplicate messages

### Busy Pane Handling

If target pane is busy (processing), messages are queued:
- Max 25 messages per role queue
- Max 75 total messages globally
- Queue persisted to `~/.config/ccc/sessions/<name>/interpane/`

---

## Utility Commands

### Check Pane Status

```bash
# List all panes with their titles and indices
tmux list-panes -F "#{pane_index}: #{pane_title} [#{pane_active}]"

# Expected output:
# 0: Planner [1]   <- active (current pane)
# 1: Executor [0]
# 2: Reviewer [0]
```

### Check Your Role

```bash
# Via environment variable
echo $CCC_ROLE  # Outputs: planner, executor, reviewer, or empty

# Via tmux pane title
tmux display-message -p '#{pane_title}'  # Outputs: Planner, Executor, Reviewer, or empty
```

### Send a Message (Direct tmux)

```bash
# Create message file (always use heredoc with 'EOF' to prevent injection)
cat > /tmp/ccc-msg.txt << 'EOF'
your message here
EOF

# Load into tmux buffer
tmux load-buffer -b ccc-msg /tmp/ccc-msg.txt

# Paste to target pane (use pane index from table above)
tmux paste-buffer -d -p -t :.{pane_index} -b ccc-msg
sleep 0.1
tmux send-keys -t :.{pane_index} Escape  # Dismiss autocomplete popup
sleep 0.05
tmux send-keys -t :.{pane_index} Enter   # Submit (first Enter)
sleep 0.05
tmux send-keys -t :.{pane_index} Enter   # Submit (second Enter required)
```

---

## Error Handling

| Error | Cause | Resolution |
|-------|-------|------------|
| "pane not found" | Target pane doesn't exist | Check `tmux list-panes` |
| "load-buffer failed" | tmux buffer error | Retry with different buffer name |
| "ACK timeout" | No response from target | Retry message or notify user |
| "no such session" | tmux session doesn't exist | Check `tmux list-sessions` |

---

## Security Notes

- **Current window only**: Messages only sent within current tmux window
- **Heredoc approach**: All messages use `<< 'EOF'` to prevent shell injection
- **No cross-session communication**: Panes in different tmux sessions cannot communicate
- **Role verification**: Always verify target pane index before sending

---

## Example Conversation

```text
[Planner - Pane 0]:
User: "@executor please implement the REST API endpoints"
Skill: Detected @executor mention. Execute:
  tmux load-buffer -b ccc-msg /tmp/ccc-msg.txt
  tmux paste-buffer -d -p -t :.1 -b ccc-msg
  tmux send-keys -t :.1 Escape  # Dismiss autocomplete
  tmux send-keys -t :.1 Enter  # Submit (double Enter required)
  tmux send-keys -t :.1 Enter

[Executor - Pane 1]:
(receives message) "please implement the REST API endpoints"
Claude: "I'll start implementing now..."
Claude: "@planner ACK - working on it"
  → tmux sends ACK to pane 0
(implements endpoints, runs tests)
Claude: "@reviewer please review my changes"
  → tmux sends to pane 2

[Reviewer - Pane 2]:
(receives message) "please review my changes"
Claude: "Looking at the code..."
Claude: "@executor Done - approved"
  → tmux sends ACK to pane 1
```

---

## Integration with CCC Hooks

The interpane skill works with CCC's hook system:

1. **Stop hook**: Delivers responses to Telegram with role prefix
2. **UserPrompt hook**: Forwards Telegram messages to Claude panes
3. **PreToolUse hook**: Routes permission requests to appropriate pane

Role prefix format: `[Planner]`, `[Executor]`, `[Reviewer]`

---

## Verification & Testing

### Run Validation Suite

To verify the skill is properly configured:

```bash
# Run the skill validation test
~/.claude/skills/ccc-interpane/test.sh

# Or from the project directory (if skill is in .claude/skills/)
.claude/skills/ccc-interpane/test.sh
```

Expected output: `✅ ALL TESTS PASSED` with 20+ tests.

### Quick Verification Commands

Run these commands to verify your setup:

```bash
# 1. Verify tmux is available
tmux -V

# 2. Check your current role
echo $CCC_ROLE

# 3. Check tmux pane title
tmux display-message -p '#{pane_title}'

# 4. List panes in current window
tmux list-panes

# 5. Test buffer functionality (using heredoc for safety)
cat > /tmp/ccc-test.txt << 'EOF'
test
EOF
tmux load-buffer -b test-buffer /tmp/ccc-test.txt
tmux delete-buffer -b test-buffer 2>/dev/null || true
```

### Team Session Verification

To verify a 3-pane team session is ready:

```bash
# Should show 3 panes
tmux list-panes | wc -l  # Expected: 3

# Should show pane titles
tmux list-panes -F '#{pane_index}: #{pane_title}'
# Expected:
# 0: Planner
# 1: Executor
# 2: Reviewer
```

### Test Inter-Pane Messaging

To test sending a message between panes:

```bash
# From any pane, send a test message to pane 1 (Executor)
cat > /tmp/test_msg.txt << 'EOF'
test message from pane 0
EOF
tmux load-buffer -b test-msg /tmp/test_msg.txt
tmux paste-buffer -d -p -t :.1 -b test-msg
sleep 0.1
tmux send-keys -t :.1 Escape  # Dismiss autocomplete popup
sleep 0.05
tmux send-keys -t :.1 Enter   # Submit (first Enter)
sleep 0.05
tmux send-keys -t :.1 Enter   # Submit (second Enter required)
```

Check pane 1 for the message. If visible, inter-pane messaging works.

### Validation Test Sections

The test suite validates:
1. **Skill File Validation** - Proper frontmatter, name, description
2. **tmux Command Validation** - tmux is installed and responsive
3. **Pane Index Mapping** - Correct mapping (0=Planner, 1=Executor, 2=Reviewer)
4. **CCC_ROLE Bootstrap** - Environment variable detection documented
5. **Security Validation** - Heredoc safety, current window constraint
6. **ACK Protocol** - ACK/Done responses documented
7. **Integration Test** - Live tmux buffer operations
8. **Related Skills** - ccc-team-session and tmux-intercom present
