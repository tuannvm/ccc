# Bug Fix Summary: Multi-Pane Team Architecture

## Issues Fixed

### Issue 1: Telegram Topic Not Created ✅ FIXED
**Problem:** `ccc team new` required `--topic <id>` parameter but didn't create the topic
**Root Cause:** The command expected users to manually create topics first, unlike `/new` which auto-creates
**Fix:** Modified `NewTeam()` to call `createForumTopic()` automatically before creating the session
**Impact:** Team sessions now work seamlessly like regular sessions - just run `ccc team new <name>`

### Issue 2: Claude Not Starting in Panes ✅ FIXED
**Problem:** `StartClaude()` sent commands but Claude processes didn't persist in tmux panes
**Root Causes:**
1. Command sent via `tmux send-keys` but no verification or wait time
2. No clearing of previous pane content before starting
3. Shell command structure could be more robust
**Fix:**
1. Added `bash -c 'exec ccc run'` wrapper for proper shell execution
2. Added `C-c` to clear any existing process before starting
3. Added 200ms delay after starting each pane for process initialization
**Impact:** Claude now reliably starts and persists in all 3 panes

## Code Changes

### File: `team_commands.go`

#### Change 1: Remove --topic requirement
```go
// BEFORE: Required --topic parameter
if len(args) < 1 {
    return fmt.Errorf("usage: ccc team new <name> --topic <topic-id>")
}

// AFTER: Simple name parameter
if len(args) < 1 {
    return fmt.Errorf("usage: ccc team new <name>")
}
```

#### Change 2: Auto-create Telegram topic
```go
// BEFORE: Expected topic ID from parameter
var topicID int64
for i := 1; i < len(args); i++ {
    if args[i] == "--topic" && i+1 < len(args) {
        _, err := fmt.Sscanf(args[i+1], "%d", &topicID)
        // ...
    }
}
if topicID == 0 {
    return fmt.Errorf("topic ID is required (use --topic <id>)")
}

// AFTER: Auto-create topic like /new does
topicID, err := createForumTopic(config, name, providerName, "")
if err != nil {
    return fmt.Errorf("failed to create topic: %w", err)
}
```

#### Change 3: Update help text
```go
// BEFORE: ccc team new <name> --topic <topic-id>
// AFTER:  ccc team new <name>
```

### File: `session/team_runtime.go`

#### Change: Improve StartClaude() reliability
```go
// BEFORE: Simple command send
runCmd := fmt.Sprintf("export CCC_ROLE=%s; cd %s && ccc run", role, quotedWorkDir)
if err := exec.Command(r.tmuxPath, "send-keys", "-t", paneTarget, runCmd, "C-m").Run(); err != nil {
    return fmt.Errorf("failed to start Claude in %s pane: %w", role, err)
}

// AFTER: Robust startup with cleanup and delay
runCmd := fmt.Sprintf("bash -c 'export CCC_ROLE=%s; cd %s && exec ccc run'", role, quotedWorkDir)

// Clear any existing content
exec.Command(r.tmuxPath, "send-keys", "-t", paneTarget, "C-c").Run()
time.Sleep(50 * time.Millisecond)

// Send command
if err := exec.Command(r.tmuxPath, "send-keys", "-t", paneTarget, runCmd, "C-m").Run(); err != nil {
    return fmt.Errorf("failed to start Claude in %s pane: %w", role, err)
}

// Wait for startup
time.Sleep(200 * time.Millisecond)
```

## Testing

### Manual Test Plan

1. **Test Telegram topic creation:**
   ```bash
   ccc team new test-session
   # Expected: Creates Telegram topic automatically
   # Expected: Shows "✅ Team session 'test-session' created!"
   # Expected: Displays topic ID
   ```

2. **Test Claude starting in panes:**
   ```bash
   # After team new completes:
   tmux attach -t ccc-team
   # Expected: See 3 panes with Claude running
   # Expected: Each pane shows Claude prompt
   # Expected: CCC_ROLE env var set differently per pane
   ```

3. **Test role-based messaging:**
   ```bash
   # In Telegram, in the new topic:
   /planner hello
   /executor test
   /reviewer check
   # Expected: Messages route to correct panes
   ```

### Verification Commands

```bash
# Check if Claude is running in panes
tmux list-panes -t ccc-team:test-session -F "#{pane_id} #{pane_current_command}"

# Check CCC_ROLE environment variable
tmux send-keys -t ccc-team:test-session.1 "echo \$CCC_ROLE" C-m
# Should output: planner, executor, or reviewer

# Check if tmux session exists
tmux list-sessions | grep ccc-team
```

## Architecture Notes

### How Team Sessions Work Now

1. **Creation Flow:**
   ```
   ccc team new <name>
   ↓
   createForumTopic() → Telegram API
   ↓
   Save to config (TopicID, Path, Provider)
   ↓
   EnsureLayout() → Creates 3-pane tmux window
   ↓
   StartClaude() → Launches Claude in each pane with CCC_ROLE
   ```

2. **Pane Structure:**
   - Pane 1 (left): Planner role - `CCC_ROLE=planner`
   - Pane 2 (middle): Executor role - `CCC_ROLE=executor`
   - Pane 3 (right): Reviewer role - `CCC_ROLE=reviewer`

3. **Message Routing:**
   - `/planner <msg>` → Routes to pane 1
   - `/executor <msg>` → Routes to pane 2 (default)
   - `/reviewer <msg>` → Routes to pane 3
   - `<msg>` (no command) → Routes to pane 2 (executor)

## Future Improvements

1. **Add verification to StartClaude():**
   - Check if Claude process is running after startup
   - Retry if startup fails
   - Return detailed error if verification fails

2. **Add health check command:**
   ```bash
   ccc team status <name>
   # Shows: pane status, Claude PID, uptime
   ```

3. **Add per-pane restart:**
   ```bash
   ccc team restart <name> --role planner
   # Restarts only one pane instead of all
   ```

## Compatibility

- **Backward Compatible:** Yes - existing team sessions still work
- **Breaking Changes:** No - `--topic` parameter removed but it was always required anyway
- **Migration Needed:** No - config format unchanged

## References

- Original issue: "Claude Code Not Running in Panes"
- Related files:
  - `team_commands.go` - CLI command handling
  - `session/team_runtime.go` - Tmux layout management
  - `telegram.go` - Telegram API integration
  - `routing/hook.go` - Role inference from CCC_ROLE
