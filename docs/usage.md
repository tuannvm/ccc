# Usage Guide

This document provides comprehensive usage instructions for ccc (Claude Code Companion).

## Quick Reference

### Terminal Commands

| Command | Description |
|---------|-------------|
| `ccc` | Start/attach to session in current directory |
| `ccc -c` | Continue previous session |
| `ccc "message"` | Send notification (if away mode enabled) |
| `ccc send <file>` | Send file to Telegram |
| `ccc start <name> <dir> <prompt>` | Start detached session with initial prompt |
| `ccc doctor` | Check dependencies and configuration |
| `ccc config [key] [value]` | View/set configuration |
| `ccc providers` | List available providers |
| `ccc listen` | Start Telegram listener (service mode) |
| `ccc install-hooks` | Install hooks in current project |
| `ccc cleanup-hooks` | Remove hooks from current project |
| `ccc run` | Run Claude directly (used by tmux) |

### Telegram Commands (Group)

| Command | Description |
|---------|-------------|
| `/new <name>` | Create new session + topic |
| `/new <name>@provider` | Create session with specific provider |
| `/new ~/path/to/dir` | Create session with custom path |
| `/new` | Restart session in current topic |
| `/worktree <base> <name>` | Create worktree session |
| `/continue` | Restart keeping conversation history |
| `/providers` | List available providers |
| `/provider [name]` | Show/change provider for current session |
| `/split <name>` | Create new pane in current session |
| `/pane <ref> <prompt>` | Send prompt to specific pane |
| `/panes` | List all panes with status |
| `/remove-pane <ref>` | Remove a pane |
| `/switch-pane <ref>` | Change active pane |
| `/c <command>` | Execute shell command |
| `/update` | Update ccc binary |
| `/restart` | Restart ccc service |
| `/stats` | Show system statistics |
| `/auth` | Re-authenticate Claude Code |

## Getting Started

### 1. Initial Setup

```bash
# Create Telegram bot via @BotFather
# Save the bot token

# Run setup
ccc setup YOUR_BOT_TOKEN

# Follow prompts to authenticate with Telegram
```

### 2. Create Your First Session

From Telegram (in your group):

```
/new myproject
```

This creates:
- A new Telegram topic named "myproject"
- A project directory at `~/myproject` (or `~/Projects/myproject` if configured)
- A tmux window with Claude Code running

### 3. Send Your First Prompt

In the "myproject" topic:

```
Fix the authentication bug in login.ts
```

Claude will process your request and respond in the same topic.

### 4. Attach from Terminal

```bash
cd ~/myproject
ccc
```

You're now attached to the same session and can continue working.

## Session Management

### Creating Sessions

**Basic session:**
```
/new myproject
```

**Session with specific provider:**
```
/new myproject@provider-name
```
Replace `provider-name` with your configured provider.

**Session with custom path:**
```
/new ~/experiments/test
```

**Session with absolute path:**
```
/new /tmp/quicktest
```

### Switching Between Sessions

**From Telegram:**
- Simply switch to a different topic
- Each topic = one session

**From Terminal:**
```bash
cd ~/otherproject
ccc
```

### Restarting Sessions

**Restart in current topic (preserves history):**
```
/new
```

**Restart keeping conversation:**
```
/continue
```

### Deleting Sessions

From terminal:
```bash
# Delete session (kills tmux window, removes topic mapping)
ccc delete myproject
```

## Provider Management

### Viewing Providers

```
/providers
```

Response:
```
Available providers:
• default (builtin)
• provider-name-1
• provider-name-2

Active: provider-name-1
```

Use inline buttons or provider names to switch between configured providers.

### Changing Provider for Current Session

```
/provider
```

This shows inline buttons for quick provider selection. Choose a provider from the list to switch.

### Creating a Session with Specific Provider

```
/new myproject@provider-name
```

Replace `provider-name` with your configured provider. Use the `/providers` command to see available providers.

### Setting Default Provider

```bash
ccc config providers --set-active provider-name
```

Or edit `~/.config/ccc/config.json`:
```json
{
  "active_provider": "provider-name"
}
```

## Multi-Pane Sessions

Multi-pane sessions allow you to run multiple Claude instances in parallel within the same project window. Useful for:
- **Coder + Reviewer**: One pane implements, another reviews
- **Multiple Providers**: Compare responses from different AI models
- **Parallel Tasks**: Run tests in one pane while writing code in another

### Creating Panes

**Create a new pane (horizontal split by default):**
```
/split reviewer
```

**Create with specific direction:**
```
/split coder --vertical
/split reviewer --horizontal
```

This creates a new pane, starts Claude in it, and sets it as the active pane.

### Listing Panes

```
/panes
```

Response:
```
myproject panes:
* 0: coder (Opus 4.6)
  1: reviewer (Haiku 4.5)
```
The `*` marks the active pane (default message target).

### Sending to Specific Panes

**Send prompt to a specific pane:**
```
/pane coder "Implement the auth feature"
/pane reviewer "Review the changes in auth.go"
```

You can reference panes by:
- Index: `0`, `1`, `2`
- Friendly name: `coder`, `reviewer`

### Switching Active Pane

Change which pane receives default messages:

```
/switch-pane reviewer
```

Or use index:
```
/switch-pane 1
```

Now regular messages (without `/pane`) go to the reviewer pane.

### Removing Panes

```
/remove-pane reviewer
```

Or by index:
```
/remove-pane 1
```

The last pane cannot be removed (protection enforced).

### Pane State Synchronization

Panes are automatically synchronized with tmux state:
- On startup, `initSessionPanes()` syncs config with live tmux panes
- Manually split panes in tmux are detected on next access
- Killed panes are removed from config on next access

### Multi-Pane Workflow Example

```
📱 Telegram: /new myproject
💻 Creates session with pane 0

📱 Telegram: /split reviewer
💻 Creates pane 1, sets as active

📱 Telegram: /pane 0 "Add user authentication"
💻 Routes to coder pane (pane 0)

📱 Telegram: /switch-pane reviewer
💻 Changes active to pane 1

📱 Telegram: "Check the auth implementation"
💻 Routes to reviewer pane (active)

📱 Telegram: /panes
💻 Shows: *0: coder, 1: reviewer
```

## Worktree Sessions

Worktree sessions allow you to work on git branches in separate tmux windows.

### Creating a Worktree Session

```
/worktree myproject feature-auth
```

This:
1. Creates a git worktree for branch `feature-auth`
2. Creates a new session `myproject-feature-auth`
3. Uses the base session's configuration

**From terminal:**
```bash
cd ~/myproject
ccc worktree feature-auth
```

### Worktree Session Behavior

- Inherits provider and configuration from base session
- Has its own Claude Code session and conversation history
- Can be attached independently: `ccc attach myproject-feature-auth`

## Advanced Usage

### Detached Sessions

Start a session with an initial prompt:

```bash
ccc start myproject ~/Projects/myproject "Review the PR and summarize changes"
```

The session runs in the background and you'll receive the response in Telegram.

### Away Mode

Enable away mode to receive notifications when Claude completes tasks:

```bash
ccc config away true
```

Now when you're away and Claude finishes, you'll get a Telegram notification.

### File Transfer

Send files from your computer to your phone:

```bash
# Small files (< 50MB) - direct upload
ccc send ./build/app.apk

# Large files (≥ 50MB) - streaming relay
ccc send ./build/large-file.zip
```

**How it works:**

| File Size | Method |
|-----------|--------|
| < 50 MB | Direct Telegram upload |
| ≥ 50 MB | Streaming download link |

For large files, you'll receive a download link that streams directly from your machine.

### Voice Messages

1. Record a voice message in Telegram
2. Bot automatically transcribes it
3. Transcribed text is sent to Claude

**Transcription backends:**

Set via `transcription_cmd` in config:
- **Local Whisper**: Run locally using your machine
- **API Services**: Fast cloud transcription (choose your preferred service)

### Image Support

Send an image in a session topic:
1. Attach image to message
2. Optionally add a caption
3. Image is saved and path sent to Claude

## Shell Commands

Execute shell commands from Telegram:

```
/c ls -la
```

Response:
```
total 24
drwxr-xr-x  5 user  staff  160 Mar  3 10:00 .
drwxr-xr-x  3 user  staff   96 Mar  3 09:00 ..
-rw-r--r--  1 user  staff  123 Mar  3 10:00 main.go
```

**Common use cases:**
- Check file status: `/c git status`
- View logs: `/c tail -f logs/app.log`
- Run tests: `/c npm test`

## Permission Approval

### Auto-approve Mode (Default)

All tool permissions are automatically approved. No interaction needed.

### OTP Mode

Requires TOTP code approval for remote prompts:

**Enable OTP mode:**
```bash
ccc config otp enable
```

**When a permission is needed:**

You'll receive a message like:
```
🔐 Permission Required

Tool: Bash
Command: rm -rf node_modules

Reply with your 6-digit OTP code to approve.
Timeout: 5 minutes
```

Reply with your code (e.g., `123456`) to approve.

## Troubleshooting Commands

### Check Dependencies

```bash
ccc doctor
```

This checks:
- tmux installation
- Claude Code installation
- Configuration file
- Hook installation
- Service status

### View Logs

**macOS:**
```bash
tail -f ~/Library/Caches/ccc/ccc.log
tail -f ~/Library/Caches/ccc/hook-debug.log
```

**Linux:**
```bash
journalctl --user -u ccc -f
tail -f ~/.cache/ccc/hook-debug.log
```

### Service Management

**Check status:**
```bash
systemctl --user status ccc
```

**Restart service:**
```bash
systemctl --user restart ccc
```

**View logs:**
```bash
journalctl --user -u ccc -n 50
```

## Session Workflows

### Workflow 1: Start on Phone, Continue on PC

```
📱 Phone                    💻 PC
────                        ────
/new myproject
"Fix the bug"

                     cd ~/myproject
                     ccc
                     → Continue working
```

### Workflow 2: Start on PC, Monitor on Phone

```
💻 PC                      📱 Phone
────                        ────
cd ~/myproject
ccc
"Deploy to staging"

                           [Receive notification]
                           [Check deployment status]
```

### Workflow 3: Long-Running Task

```
📱 Phone
────
/new myproject
"Run full test suite and report results"

[Phone in pocket]
...
[Notification: Tests complete]
"3 failures, 47 passed"
```

## Tips and Best Practices

### Session Organization

- Use descriptive session names: `frontend`, `backend-auth`, `ml-experiment`
- Group related projects in `~/Projects/`
- Archive completed topics to reduce clutter

### Provider Selection

- Use the builtin provider for standard Claude access
- Use custom providers for specialized models or alternative APIs
- Set `active_provider` in config for your default
- All providers are treated equally - no hardcoded preferences

### Hook Management

- Hooks are auto-installed when creating sessions
- Use `ccc install-hooks` for existing projects
- Use `ccc cleanup-hooks` to remove hooks when done

### Performance

- ccc polls for transcript updates every 500ms
- Large files use streaming to avoid Telegram limits
- Sessions run in tmux for persistence

### Security

- Enable OTP mode for shared environments
- Keep `bot_token` private (file permissions: 0600)
- Use `auth_env_var` for provider API keys (not `auth_token`)

## Common Scenarios

### Scenario: Quick Question

```
/private chat on Telegram)
"What's the difference between []interface{} and []any in Go?"
→ Get response in private chat
```

### Scenario: Debug Production Issue

```
/new production-debug
"Check the logs for the auth service error"
[Attach from PC to investigate]
"Show me the recent error patterns"
```

### Scenario: Code Review

```
/new pr-review ~/Projects/myproject
"Review the changes in PR #123"
[Receive summary in Telegram]
[Attach from PC to see details]
```

### Scenario: Scheduled Task

```bash
ccc start nightly-build ~/Projects/myproject "Run build and send me the APK"
```

Receive the built APK on Telegram when complete.
