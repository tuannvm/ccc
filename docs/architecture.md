# Architecture

This document describes the system architecture of ccc (Claude Code Companion), its components, data flow, and design decisions.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLIENT LAYER                            │
├─────────────────────────────────────────────────────────────────┤
│  📱 Mobile Phone          💻 Terminal/Tmux                      │
│       │                         │                               │
│       ▼                         │                               │
│  ┌─────────┐                   │                               │
│  │Telegram │                   │                               │
│  └─────────┘                   │                               │
└─────────────────────────────────────────────────────────────────┘
            │                           │
            ▼                           │
┌─────────────────────────────────────────────────────────────────┐
│                          CCC SERVICE                             │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌─────────────┐  ┌──────────┐  ┌──────────┐   │
│  │ccc listen│  │Config Manager│  │Hook Syst.│  │Tmux Mgr. │   │
│  └──────────┘  └─────────────┘  └──────────┘  └──────────┘   │
│        │              │               │              │          │
│        │              └───────────────┴──────────────┘          │
│        │                     ┌─────────────┐                    │
│        └────────────────────►│Session Mgr. │                    │
│                             └─────────────┘                    │
└─────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                         CLAUDE CODE                             │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐        ┌──────────────┐                       │
│  │Claude Code CLI│────────►│Transcript Files│                     │
│  └──────────────┘        └──────────────┘                       │
└─────────────────────────────────────────────────────────────────┘

LEGEND:
══════  Messages/Notifications
──────  Tmux Operations
──────  Config/State
```

## Component Overview

### Core Components

| Component | File | Responsibility |
|-----------|------|---------------|
| **Telegram Listener** | `telegram.go`, `commands.go` | Polls Telegram for messages, handles commands, routes prompts to sessions |
| **Tmux Manager** | `tmux.go` | Creates/manages tmux sessions, switches windows, detects Claude state |
| **Session Manager** | `session.go` | Manages session lifecycle, creates topics, persists state |
| **Config Manager** | `config.go` | Loads/saves config, manages providers and sessions |
| **Hook System** | `hooks.go` | Installs Claude Code hooks, reads transcripts, sends notifications |
| **Provider Abstraction** | `provider.go` | Provider-agnostic interface for AI providers |

## Message Flow

### 1. Creating a New Session

```
┌─────────────────────────────────────────────────────────────────┐
│                    SESSION CREATION FLOW                         │
└─────────────────────────────────────────────────────────────────┘

  User           Telegram        ccc listen     Session Mgr    Tmux      Claude
   │                 │                │              │         │
   │  /new myproject │                │              │         │
   ├────────────────►│                │              │         │
   │                 │  Message recv   │              │         │
   │                 ├───────────────►│              │         │
   │                 │                │  Create topic │         │
   │                 │                ├─────────────►│         │
   │                 │                │              │ Create window
   │                 │                │              ├────────►│
   │                 │                │              │         │ ccc run
   │                 │                │              │         ├──────►
   │                 │                │              │         │ Running
   │                 │                │  Created     │         │
   │                 │                │◄─────────────┤         │
   │                 │  🚀 Started!    │              │         │
   │◄────────────────│◄───────────────┤              │         │
   │                                                                         │
```

### 2. Sending a Prompt

```
┌─────────────────────────────────────────────────────────────────┐
│                      PROMPT PROCESSING FLOW                     │
└─────────────────────────────────────────────────────────────────┘

  User       Telegram      ccc listen     Tmux Mgr    Claude    Hook System
   │             │              │            │          │          │
   │ "Fix bug"   │              │            │          │          │
   ├───────────►│              │            │          │          │
   │             │  Message recv │            │          │          │
   │             ├─────────────►│            │          │          │
   │             │              │ Find session│          │          │
   │             │              ├───────────►│          │          │
   │             │              │            │ Switch   │          │
   │             │              │            ├─────────►│          │
   │             │              │            │         Send prompt│     │
   │             │              │            ├──────────────────►│     │
   │             │              │            │          Process  │    │
   │             │              │            │          ├──────►│     │
   │             │              │            │          Write transcript│ │
   │             │              │            │          │   │      │
   │             │              │            │          │  ◄─────┤     │
   │             │              │            │          │  Poll   │     │
   │             │              │            │          │  │      │     │
   │             │              │            │          │  ◄─────┤     │
   │             │              │            │          │        │     │
   │             │              │            │          │  New content│
   │             │              │            │          │  │      │     │
   │             │              │            │          │  ◄─────┤     │
   │             │              │            │          │        │     │
   │             │              │  Response   │          │        │     │
   │             │  Claude response◄────────────┼──────────┼────────┤     │
   │  Response   ◄───────────────┤            │          │        │     │
   │◄────────────│              │            │          │        │     │
```

### 3. Hook Notification Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    NOTIFICATION WORKFLOW                         │
└─────────────────────────────────────────────────────────────────┘

  Claude Code    Transcript File    Hook System    Response Parser    Telegram    User
      │                 │                 │                │             │         │
      │ Write           │                 │                │             │         │
      ├────────────────►│                 │                │             │         │
      │                 │                 │                │             │         │
      │                 │                 │  Poll           │             │         │
      │                 │◄────────────────┤                │             │         │
      │                 │                 │  New content    │             │         │
      │                 │                 ├───────────────►│             │         │
      │                 │                 │                │ Extract     │         │
      │                 │                 │                ├──────────►│         │
      │                 │                 │                │          Send│        │
      │                 │                 │                ├──────────►│         │
      │                 │                 │                │             │ Notify  │
      │                 │                 │                │             ├────────►│
```

## Session Lifecycle

```
┌─────────────────────────────────────────────────────────────────┐
│                      SESSION LIFECYCLE                           │
└─────────────────────────────────────────────────────────────────┘

    ┌─────────┐
    │  START  │
    └────┬────┘
         │ /new command
         ▼
    ┌─────────┐
    │Creating │ ◄─────────────┐
    └────┬────┘                │
         │ Topic created        │
         ▼                      │
    ┌─────────┐                │
    │Starting │                │
    └────┬────┘                │
         │ Claude started       │
         ▼                      │
    ┌─────────┐                │
    │ Running │◄───────────────┘
    └────┬────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌─────────┐ ┌───────┐
│  Idle   │ │Processing│
│(waiting │ │  (working)│
│ input)  │ └─────┬─────┘
└────┬────┘       │
     │             │
     │             │◄──────┐
     │             │       │ Prompt
     │             │       │ received
     │             ▼       │
     │         ┌───────┐   │
     │         │Running│───┘
     │         └───────┘
     │             │
     │             │ User disconnects
     │             ▼
     │         ┌─────────┐
     │         │ Detached│
     │         │(background)
     │         └────┬────┘
     │              │
     │              │ /delete or error
     │              ▼
     │         ┌─────────┐
     │         │ Stopped │
     │         └────┬────┘
     │              │
     └──────────────┘
```

## Tmux Integration

### Window Management

Each session gets its own tmux window within the shared "ccc" session:

```
ccc (tmux session)
├── myproject (window)
├── experiment (window)
└── test (window)
```

### Claude Detection

The system uses multiple methods to detect if Claude is running:

1. **Process-based detection**: Checks if `claude` or `node` process is active
2. **Prompt-based detection**: Looks for Claude's prompt character (❯) in pane content
3. **Child process detection**: Checks if shell has Claude as child process
4. **npm Claude detection**: Handles npm-installed Claude via `claude/cli`

### Session Switching

When switching between sessions:

1. Check if target window exists
2. Detect if Claude is running in target
3. If `skipRestart=true`: Preserve session, send prompts directly
4. If `skipRestart=false`: May restart to ensure clean state

## Provider System

ccc uses a provider abstraction to support multiple AI providers:

### Provider Interface

```go
type Provider interface {
    Name() string
    BaseURL() string
    AuthToken(config *Config) string
    Models() ModelConfig
    ConfigDir() string
    TranscriptPath(sessionID string) string
    EnvVars(config *Config) []string
    IsBuiltin() bool
}
```

### Provider Types

1. **BuiltinProvider**: Default Anthropic provider using environment variables
2. **ConfiguredProvider**: Custom providers from `config.json`

### Provider Resolution

```
┌─────────────────────────────────────────────────────────────────┐
│                    PROVIDER SELECTION FLOW                        │
└─────────────────────────────────────────────────────────────────┘

  Session Request
        │
        ▼
  ┌───────────────────┐
  │ Provider Specified?│
  └────┬──────────────┘
       │
  ┌────┴────┐
  │         │
 Yes       No
  │         │
  ▼         ▼
┌────────┐ ┌──────────────────┐
│Use     │ │Active Provider Set?│
│Specified│ └────┬──────────────┘
└────┬───┘      │
     │         │
     │    ┌────┴────┐
     │    │         │
     │   Yes       No
     │    │         │
     │    ▼         ▼
     │ ┌──────┐ ┌────────┐
     │ │Use   │ │Use     │
     │ │Active│ │Builtin │
     │ └──┬───┘ └────────┘
     │    │
     ▼    ▼
┌───────────────────┐
│ Apply Provider   │
│ Environment Vars │
└─────────┬─────────┘
          │
          ▼
┌───────────────────┐
│  Start Claude     │
│      Code         │
└───────────────────┘
```

## Hook System

### Hook Installation

Hooks are installed per-project when a session is created:

```bash
.claude/
├── hooks/
│   ├── pre-run   # Runs before any command
│   ├── post-run   # Runs after command completes
│   └── ask        # Runs before permission approval
└── settings.json
```

### Hook Functionality

1. **Transcript Monitoring**: Polls `transcript.jsonl` for new content
2. **Response Extraction**: Parses assistant responses and tool results
3. **Telegram Notifications**: Sends responses back to the appropriate topic
4. **Permission Handling**: Integrates with OTP mode for remote approval

### Per-Project Hooks

ccc supports per-project hook installation:

```bash
ccc install-hooks        # Install hooks in current project
ccc cleanup-hooks        # Remove hooks from current project
```

## Configuration Structure

```mermaid
graph TD
    A[config.json] --> B[Bot Token]
    A --> C[Chat ID]
    A --> D[Group ID]
    A --> E[Providers]
    A --> F[Sessions]
    A --> G[Settings]

    E --> H[Provider 1]
    E --> I[Provider 2]
    E --> J[Active Provider]

    F --> K[Session 1]
    F --> L[Session 2]

    K --> M[Topic ID]
    K --> N[Path]
    K --> O[Provider]
    K --> P[Claude Session ID]

    G --> Q[OTP Secret]
    G --> R[Away Mode]
    G --> S[Projects Dir]
```

### Permission Modes

| Mode | Behavior |
|------|----------|
| **Auto-approve** (default) | All permissions automatically approved |
| **OTP** | Remote prompts require TOTP code approval |

### Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    AUTHORIZATION & PERMISSION FLOW                │
└─────────────────────────────────────────────────────────────────┘

  User Message
      │
      ▼
┌─────────────────┐
│  Authorized?    │
└────┬────────────┘
     │
     │
 ┌───┴───┐
 │       │
 No      Yes
 │       │
 ▼       ▼
Rejected │
     ┌───────────────────┐
     │   OTP Mode?       │
     └────┬──────────────┘
          │
     ┌────┴─────┐
     │          │
    Yes        No
     │          │
     ▼          ▼
┌────────────────┐  ┌─────────┐
│Permission Needed?│  │Send to  │
└────┬─────────┘  │Claude   │
     │            └─────────┘
 ┌───┴───┐
 │       │
 No      Yes
 │       │
 ▼       ▼
┌─────────┐ ┌────────────┐
│Send to  │ │Request OTP │
│Claude  │ │   Code     │
└─────────┘ └──────┬─────┘
                  │
                  ▼
          ┌────────────┐
          │User Provides│
          │    OTP      │
          └──────┬─────┘
                 │
                 ▼
          ┌────────────┐
          │   Valid?    │
          └────┬───────┘
               │
        ┌──────┴──────┐
        │             │
       Yes           No
        │             │
        ▼             ▼
    ┌─────────┐  ┌─────────┐
    │Send to  │  │Rejected │
    │Claude  │  └─────────┘
    └─────────┘
```

## File System Layout

```
~/
├── .config/
│   └── ccc/
│       └── config.json          # Main configuration
├── .claude/
│   ├── hooks/                   # Per-project hooks
│   │   ├── pre-run
│   │   ├── post-run
│   │   └── ask
│   ├── settings.json            # Claude Code settings
│   └── transcripts/             # Claude Code transcripts
│       └── <session-id>/
│           └── transcript.jsonl
├── Projects/                    # Default projects directory
│   ├── myproject/
│   └── experiment/
└── bin/
    └── ccc                       # Binary
```

## Concurrency Model

### Single Listener Instance

Only one `ccc listen` instance runs at a time, enforced via lock file:

```
~/Library/Caches/ccc/ccc.lock (macOS)
~/.cache/ccc/ccc.lock (Linux)
```

### Message Processing

- **Sequential processing**: Messages processed one at a time
- **Non-blocking I/O**: Uses Telegram long-polling with timeout
- **Graceful shutdown**: Handles SIGTERM/SIGINT for clean exit

### Session Isolation

- Each session runs in its own tmux window
- Sessions are isolated but share the same tmux session
- No shared state between sessions except config file

## Error Handling

### Retry Logic

| Operation | Retry Strategy |
|-----------|---------------|
| Telegram API | Exponential backoff, max 3 attempts |
| Claude Detection | Multiple detection methods with fallback |
| Tmux Operations | Retry once on failure |
| Hook Transcript Read | Continuous polling, no retries on parse errors |

### Failure Modes

1. **Claude Not Found**: Falls back to alternative paths
2. **Tmux Not Available**: Provides clear error message
3. **Config Corruption**: Attempts migration from old format
4. **Network Issues**: Continues polling, logs errors

## Performance Considerations

### Transcript Polling

- Polling interval: 500ms
- Only polls sessions with active topics
- Uses file modification time for optimization

### Memory Usage

- Transcript reader keeps only recent entries in memory
- Message ledger bounded by file size
- No in-memory caching of full conversation history

### Network Usage

- Telegram messages limited to 4000 characters (split automatically)
- File transfer uses relay for large files (>50MB)
- Voice messages transcribed before sending to Claude
