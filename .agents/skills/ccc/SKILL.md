---
name: ccc
description: Sync, link, continue, hand off, or take over the current Claude Code or Codex-backed session from Telegram by running `ccc sync`.
allowed-tools: Bash
---

# CCC - Telegram Session Sync

## Description
Use CCC when the user asks to sync, link, continue, hand off, or take over the current Claude Code or Codex-backed session from Telegram. CCC creates or reuses the Telegram topic for the current project, installs the project hooks, and keeps the current agent session running.

## Usage
When this skill is triggered from Codex, run:

```bash
CCC_AGENT_PROVIDER=codex ccc sync
```

When this skill is triggered from Claude Code, run:

```bash
CCC_AGENT_PROVIDER=anthropic ccc sync
```

If you know the exact current provider name, use it instead of the defaults above. If the runtime exposes a session or thread id, pass it as `CCC_AGENT_SESSION_ID`:

```bash
CCC_AGENT_PROVIDER=codex CCC_AGENT_SESSION_ID=$CODEX_THREAD_ID ccc sync
```

If the runtime is unknown, run:

```bash
ccc sync
```

If the user gave a short handoff note that should appear in Telegram, include it:

```bash
CCC_AGENT_PROVIDER=codex ccc sync "Continuing this agent session from Telegram."
```

## How It Works
- If the current directory already maps to a CCC session, CCC reuses that Telegram topic.
- If no session maps to the current directory, CCC creates a new Telegram topic using the normal provider/session flow.
- `CCC_AGENT_PROVIDER` tells CCC whether this running session is Codex or Claude so Telegram resumes the same agent backend instead of using the configured default.
- `CCC_AGENT_SESSION_ID` lets CCC store an exact resume target when the runtime exposes one. Codex may not expose this to shell commands; in that case CCC uses Codex resume-last behavior for that backend.
- CCC installs or refreshes project-local hooks so later prompts, tool updates, permission requests, and completion messages are routed to the topic.
- CCC does not attach tmux or restart the agent; the current session keeps control after the sync command returns.

## Important Notes
- Run the command from the project directory the current session is working in.
- Do not run plain `ccc` for this skill; that command attaches tmux and is only for starting CCC at the beginning of a terminal session.
- After `ccc sync` succeeds, continue the user's request normally.
