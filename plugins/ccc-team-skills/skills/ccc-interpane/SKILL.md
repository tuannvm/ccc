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
│ Pane 1   │ Pane 2   │ Pane 3   │
│ Planner  │ Executor │ Reviewer │
└──────────┴──────────┴──────────┘
```

**NOTE**: tmux uses 1-based pane indexing. Panes are:
- Pane 1 = Planner (@planner)
- Pane 2 = Executor (@executor)
- Pane 3 = Reviewer (@reviewer)

## Roles

- **@planner (Pane 1)**: Creates plans, breaks down tasks, delegates work to Executor, requests reviews from Reviewer
- **@executor (Pane 2)**: Executes code changes, runs commands, runs tests, reports back to Planner only
- **@reviewer (Pane 3)**: Reviews code, provides feedback, approves changes, reports back to Planner only

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
| @planner   | `@planner`   | :.1         |
| @executor  | `@executor`  | :.2         |
| @reviewer  | `@reviewer`  | :.3         |

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

**NOTE**: tmux uses 1-based pane indexing. The panes created by CCC are:
- Pane 1 = Planner
- Pane 2 = Executor
- Pane 3 = Reviewer

| Role      | Pane Index | Buffer Name       |
|-----------|------------|------------------|
| @planner  | :.1        | ccc-interpane-1  |
| @executor | :.2        | ccc-interpane-2  |
| @reviewer | :.3        | ccc-interpane-3  |

---

## Handling @Mentions

### MANDATORY: Submit Received Messages Immediately

**CRITICAL**: When a message is sent to your pane via tmux paste-buffer, it appears as typed text in your prompt area. Claude Code does NOT automatically process it — the message just sits there until you press Enter.

**You MUST run the submit commands immediately when you see new text appear.**

After the message appears in your prompt area, **immediately** run:

```bash
sleep 0.2
tmux send-keys -t :. Escape  # REQUIRED: Dismiss autocomplete popup
tmux send-keys -t :. Enter   # REQUIRED: First Enter
tmux send-keys -t :. Enter   # REQUIRED: Second Enter (Claude Code 2.1.84+)
```

**Do NOT wait for the message to disappear or hope it auto-submits. It will NOT.**

### Step-by-Step When You Receive a Message

1. **Immediately after text appears** (do this NOW, don't wait):
   ```bash
   tmux send-keys -t :. Escape; tmux send-keys -t :. Enter; tmux send-keys -t :. Enter
   ```
   Or the full version with dismiss:
   ```bash
   sleep 0.2
   tmux send-keys -t :. Escape
   sleep 0.05
   tmux send-keys -t :. Enter
   sleep 0.05
   tmux send-keys -t :. Enter
   ```

2. **Wait for processing** to complete

3. **Send an ACK** back to the sender:
```bash
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

# Expected output (tmux uses 1-based indexing):
# 1: Planner [1]   <- active (current pane)
# 2: Executor [0]
# 3: Reviewer [0]
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

**NOTE**: Only the Planner communicates directly with Executor and Reviewer. Executor and Reviewer report back to the Planner, who then coordinates next steps.

```text
[Planner - Pane 1]:
User: "Please implement the REST API endpoints and get them reviewed"
Planner: Creates task plan, then sends to Executor:
  tmux load-buffer -b ccc-msg /tmp/ccc-msg.txt
  tmux paste-buffer -d -p -t :.2 -b ccc-msg
  tmux send-keys -t :.2 Escape  # Dismiss autocomplete
  tmux send-keys -t :.2 Enter   # Submit (double Enter required)
  tmux send-keys -t :.2 Enter
  → Message: "@executor Please implement the REST API endpoints"

[Executor - Pane 2]:
(receives message) "Please implement the REST API endpoints"
Claude: "I'll start implementing now..."
Claude: "@planner ACK - working on it"
  → tmux sends ACK to pane 1 (Planner)
(implements endpoints, runs tests)
Claude: "@planner Done - REST API implemented and tested"
  → tmux sends to pane 1 (Planner)

[Planner - Pane 1]:
(receives ACK from Executor)
(after Executor reports done, sends to Reviewer):
  tmux send-keys -t :.3 "Please review the REST API implementation"
  → Message: "@reviewer Please review the REST API implementation"

[Reviewer - Pane 3]:
(receives message) "Please review the REST API implementation"
Claude: "Looking at the code..."
Claude: "@planner LGTM - approved"
  → tmux sends to pane 1 (Planner)

[Planner - Pane 1]:
(receives approval from Reviewer)
Claude: "@executor The REST API has been approved by Reviewer"
  → tmux sends to pane 2 (Executor)
```

### Communication Rules

| From | To | When |
|------|-----|------|
| Planner | @executor | Delegate tasks |
| Planner | @reviewer | Request reviews |
| Executor | @planner | Report completion or blockers |
| Reviewer | @planner | Report approval or request changes |
| Planner | @executor | Relay feedback from Reviewer |

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
# Run the skill validation test from the skills directory
./ccc-interpane/test.sh

# Or from project-local installation
.your-project/.claude/skills/ccc-interpane/test.sh
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
# From any pane, send a test message to pane 2 (Executor)
cat > /tmp/test_msg.txt << 'EOF'
test message from pane 1
EOF
tmux load-buffer -b test-msg /tmp/test_msg.txt
tmux paste-buffer -d -p -t :.2 -b test-msg
sleep 0.1
tmux send-keys -t :.2 Escape  # Dismiss autocomplete popup
sleep 0.05
tmux send-keys -t :.2 Enter   # Submit (first Enter)
sleep 0.05
tmux send-keys -t :.2 Enter   # Submit (second Enter required)
```

Check pane 2 for the message. If visible, inter-pane messaging works.

### Validation Test Sections

The test suite validates:
1. **Skill File Validation** - Proper frontmatter, name, description
2. **tmux Command Validation** - tmux is installed and responsive
3. **Pane Index Mapping** - Correct mapping (1=Planner, 2=Executor, 3=Reviewer)
4. **CCC_ROLE Bootstrap** - Environment variable detection documented
5. **Security Validation** - Heredoc safety, current window constraint
6. **ACK Protocol** - ACK/Done responses documented
7. **Integration Test** - Live tmux buffer operations
8. **Related Skills** - ccc-team-session and tmux-intercom present
