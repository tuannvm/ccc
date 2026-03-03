# CCC Fixes

This document documents significant fixes and improvements to ccc.

## Problem: "Session created but died immediately"

When sending `/new projectname` in Telegram, the bot responded with "Session created but died immediately".

---

## Root Causes Found

### 1. Wrong Tmux Socket Path (Linux vs macOS)

**File:** `main.go`, function `init()`

**Problem:**
```go
tmuxSocket = fmt.Sprintf("/private/tmp/tmux-%d/default", os.Getuid())
```
This path (`/private/tmp/...`) only exists on macOS. On Linux, tmux uses `/tmp/tmux-UID/default`.

**Fix:**
```go
uid := os.Getuid()
macOSSocket := fmt.Sprintf("/private/tmp/tmux-%d/default", uid)
linuxSocket := fmt.Sprintf("/tmp/tmux-%d/default", uid)

// Check which socket exists, prefer Linux path first
if _, err := os.Stat(linuxSocket); err == nil {
    tmuxSocket = linuxSocket
} else if _, err := os.Stat(macOSSocket); err == nil {
    tmuxSocket = macOSSocket
} else {
    // Default based on OS detection
    if _, err := os.Stat("/private"); err == nil {
        tmuxSocket = macOSSocket
    } else {
        tmuxSocket = linuxSocket
    }
}
```

---

### 2. Claude Binary Not Found (nvm/npm installations)

**File:** `main.go`, function `init()`

**Problem:**
```go
claudePaths := []string{
    filepath.Join(home, ".claude", "local", "claude"),
    "/usr/local/bin/claude",
}
```
When Claude is installed via npm with nvm, it's located at:
`~/.nvm/versions/node/vXX.XX.X/bin/claude`

This path wasn't checked.

**Fix:**
```go
// Find claude binary - first try PATH, then fallback paths
if path, err := exec.LookPath("claude"); err == nil {
    claudePath = path
} else {
    // Fallback to known paths
    home, _ := os.UserHomeDir()
    claudePaths := []string{
        filepath.Join(home, ".claude", "local", "claude"),
        "/usr/local/bin/claude",
    }
    for _, p := range claudePaths {
        if _, err := os.Stat(p); err == nil {
            claudePath = p
            break
        }
    }
}
```

---

### 3. Tmux Session Dies Immediately (command as session argument)

**File:** `main.go`, function `createTmuxSession()`

**Problem:**
```go
args := []string{"-S", tmuxSocket, "new-session", "-d", "-s", name, "-c", workDir,
    "/bin/zsh", "-l", "-c", cccCmd}
```

When you pass a command as the session's main process:
1. Tmux creates session with that command as PID 1
2. When command exits (for any reason), session terminates
3. Claude Code requires proper TTY - running as subprocess of `zsh -c` doesn't provide it correctly

**Fix:**
```go
// Create tmux session with default shell (don't run command directly)
args := []string{"-S", tmuxSocket, "new-session", "-d", "-s", name, "-c", workDir}
cmd := exec.Command(tmuxPath, args...)
if err := cmd.Run(); err != nil {
    return err
}

// Enable mouse mode
exec.Command(tmuxPath, "-S", tmuxSocket, "set-option", "-t", name, "mouse", "on").Run()

// Send command via send-keys (preserves TTY properly)
time.Sleep(200 * time.Millisecond)
exec.Command(tmuxPath, "-S", tmuxSocket, "send-keys", "-t", name, cccCmd, "C-m").Run()
```

**Why this works:**
- Session starts with interactive shell (has proper TTY)
- Command is sent as user input via `send-keys`
- If Claude exits, shell remains → session stays alive
- Proper TTY prevents Claude from switching to `--print` mode

---

## Additional Notes

### Claude Code Interactive Prompts

Claude Code may show interactive prompts:
1. "Do you trust the files in this folder?" - handled by `trustedDirectories` in `~/.claude/settings.json`
2. Bypass permissions acceptance - handled by `--dangerously-skip-permissions` flag

Current `~/.claude/settings.json` already has:
```json
{
  "trustedDirectories": [
    "/home/wlad",
    "/home/wlad/Projects",
    ...
  ],
  "autoApprove": {
    "trustDirectories": true
  }
}
```

This prevents the trust prompt for subdirectories of `/home/wlad`.

### Claude Requires TTY

When Claude Code detects no TTY (e.g., running in pipe or background), it automatically switches to `--print` mode which requires stdin input:
```
Error: Input must be provided either through stdin or as a prompt argument when using --print
```

This is why using `send-keys` instead of running command directly is essential.

---

---

### 4. Project Directory Not Created (hooks can't find session)

**File:** `main.go`, multiple locations

**Problem:**
```go
workDir := filepath.Join(home, arg)
if _, err := os.Stat(workDir); os.IsNotExist(err) {
    workDir = home  // Fallback to home - BAD!
}
```

When `/new projectname` is called:
1. Code checks if `~/projectname` exists
2. If not, it falls back to `~` (home directory)
3. Claude runs in `~` instead of `~/projectname`
4. Hook receives `cwd=/home/wlad`
5. Hook tries to match session by path suffix (`/projectname`)
6. No match found → message not sent to Telegram

**Locations fixed:**
- Line ~539: `createSession()` function
- Line ~1751: `/new <name>` handler in `listen()`
- Line ~1786: `/new` (restart) handler in `listen()`

**Fix:**
```go
workDir := filepath.Join(home, arg)
if _, err := os.Stat(workDir); os.IsNotExist(err) {
    os.MkdirAll(workDir, 0755)  // Create directory instead of fallback
}
```

**Result:**
- `/new test8` creates `~/test8/` directory
- tmux session runs in `~/test8/`
- Claude's cwd is `/home/wlad/test8`
- Hook matches session by path → sends to correct Telegram topic

---

## Testing

After fixes:
```bash
# Verify all checks pass
ccc doctor

# Test session creation manually
tmux new-session -d -s test-session -c /home/wlad/testdir
tmux send-keys -t test-session "/home/wlad/bin/ccc run" C-m
sleep 3
tmux has-session -t test-session && echo "SUCCESS" || echo "FAILED"
tmux capture-pane -t test-session -p

# Test via Telegram
# Send: /new testproject
# Expected: "Session 'testproject' started!" message in topic
```

---

## Problem: "Every prompt causes session restart" - March 3, 2026

When sending prompts rapidly to a session, every prompt caused the session to restart even though Claude was already running.

### Root Cause

When `skipRestart=true` was passed to `switchSessionInWindow()`, the function would still send restart commands or respawn the pane if:

1. `tmuxWindowHasClaudeRunning()` had a false negative (Claude running but not detected)
2. A shell was detected in the pane (which could be the parent of Claude)
3. The pane was in an unknown state (Claude might be starting up)

### Fix (PR #2)

**File:** `tmux.go`, function `switchSessionInWindow()`

Added more conservative behavior when `skipRestart=true`:

1. **Fallback detection**: When primary detection fails, check for Claude prompt in pane content
2. **Shell handling**: Don't send restart command when shell detected with `skipRestart=true`
3. **Unknown state**: Don't respawn when pane is in unknown state with `skipRestart=true`

```go
// When skipRestart=true but we don't detect Claude or shell, be extra cautious
// This handles false negatives in detection where Claude is actually running
if !tmuxWindowHasClaudeRunning(target, "") && !tmuxWindowHasShellRunning(target, "") {
    // Check for Claude prompt in the pane content as a fallback
    if tmuxPaneHasActiveClaudePrompt(target) {
        listenLog("skipRestart=true: Claude prompt detected in pane content (fallback detection)")
        shouldRestart = false
    }
}

// When skipRestart=true and shell is detected
if skipRestart {
    listenLog("Shell detected with skipRestart=true - not sending restart command to preserve session state")
} else {
    // Send the command to start Claude
    fullCmd := "cd " + shellQuote(workDir) + " && " + runCmd
    if err := exec.Command(tmuxPath, "send-keys", "-t", target, fullCmd, "C-m").Run(); err != nil {
        return fmt.Errorf("failed to send command: %w", err)
    }
}
```

### Impact

- No more unnecessary session restarts when prompts arrive rapidly
- Direct prompt delivery to existing Claude sessions
- Better handling of concurrent/rapid prompts
- Preserved session state during transient states

---

## Feature: Per-Project Hooks - February 28, 2026

Previously, hooks were installed globally in `~/.claude/hooks/`, which affected all Claude Code sessions system-wide.

### Enhancement (PR #1)

Added per-project hook installation:

1. **`ccc install-hooks`** - Install hooks in current project
2. **`ccc cleanup-hooks`** - Remove hooks from current project
3. **Auto-install** - Hooks are automatically installed when creating sessions

**Hook location:**
- Old: `~/.claude/hooks/*` (global)
- New: `<project>/.claude/hooks/*` (per-project)

**Benefits:**
- Project-specific hook configurations
- No interference between projects
- Cleaner separation of concerns
- Easier hook management

**Implementation:**

Hooks are installed in the project's `.claude/hooks/` directory:
```bash
myproject/.claude/hooks/
├── pre-run
├── post-run
└── ask
```

Each hook checks if it's in the correct project directory before acting.

---

## Additional Improvements (2026)

### Provider Abstraction

Refactored provider configuration to support multiple AI providers:

- **Provider interface**: `Provider` interface for provider-agnostic design
- **Multiple providers**: Configure multiple providers in `config.json`
- **Active provider**: Set default provider globally
- **Per-session provider**: Override provider for specific sessions

```json
{
  "active_provider": "example-provider",
  "providers": {
    "example-provider": { "auth_env_var": "EXAMPLE_API_KEY" },
    "alternative-provider": { "base_url": "https://api.example.com/v1", "auth_env_var": "ALT_API_KEY" }
  }
}
```

### Atomic Config Writes

Configuration writes now use atomic operations to prevent corruption:

1. Write to temp file (`config-*.json.tmp`)
2. Rename to `config.json`

This prevents data corruption when multiple processes write simultaneously.

### Improved Claude Detection

Enhanced detection of running Claude Code processes:

- **npm Claude**: Detects npm-installed Claude via `claude/cli`
- **Child process detection**: Checks if shell has Claude as child
- **Prompt detection**: Looks for Claude's prompt character (❯) in pane
- **Multiple methods**: Falls back through detection methods

---
