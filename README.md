# ccc - Claude Code Companion

> Control Claude Code from your phone 📱

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/tuannvm/ccc)](https://github.com/tuannvm/ccc/releases/latest)

---

![ccc demo](https://github.com/user-attachments/assets/cf291c73-45ae-4d08-8493-782ed1e32d26)

## What is ccc?

**ccc** lets you control Claude Code from your phone via Telegram. Start coding sessions, get notified when tasks complete, and continue seamlessly on your computer.

**Perfect for:**
- 🏖️ Starting tasks while away from your desk
- ⏰ Monitoring long-running tests and builds
- 💡 Quick questions without opening your laptop

## Install

**One-line install (macOS/Linux):**

```bash
curl -sSL https://raw.githubusercontent.com/tuannvm/ccc/main/install.sh | bash
```

**Or download manually:**

```bash
# Download latest release for your platform
curl -LO https://github.com/tuannvm/ccc/releases/latest/download/ccc_VERSION_linux_amd64.tar.gz
tar -xzf ccc_VERSION_linux_amd64.tar.gz
sudo mv ccc /usr/local/bin/
```

**From source:**

```bash
git clone https://github.com/tuannvm/ccc.git
cd ccc
make install
```

## Quick Start

### 1. Create a Telegram Bot (30 sec)

```
1. Open Telegram → @BotFather
2. Send: /newbot
3. Follow prompts, save your token
```

### 2. Setup ccc

```bash
ccc setup YOUR_BOT_TOKEN
```

This connects to Telegram, sets up topics, and installs hooks.

### 3. Start Coding

```bash
cd ~/myproject
ccc
```

Then in Telegram, create a session:

```
/new myproject
```

Send your first prompt:

```
"Help me add user authentication to this Express.js app"
```

That's it! 🎉

## Features

- 📱 **Remote Control** — Start sessions from Telegram
- 🔔 **Smart Notifications** — Get notified when tasks complete
- 📁 **Multi-Session** — Multiple projects, each in its own topic
- 🌳 **Git Worktrees** — Auto-generated sessions with color grouping
- 🔄 **Seamless Handoff** — Start on phone, continue on PC
- 📤 **File Transfer** — Send files to your phone
- 🎤 **Voice Messages** — Auto-transcribed voice notes
- 🔒 **100% Self-Hosted** — Runs on your machine, no cloud
- 🏢 **Multiple Providers** — Anthropic, Zai, OpenAI, and more

## Privacy & Security

✅ Runs locally on your machine
✅ No telemetry or tracking
✅ Only your Telegram ID can send commands
✅ Optional OTP mode for permission approval

## Documentation

| Guide | Description |
|-------|-------------|
| [**Usage Guide**](docs/usage.md) | Commands, sessions, patterns |
| [**Configuration**](docs/configuration.md) | Providers, settings, environment |
| [**Architecture**](docs/architecture.md) | System design, data flow |
| [**Troubleshooting**](docs/troubleshooting.md) | Common issues & solutions |
| [**Changelog**](docs/changelog.md) | Version history |

## Requirements

- **OS**: macOS, Linux, or Windows (WSL)
- **tmux**: Terminal multiplexer
- **Claude Code**: [Install here](https://claude.ai/claude-code)
- **Telegram**: Account + bot token

## Troubleshooting

Having issues? Run:

```bash
ccc doctor
```

See [Troubleshooting guide](docs/troubleshooting.md) for more help.

## License

[MIT License](LICENSE)

---

Made with Claude Code 🤖

For questions or issues, visit [github.com/tuannvm/ccc](https://github.com/tuannvm/ccc)
