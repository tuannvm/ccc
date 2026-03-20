# Tmux Pane Architecture

## Overview

The tmux structure mirrors the Telegram hierarchy for complete visibility into the multi-bot collaboration.

## Hierarchy Mapping

```
Telegram                   Tmux
─────────────────────────────────────────
Group                      Session (ccc-sessions)
└── Topic                  └── Window (topic name)
    └── Planner role          └── Pane 0 (left)
    └── Executor role         └── Pane 1 (middle)
    └── Reviewer role         └── Pane 2 (right)
```

## Visual Layout

```
┌─────────────────────────────────────────────────────────────────────┐
│                         TMUX SESSION: ccc                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  WINDOW: feature-api-development (Telegram Topic ID: 12345)   │  │
│  ├──────────────┬──────────────┬──────────────────────────────────┤  │
│  │              │              │                                  │  │
│  │   PANE 0     │   PANE 1     │           PANE 2                 │  │
│  │   Planner    │   Executor   │          Reviewer               │  │
│  │              │              │                                  │  │
│  │ @planner     │ @executor    │     @reviewer                    │  │
│  │ working...   │ running...   │     analyzing...                 │  │
│  │              │              │                                  │  │
│  │ Planning     │ Executing    │     Reviewing                    │  │
│  │ steps for    │ git clone    │     /path/to/file                │  │
│  │ REST API     │              │                                  │  │
│  │              │              │                                  │  │
│  │ $            │ $            │     $                            │  │
│  └──────────────┴──────────────┴──────────────────────────────────┘  │
│                                                                       │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  WINDOW: bugfix-auth-flow (Telegram Topic ID: 12346)           │  │
│  ├──────────────┬──────────────┬──────────────────────────────────┤  │
│  │   Planner    │   Executor   │          Reviewer               │  │
│  │              │              │                                  │  │
│  │ Analyzing    │              │     LGTM! ✓                      │  │
│  │ auth issue   │              │                                  │  │
│  │              │              │                                  │  │
│  └──────────────┴──────────────┴──────────────────────────────────┘  │
│                                                                       │
└─────────────────────────────────────────────────────────────────────┘
```

## Pane Responsibilities

### Pane 0 (Left) - Planner
- Receives and processes planning requests
- Creates structured plans
- Delegates to executor via @mention
- Shows planning context and history

### Pane 1 (Middle) - Executor
- Receives tasks from planner
- Executes code changes
- Runs commands and tests
- Shows working directory and git status

### Pane 2 (Right) - Reviewer
- Reviews changes from executor
- Provides feedback
- Shows code diffs and analysis
- Can request fixes from executor

## Inter-Pane Communication

```
┌─────────────────────────────────────────────────────────────────┐
│                    Telegram Topic (High Level)                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Planner: "Plan created. @executor implement steps 1-3"   │  │
│  │ Executor: "Done. @reviewer please review"                │  │
│  │ Reviewer: "Found 2 issues. @executor please fix"         │  │
│  │ Executor: "Fixed! @reviewer please verify"               │  │
│  │ Reviewer: "LGTM! ✓"                                      │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                            │
                            │ Same context, different view
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Tmux Window (Low Level)                       │
│  ┌────────────┬────────────┬────────────────┐                   │
│  │  Planner   │  Executor  │   Reviewer     │                   │
│  │            │            │                │                   │
│  │ Reading    │ Executing  │ Analyzing      │                   │
│  │ @executor  │ git clone  │ diff...        │                   │
│  │ mention    │            │                │                   │
│  │            │            │                │                   │
│  │ Switching  │ Running    │ Preparing      │                   │
│  │ to next    │ tests...   │ feedback...    │                   │
│  │ task...    │            │                │                   │
│  └────────────┴────────────┴────────────────┘                   │
└─────────────────────────────────────────────────────────────────┘
```

## Go Implementation

### Window Structure

```go
// Tmux window represents a Telegram topic with 3 panes
type TmuxWindow struct {
    SessionName string   // "ccc-sessions"
    WindowName  string   // Topic-safe name (e.g., "feature-api-development")
    TopicID     int64    // Telegram topic ID
    Panes       []*TmuxPane
}

type TmuxPane struct {
    PaneID      int      // 0, 1, or 2
    Role        string   // "planner", "executor", "reviewer"
    BotUsername string   // "@planner_bot", etc.
    PaneIndex   int      // Tmux pane index (0, 1, 2)
    WorkingDir  string   // Each pane may have different working directory
}

// Create window with 3 panes for a topic
func CreateTopicWindow(topicID int64, topicName string) (*TmuxWindow, error) {
    windowName := tmuxSafeName(topicName)

    // Create new window with 3 panes
    cmd := exec.Command(tmuxPath, "new-window", "-n", windowName, "-t", cccSessionName)
    if err := cmd.Run(); err != nil {
        return nil, err
    }

    window := &TmuxWindow{
        SessionName: cccSessionName,
        WindowName:  windowName,
        TopicID:     topicID,
        Panes:       make([]*TmuxPane, 3),
    }

    // Configure 3 panes: Planner | Executor | Reviewer
    target := cccSessionName + ":" + windowName

    // Pane 0 (left): Planner - keep as is
    window.Panes[0] = &TmuxPane{
        PaneID:      0,
        Role:        "planner",
        BotUsername: config.BotUsernames["planner"],
        PaneIndex:   0,
        WorkingDir:  sharedWorkDir,
    }

    // Pane 1 (middle): Executor - split vertical
    exec.Command(tmuxPath, "split-window", "-h", "-t", target).Run()
    window.Panes[1] = &TmuxPane{
        PaneID:      1,
        Role:        "executor",
        BotUsername: config.BotUsernames["executor"],
        PaneIndex:   1,
        WorkingDir:  sharedWorkDir,
    }

    // Pane 2 (right): Reviewer - split vertical again
    exec.Command(tmuxPath, "select-pane", "-t", target + ".1").Run()
    exec.Command(tmuxPath, "split-window", "-h", "-t", target).Run()
    window.Panes[2] = &TmuxPane{
        PaneID:      2,
        Role:        "reviewer",
        BotUsername: config.BotUsernames["reviewer"],
        PaneIndex:   2,
        WorkingDir:  sharedWorkDir,
    }

    // Set pane titles
    for i, pane := range window.Panes {
        paneTarget := fmt.Sprintf("%s.%d", target, i)
        title := fmt.Sprintf("[%s] %s", pane.Role, pane.BotUsername)
        exec.Command(tmuxPath, "select-pane", "-t", paneTarget, "-T", title).Run()
    }

    // Equalize pane sizes
    exec.Command(tmuxPath, "select-layout", "-t", target, "even-horizontal").Run()

    return window, nil
}
```

### Message Routing to Panes

```go
// Send message to specific role's pane
func (w *TmuxWindow) SendToPane(role string, message string) error {
    var pane *TmuxPane
    for _, p := range w.Panes {
        if p.Role == role {
            pane = p
            break
        }
    }

    if pane == nil {
        return fmt.Errorf("no pane found for role: %s", role)
    }

    target := fmt.Sprintf("%s:%s.%d", w.SessionName, w.WindowName, pane.PaneIndex)
    return sendToTmuxPane(target, message)
}

// Switch to specific pane for interactive use
func (w *TmuxWindow) SwitchToPane(role string) error {
    var pane *TmuxPane
    for _, p := range w.Panes {
        if p.Role == role {
            pane = p
            break
        }
    }

    target := fmt.Sprintf("%s:%s.%d", w.SessionName, w.WindowName, pane.PaneIndex)
    return exec.Command(tmuxPath, "select-pane", "-t", target).Run()
}
```

### Topic Lifecycle

```go
// Track active topic windows
var topicWindows = make(map[int64]*TmuxWindow)
var topicWindowsMutex sync.RWMutex

// Get or create window for topic
func GetOrCreateTopicWindow(topicID int64, topicName string) (*TmuxWindow, error) {
    topicWindowsMutex.Lock()
    defer topicWindowsMutex.Unlock()

    // Return existing if available
    if window, exists := topicWindows[topicID]; exists {
        return window, nil
    }

    // Create new window
    window, err := CreateTopicWindow(topicID, topicName)
    if err != nil {
        return nil, err
    }

    topicWindows[topicID] = window
    return window, nil
}

// Clean up window when topic is deleted
func DeleteTopicWindow(topicID int64) error {
    topicWindowsMutex.Lock()
    defer topicWindowsMutex.Unlock()

    window, exists := topicWindows[topicID]
    if !exists {
        return nil
    }

    target := fmt.Sprintf("%s:%s", window.SessionName, window.WindowName)
    if err := exec.Command(tmuxPath, "kill-window", "-t", target).Run(); err != nil {
        return err
    }

    delete(topicWindows, topicID)
    return nil
}
```

## Integration with Router

```go
// Enhanced router with tmux pane awareness
type MessageRouter struct {
    config          *Config
    stateManager    *ConversationStateManager
    roleHandlers    map[string]RoleHandler
    topicWindows    map[int64]*TmuxWindow
}

func (r *MessageRouter) routeUpdate(update TelegramUpdate) error {
    msg := update.Message
    sourceRole := update.SourceRole

    // Get or create tmux window for this topic
    window, err := GetOrCreateTopicWindow(msg.MessageThreadID, getSessionName(msg.MessageThreadID))
    if err != nil {
        return err
    }

    // Route to appropriate handler
    handler := r.roleHandlers[sourceRole]

    // Send message to the role's pane
    window.SendToPane(sourceRole, msg.Text)

    // Handle the message
    return handler.Handle(msg)
}
```

## User Experience

### High-Level View (Telegram)
```
You see the conversation flow:
@planner_bot: "Here's my plan..."
@executor_bot: "Implementing..."
@reviewer_bot: "Review complete..."
```

### Low-Level View (Tmux)
```
You can drill down to see details:
- Pane 0: See planner's thinking process
- Pane 1: See executor's terminal output
- Pane 2: See reviewer's analysis and diffs
```

### Switching Between Views

```bash
# In tmux, switch panes:
Ctrl+B, Left Arrow   # Switch to Planner pane
Ctrl+B, Down Arrow   # Switch to Executor pane
Ctrl+B, Right Arrow  # Switch to Reviewer pane

# Or use pane numbers:
Ctrl+B, q, then 0/1/2

# Zoom into a pane (temporarily make it full window):
Ctrl+B, z
# Press again to unzoom
```

## Benefits

1. **Parallel Visibility**: See all 3 bots working simultaneously
2. **Debugging**: Each bot's session is isolated but visible
3. **Context Switching**: Easy to jump between high-level (Telegram) and low-level (tmux)
4. **Audit Trail**: Each pane maintains its own history
5. **Interactive Intervention**: Can type into any pane to guide specific bot

## Configuration

```json
{
  "tmux": {
    "session_name": "ccc-sessions",
    "pane_layout": "even-horizontal",
    "pane_titles": true
  },
  "bot_tokens": {
    "planner": "...",
    "executor": "...",
    "reviewer": "..."
  },
  "bot_usernames": {
    "planner": "your_planner_bot",
    "executor": "your_executor_bot",
    "reviewer": "your_reviewer_bot"
  }
}
```

## Window Commands

```bash
# List all active topic windows
ccc list-topics

# Switch to specific topic window
ccc attach-topic <topic-name>

# Send command to specific role's pane
ccc send-to <topic-name> <role> "<command>"

# Example: send to executor pane
ccc send-to feature-api executor "git status"
```

## Inter-Pane @Mentions (Local Claude Sessions)

When working directly in tmux panes with local Claude sessions, you can use @mentions to route messages between panes—mirroring the Telegram bot behavior.

### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│  TMUX WINDOW: ccc-feature-api                                    │
│  ┌──────────────┬──────────────┬──────────────────────────────┐ │
│  │ Planner Pane │ Executor Pane│       Reviewer Pane          │ │
│  │ (pane 0)     │ (pane 1)     │        (pane 2)              │ │
│  │              │              │                              │ │
│  │ You type:    │              │                              │ │
│  │ "@executor   │              │                              │ │
│  │  implement   │              │                              │ │
│  │  the API"    │              │                              │ │
│  │      │       │              │                              │ │
│  │      └───────┴──────────────┴──► Message routed to pane 1  │ │
│  │                                  │                         │ │
│  │                                  ▼                         │ │
│  │                     "implement the API"                    │ │
│  │                          appears here                       │ │
│  └──────────────┴──────────────┴──────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Implementation Approach

**Option A: CCC Hooks** (Recommended)

CCC implements inter-pane routing via built-in hooks, following CCC's existing pattern:

```json
// .claude/settings.local.json (auto-generated by CCC)
{
  "hooks": {
    "PostResponse": [
      {
        "hooks": [
          {
            "command": "/home/tuannvm/bin/ccc hook-post-response",
            "type": "command"
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "hooks": [
          {
            "command": "/home/tuannvm/bin/ccc hook-session-start",
            "type": "command"
          }
        ]
      }
    ]
  }
}
```

**How CCC implements the hooks:**

```go
// In hooks.go - add these new handlers

func handlePostResponseHook() error {
    // Detect @mentions in Claude's response
    // Route to target pane via shared inbox
    // Follow same pattern as handleStopHook, handleNotificationHook, etc.
}

func handleSessionStartHook() error {
    // Start background listener for incoming inter-pane messages
    // Poll inbox and display when messages arrive
}
```

**Option B: Manual tmux send-keys** (CLI Commands)

Use `tmux send-keys` to route messages:

```bash
# From planner pane (0), send to executor pane (1)
ccc send-to-pane ccc-feature-api 1 "implement the API"

# Or use tmux directly
tmux send-keys -t ccc-feature-api.1 "implement the API" C-m
```

### Usage

In any pane, type:
```
@executor implement the REST API
@reviewer please review my changes  
@planner help me refine this plan
```

The message appears in the target pane automatically.

### Limitations

- Only works in tmux windows named `ccc-*`
- Pane indexing must be: 0=planner, 1=executor, 2=reviewer
- Hooks-based approach requires Claude Code settings configuration
- Manual approach requires explicit tmux commands


### CCC Implementation: Automatic Hook Setup

**IMPORTANT**: CCC should create and configure hooks in `.claude/settings.local.json` following CCC's existing hook pattern.

When CCC creates a multi-bot window, it should:

1. **Create hook scripts** in the project directory:
   ```
   .claude/hooks/
   ├── postresponse-interpane-mention.sh
   └── sessionstart-interpane-listener.sh
   ```

2. **Add hooks to** `.claude/settings.local.json` using CCC's format:

   ```json
   {
     "hooks": {
       "PostResponse": [
         {
           "hooks": [
             {
               "command": "/home/tuannvm/bin/ccc hook-post-response",
               "type": "command"
             }
           ]
         }
       ],
       "SessionStart": [
         {
           "hooks": [
             {
               "command": "/home/tuannvm/bin/ccc hook-session-start",
               "type": "command"
             }
           ]
         }
       ]
     }
   }
   ```

3. **CCC implements the hook commands** in `hooks.go`:

   ```go
   // Handle PostResponse - detect @mentions and route to other panes
   func handlePostResponseHook() error {
       // ... implementation ...
   }

   // Handle SessionStart - start listener for incoming messages
   func handleSessionStartHook() error {
       // ... implementation ...
   }
   ```

**Why CCC implements hooks (not shell scripts)?**
- CCC already handles all other hooks via `ccc hook-*` commands
- Consistent with CCC's architecture
- Single binary, no external script dependencies
- Cross-platform (no bash assumptions)

**Hook variables available:**
- `TMUX` - Set when in tmux
- `TMUX_PANE` - Current pane ID
- Current working directory - project root

**Implementation:**

The inter-pane routing logic should be added to CCC's existing `hooks.go`:

```go
// Add to handlePostResponseHook() - after existing tool display logic
// Check for @mentions in assistant responses and route to other panes

// Add new handleSessionStartHook() - start background listener
// Polls for incoming messages from other panes
```
