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
- 🔄 Syncing an already-running Claude session to Telegram with the CCC skill

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

This connects to Telegram, sets up topics, and installs the background listener.

To add the CCC skill to a project, install it through the skills marketplace from that project directory:

```bash
npx skills add
```

In an existing Claude Code or Codex session, trigger the skill to run `ccc sync`; ccc will reuse the current project topic or create one, refresh hooks, and leave the current session running.

**Native plugin install:**

ccc ships plugin manifests for marketplace-style installs:

- Claude Code: install the `ccc` plugin from this repo/marketplace; it exposes the CCC and CCC Send skills.
- Codex: add the repo-local marketplace, then install/enable the `ccc` and `ccc-send` plugins from that marketplace.

For users who intentionally want the skill available globally instead of project-scoped, run:

```bash
ccc skill
```

`ccc skill` writes global Claude Code and Codex skill files. Project-scoped installs should use the marketplace flow above.

### 3. Start Coding

```bash
cd ~/myproject
ccc
```

Then in Telegram, create a session:

```
/new myproject
```

For a GitHub repo, send the URL and choose a provider from the inline picker:

```
/new https://github.com/tuannvm/gemini-mcp-server
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
- 📌 **Pinned Session Context** — Session, provider, and path stay visible in each topic
- 🔄 **Seamless Handoff** — Start on phone, continue on PC
- 🧩 **Skill Handoff** — Link an existing Claude session to Telegram without restarting it
- ⚡ **Streaming Responses** — Real-time typing effect for AI messages
- 📤 **File Transfer** — Send files to your phone
- 🎤 **Voice Messages** — Auto-transcribed voice notes
- 🔒 **100% Self-Hosted** — Runs on your machine, no cloud
- 🏢 **Multiple Backends** — Claude-compatible providers plus Codex CLI

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
| [**API 9.5 Features**](API_9_5_FEATURES.md) | Telegram Bot API 9.5 integration |

## Requirements

- **OS**: macOS, Linux, or Windows (WSL)
- **tmux**: Terminal multiplexer
- **Claude Code or Codex CLI**: install at least one supported backend
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
