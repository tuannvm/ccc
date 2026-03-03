# Troubleshooting

This document covers common issues and their solutions.

## Quick Diagnosis

Always start with:

```bash
ccc doctor
```

This checks:
- ✓ tmux installation
- ✓ Claude Code installation
- ✓ Configuration file
- ✓ Hook installation
- ✓ Service status

## Common Issues

### "Session created but died immediately"

**Symptoms:**
- `/new myproject` returns "Session created but died immediately"
- No tmux window is created
- Claude doesn't start

**Causes & Solutions:**

1. **Wrong tmux socket path (Linux vs macOS)**

   ccc now auto-detects the correct socket path. If issues persist:
   ```bash
   # Check tmux socket
   ls /tmp/tmux-$(id -u)/default    # Linux
   ls /private/tmp/tmux-$(id -u)/default  # macOS
   ```

2. **Claude binary not found**

   Ensure Claude Code is installed:
   ```bash
   which claude
   claude --version
   ```

   If installed via npm with nvm:
   ```bash
   # Add to PATH
   export PATH="$HOME/.nvm/versions/node/$(node -v)/bin:$PATH"
   ```

3. **Project directory not created**

   This was fixed in recent versions. Ensure you have the latest ccc:
   ```bash
   /update
   ```

### "Bot not responding"

**Symptoms:**
- Messages sent to bot get no response
- Commands like `/new` don't work

**Causes & Solutions:**

1. **`ccc listen` not running**

   Check service status:
   ```bash
   systemctl --user status ccc
   ```

   Start if needed:
   ```bash
   systemctl --user start ccc
   ```

2. **Wrong bot token**

   Verify in config:
   ```bash
   ccc config bot_token
   ```

   Should look like: `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`

3. **Not authorized**

   Check `chat_id` in config:
   ```bash
   ccc config chat_id
   ```

   Re-run setup if needed:
   ```bash
   ccc setup YOUR_BOT_TOKEN
   ```

4. **Network issues**

   Check logs:
   ```bash
   tail -f ~/Library/Caches/ccc/ccc.log  # macOS
   journalctl --user -u ccc -f           # Linux
   ```

### "Messages not reaching Claude"

**Symptoms:**
- Bot responds to commands
- Prompts don't reach Claude
- No response in topic

**Causes & Solutions:**

1. **Wrong topic**

   Ensure you're in the correct topic for the session

2. **Claude not running**

   Check tmux windows:
   ```bash
   tmux list-windows -t ccc
   ```

   Restart if needed:
   ```
   /new
   ```

3. **Hook not installed**

   Install hooks:
   ```bash
   cd ~/myproject
   ccc install-hooks
   ```

4. **Session path mismatch**

   Check session config:
   ```bash
   ccc config sessions
   ```

### "Permission denied" errors

**Symptoms:**
- Can't create config file
- Can't install hooks
- Service won't start

**Causes & Solutions:**

1. **Directory permissions**

   ```bash
   chmod 755 ~/.config
   chmod 755 ~/.config/ccc
   ```

2. **Config file permissions**

   ```bash
   chmod 600 ~/.config/ccc/config.json
   ```

3. **Hook directory permissions**

   ```bash
   chmod 755 ~/.claude/hooks
   chmod +x ~/.claude/hooks/*
   ```

### "Claude stuck at 'Do you trust these files?'"

**Symptoms:**
- Claude prompts for trust on every run
- Can't send commands to Claude

**Solution:**

ccc should auto-configure trusted directories. If issues persist:

1. Check provider settings:
   ```bash
   cat ~/.claude/settings.json
   ```

   Should contain:
   ```json
   {
     "trustedDirectories": ["~/Projects", "~/Projects/cli"],
     "autoApprove": {
       "trustDirectories": true
     }
   }
   ```

2. Manually add trusted directory:
   ```bash
   ccc trust ~/myproject
   ```

### "No conversation found to continue"

**Symptoms:**
- `/continue` command fails
- Can't resume previous session

**Causes & Solutions:**

1. **Claude session ID not saved**

   Check session config:
   ```bash
   ccc config sessions
   ```

   Look for `claude_session_id` field

2. **Transcript not found**

   Check if transcript exists:
   ```bash
   ls ~/.claude/transcripts/
   ```

3. **Provider mismatch**

   Ensure you're using the same provider:
   ```
   /provider
   ```

### "File transfer not working"

**Symptoms:**
- `ccc send` hangs or fails
- Download link doesn't work

**Causes & Solutions:**

1. **File too large for Telegram (>50MB)**

   Large files use relay. Check relay URL:
   ```bash
   ccc config relay_url
   ```

2. **Relay server down**

   Try default relay or host your own

3. **Firewall blocking**

   Ensure outbound connections are allowed

### "Voice messages not transcribing"

**Symptoms:**
- Voice message received but no text
- Error in logs

**Causes & Solutions:**

1. **Transcription not configured**

   Check config:
   ```bash
   ccc config transcription_cmd
   ```

2. **Whisper not installed**

   Install:
   ```bash
   pip install openai-whisper
   ```

3. **API key not set**

   For Groq/OpenAI, ensure env var is set:
   ```bash
   export GROQ_API_KEY="your-key"
   ```

## Debug Mode

### Enable Debug Logging

```bash
# Stop service
systemctl --user stop ccc

# Run with debug output
ccc listen --debug
```

### Check Hook Logs

```bash
# macOS
tail -f ~/Library/Caches/ccc/hook-debug.log

# Linux
tail -f ~/.cache/ccc/hook-debug.log
```

### View Tmux Session

```bash
# List all sessions
tmux list-sessions

# Attach to ccc session
tmux attach -t ccc

# List windows
tmux list-windows -t ccc

# View window content
tmux capture-pane -t ccc:myproject -p
```

## Getting Help

### Collect Diagnostic Information

```bash
# Run diagnostics
ccc doctor > ccc-doctor.txt

# Get logs
tail -100 ~/Library/Caches/ccc/ccc.log > ccc.log  # macOS
journalctl --user -u ccc -n 100 > ccc.log          # Linux

# Get config (remove sensitive data)
ccc config > ccc-config.txt

# Get session list
tmux list-sessions > tmux-sessions.txt
tmux list-windows -t ccc > tmux-windows.txt
```

### Report an Issue

When reporting issues, include:

1. ccc version: `ccc --version`
2. OS and version
3. Output of `ccc doctor`
4. Relevant logs
5. Steps to reproduce

### Useful Commands

| Command | Purpose |
|---------|---------|
| `ccc doctor` | Check all dependencies |
| `ccc config` | View configuration |
| `tmux list-sessions` | Check tmux state |
| `ps aux | grep ccc` | Check running processes |
| `systemctl --user status ccc` | Check service status |

## Known Issues

### Session Restart Bug (Fixed)

**Issue:** Every prompt caused session restart even when Claude was running.

**Status:** Fixed in version 1.2.1

**Workaround:** Update to latest version:
```
/update
```

### macOS Code Signing

**Issue:** `killed` error when running ccc on macOS.

**Solution:**
```bash
codesign -s - ~/bin/ccc
```

### WSL2 Windows Integration

**Issue:** ccc doesn't work directly on Windows.

**Solution:** Use WSL2 with Ubuntu:
```bash
# In WSL2
sudo apt update
sudo apt install -y tmux
```

Then follow Linux instructions.

## Performance Tips

### Reduce Polling Overhead

If running many sessions, transcript polling can add up. Solutions:

1. Archive completed topics
2. Stop unused sessions: `/delete` in topic
3. Use `ccc cleanup-hooks` for finished projects

### Optimize File Transfers

For large files:
- Use relay URL for files > 50MB
- Keep `ccc send` running until download completes
- Compress large files before sending

### Reduce Memory Usage

- Limit concurrent sessions (5-10 recommended)
- Clear old transcripts periodically
- Restart ccc service weekly

## Recovery Procedures

### Recover Orphaned Sessions

If tmux session exists but config is missing:

```bash
# Attach to tmux
tmux attach -t ccc

# Note window names and session names

# Re-create in config
ccc config sessions add myproject ~/Projects/myproject
```

### Reset Configuration

If config is corrupted:

```bash
# Backup current config
cp ~/.config/ccc/config.json ~/.config/ccc/config.json.bak

# Re-run setup
ccc setup YOUR_BOT_TOKEN
```

### Clean Reinstall

```bash
# Stop service
systemctl --user stop ccc

# Remove binary
rm ~/bin/ccc

# Remove config (optional)
rm -rf ~/.config/ccc

# Reinstall
cd ~/Projects/ccc
make install

# Re-setup
ccc setup YOUR_BOT_TOKEN
```

## Contact & Support

- **GitHub Issues:** https://github.com/kidandcat/ccc/issues
- **Documentation:** https://github.com/kidandcat/ccc
- **README:** See main README for quick start

When reporting issues, please use the diagnostic collection procedure above.
