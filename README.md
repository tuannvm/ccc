# ccc - Claude Code Companion

> Your AI coding assistant, controlled from your phone 📱

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

![ccc demo](https://github.com/user-attachments/assets/cf291c73-45ae-4d08-8493-782ed1e32d26)

## What is ccc?

**ccc** lets you control Claude Code (AI coding assistant) from your phone via Telegram. Start coding sessions, get notified when tasks complete, and continue seamlessly on your computer.

```
┌─────────────┐                    ┌─────────────┐
│  📱 Phone   │                    │  💻 Computer│
│             │                    │             │
│  ┌────────┐ │                    │  ┌────────┐ │
│  │Telegram│ │ ◄──────────────────► │  tmux   │ │
│  └────────┘ │                    │  └────────┘ │
│             │                    │      │      │
│             │                    │      ▼      │
│             │                    │  ┌────────┐ │
│             │                    │  │Claude  │ │
│             │                    │  │  Code  │ │
│             │                    │  └────────┘ │
└─────────────┘                    └─────────────┘

     Send prompts                    Get responses &
     from phone                      notifications
```

## Why Use ccc?

### You're Away From Your Desk 🏖️

```
You (at cafe): "Fix the authentication bug"
    ↓
Claude starts working on your PC
    ↓
You get notified: "Fixed! Updated login.ts with proper validation"
    ↓
Back at desk: ccc attach → Continue exactly where Claude left off
```

### Long-Running Tasks ⏰

```
You: "Run full test suite and report results"
    ↓
Go about your day...
    ↓
Notification: "Tests complete: 3 failures, 47 passing"
    ↓
You: "Show me the failures"
Claude: Shows detailed failure reports
```

### Quick Questions 💡

```
You: "What's the difference between []interface{} and []any?"
    ↓
Immediate response in Telegram
    ↓
No need to open your laptop
```

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│                     MESSAGE FLOW                             │
└─────────────────────────────────────────────────────────────┘

  You (Telegram) ──► ccc listen ──► tmux window ──► Claude Code
       │                                    │                  │
       │                                    │                  │
       ◄─────────────────────────────────────┴──────────────────┘
       │
       ▼
  Claude's response (via Hook System)
```

**The Flow:**

1. **You send a message** in a Telegram topic
2. **ccc receives it** and sends to Claude in the tmux window
3. **Claude processes** and writes response to transcript file
4. **Hook detects** the new response and sends it back to Telegram
5. **You receive** the response in the same topic

```
┌────────────────────────────────────────────────────────────────┐
│                    SESSION ARCHITECTURE                         │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Telegram Topic  ═══  Project Directory  ═══  tmux Window      │
│       📱                  📁                  💻               │
│                                                                │
│  "myproject"   ─────►  ~/myproject      ─────►  Claude Code    │
│  "experiment"  ─────►  ~/experiment     ─────►  Running        │
│  "api-test"    ─────►  ~/api-test       ─────►                 │
│                                                                │
│  Each topic = one coding session                                │
│  Switch topic = switch project                                   │
└────────────────────────────────────────────────────────────────┘
```

**Multi-Pane Sessions** (NEW):
Each session can have multiple Claude panes running in parallel:

```
myproject session
├── Pane 0: coder (using Opus)       ← active
└── Pane 1: reviewer (using Haiku)
```

Send prompts to specific panes:
```
/pane coder "Implement the feature"
/pane reviewer "Review the changes in auth.go"
```

## Features

| Feature | Description |
|---------|-------------|
| 📱 **Remote Control** | Start and manage sessions from Telegram |
| 🔔 **Smart Notifications** | Get notified when tasks complete |
| 📁 **Multi-Session** | Multiple projects, each with its own topic |
| 🎯 **Multi-Pane** | Parallel Claude instances per session (coder + reviewer) |
| 🔄 **Seamless Handoff** | Start on phone, continue on PC |
| 📤 **File Transfer** | Send files to your phone (large files via streaming) |
| 🎤 **Voice Messages** | Send voice notes, auto-transcribed |
| 🖼️ **Image Support** | Send images for Claude to analyze |
| 🔒 **100% Self-Hosted** | Runs on your machine, no cloud servers |
| 🏢 **Multiple Providers** | Anthropic, Zai, OpenAI, and more |

## Quick Start

### Step 1: Create Telegram Bot (30 seconds)

```
1. Open Telegram
2. Search for @BotFather
3. Send: /newbot
4. Follow prompts, choose a name
5. Save the token (looks like: 123456789:ABCdefGHI...)
```

### Step 2: Install ccc

```bash
# Clone and install
git clone https://github.com/kidandcat/ccc.git
cd ccc
make install
```

### Step 3: Run Setup

```bash
ccc setup YOUR_BOT_TOKEN
```

This single command:
- ✅ Connects to your Telegram
- ✅ Sets up session topics
- ✅ Installs Claude hooks
- ✅ Starts background service

### Step 4: Start Coding!

```bash
cd ~/myproject
ccc
```

Then in Telegram, create a session:

```
/new myproject
```

That's it! Send your first prompt:

```
"Help me add user authentication to this Express.js app"
```

## Configuration

ccc stores configuration in `~/.config/ccc/config.json`. Run `ccc config` to manage settings.

### Quick Config Commands

```bash
# View current config
ccc config show

# Set bot token
ccc config set bot-token YOUR_BOT_TOKEN

# Set chat/group ID (automatically discovered during setup)
ccc config set chat-id 123456789
ccc config set group-id -1001234567890

# Set active provider
ccc config set active-provider my-provider

# List available providers
ccc config list-providers
```

### Provider Configuration

ccc supports multiple AI providers through a flexible provider system. Configure custom providers in your config file:

```json
{
  "active_provider": "my-custom-provider",
  "providers": {
    "my-custom-provider": {
      "base_url": "https://api.example.com/v1",
      "auth_env_var": "MY_API_KEY",
      "opus_model": "claude-3-opus-20250214",
      "sonnet_model": "claude-3-7-sonnet-20250214",
      "haiku_model": "claude-3-5-haiku-20250214",
      "subagent_model": "claude-3-5-haiku-20250214",
      "config_dir": "~/.claude",
      "api_timeout": 120000
    }
  }
}
```

**Provider Options:**

| Field | Type | Description |
|-------|------|-------------|
| `base_url` | string | API base URL (e.g., `https://api.anthropic.com`) |
| `auth_token` | string | API key (not recommended - use `auth_env_var`) |
| `auth_env_var` | string | Environment variable name containing API key |
| `opus_model` | string | Model name for Opus |
| `sonnet_model` | string | Model name for Sonnet |
| `haiku_model` | string | Model name for Haiku |
| `subagent_model` | string | Model name for subagent |
| `config_dir` | string | Provider config directory (supports `~`) |
| `api_timeout` | int | API timeout in milliseconds |

**Built-in Provider:**

The `anthropic` provider is built-in and uses:
- Environment variable: `ANTHROPIC_API_KEY`
- Default models from Claude Code
- Default config directory: `~/.claude`

**Session-Level Provider:**

Assign a specific provider to a session:

```json
{
  "sessions": {
    "myproject": {
      "topic_id": 42,
      "path": "/home/user/Projects/myproject",
      "provider_name": "my-custom-provider"
    }
  }
}
```

### Environment Variables

Some settings can be overridden via environment variables:

```bash
# Anthropic API key (for builtin provider)
export ANTHROPIC_API_KEY="sk-ant-..."

# Custom base URL
export ANTHROPIC_BASE_URL="https://api.example.com"

# Custom models
export ANTHROPIC_DEFAULT_SONNET_MODEL="claude-3-7-sonnet-20250214"
```

## Session Organization

```
┌─────────────────────────────────────────────────────────────────┐
│              TELEGRAM GROUP → YOUR COMPUTER                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  📱 Telegram Topics        📁 Project Directories             │
│                                                                 │
│  ┌─────────────┐           ┌──────────────┐                    │
│  │ myproject   │ ════════► │ ~/myproject  │                    │
│  └─────────────┘           └──────────────┘                    │
│                                                                 │
│  ┌─────────────┐           ┌──────────────┐                    │
│  │ experiment  │ ════════► │ ~/experiment │                    │
│  └─────────────┘           └──────────────┘                    │
│                                                                 │
│  ┌─────────────┐           ┌──────────────┐                    │
│  │ api-test    │ ════════► │ ~/api-test   │                    │
│  └─────────────┘           └──────────────┘                    │
│                                                                 │
│  Each topic = one session, Switch topic = switch project       │
└─────────────────────────────────────────────────────────────────┘
```

## Common Usage Patterns

### Pattern 1: Start Remote, Finish Local

```
📱 /new myproject  ─────►  💻 Work on PC  ─────►  📱 Get notified
                                              │
                                              ▼
                                      💻 Continue working
```

### Pattern 2: Monitor Long Tasks

```
📱 "Run full tests"  ─────►  ⏰ Do other things  ─────►  🔔 Notification!
                                                              │
                                                              ▼
                                                    📱 "Show failures"
```

```
┌─────────────────────────────────────────────────────────────────┐
│                    LONG-RUNNING TASK FLOW                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. 📱 Send: "Run full test suite & report"                    │
│                                                                 │
│  2. 💻 Claude starts testing...                                │
│                                                                 │
│  3. 🏃 You go about your day                                    │
│                                                                 │
│  4. 🔔 Notification: "Tests complete: 3 failures, 47 passing"    │
│                                                                 │
│  5. 📱 Ask: "Show me the failures"                              │
│                                                                 │
│  6. 📱 Get detailed failure reports                             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Requirements

- **OS**: macOS, Linux, or Windows (WSL)
- **Go**: 1.21+
- **tmux**: Terminal multiplexer
- **Claude Code**: [Install here](https://claude.ai/claude-code)
- **Telegram**: Account + bot token

## Documentation

| Document | Description |
|----------|-------------|
| [**Architecture**](docs/architecture.md) | System design, data flow diagrams |
| [**Configuration**](docs/configuration.md) | All configuration options |
| [**Usage Guide**](docs/usage.md) | Complete command reference |
| [**Troubleshooting**](docs/troubleshooting.md) | Common issues & solutions |
| [**Changelog**](docs/changelog.md) | Version history |
| [**Refactor Design**](docs/REFACTOR_DESIGN.md) | Config system architecture & design |

**Project Structure:**
- `types.go` - All struct definitions
- `config_*.go` - Modular config system (load, save, paths, validation)
- `session*.go` - Session management (lookup, persistence, pane CRUD)
- `provider.go` - Provider abstraction layer
- `tmux.go` - Tmux operations (pane lifecycle, detection)
- `telegram.go` - Telegram Bot API integration
- `hooks.go` - Claude Code hook system
- `ledger.go` - Message delivery tracking

## Privacy & Security

```
┌─────────────────────────────────────────────────────────────────┐
│                    WHERE YOUR DATA GOES                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Your Code            ══════════  Your Computer Only             │
│  💻 Project files                  🔒 Local Storage             │
│                                                                 │
│  Conversations        ══════════  Telegram + AI Provider         │
│  💬 Messages                         📡 Encrypted             │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ ✅ Runs on YOUR machine                                  │   │
│  │ ✅ No cloud servers                                      │   │
│  │ ✅ No telemetry or tracking                               │   │
│  │ ✅ Open source - audit it yourself                       │   │
│  │ ✅ Only your Telegram ID can send commands               │   │
│  │ ✅ Optional OTP mode for permission approval              │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

- ✅ **Self-hosted**: Runs entirely on your machine
- ✅ **No tracking**: No telemetry, no analytics
- ✅ **Open source**: Full code transparency
- ✅ **Authorization**: Only your Telegram ID can send commands
- ✅ **OTP mode**: Optional TOTP for permission approval

## Troubleshooting

**Problem? Run this first:**

```bash
ccc doctor
```

**Common issues:**

| Issue | Solution |
|-------|----------|
| Bot not responding | `systemctl --user status ccc` |
| Session not starting | `which claude` - verify Claude Code installed |
| Messages not reaching | Try `/new` to restart session |

See [Troubleshooting guide](docs/troubleshooting.md) for more help.

## Contributing

Contributions welcome!

```bash
git clone https://github.com/kidandcat/ccc.git
cd ccc
go test ./...
```

## License

[MIT License](LICENSE)

---

Made with Claude Code 🤖

For questions or issues, visit https://github.com/kidandcat/ccc
