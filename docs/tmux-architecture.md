# Tmux Pane Architecture (Design Spec)

> **Status**: Design document for a multi-pane tmux architecture.
>
> **Last Updated**: 2026-03-20
> **Implementation Status**: 🟡 **In Progress** - Phases 0-3 Complete, Phases 4-7 Pending
>
> See `docs/final-summary.md` for detailed implementation status.

## Overview

A proposed **`ccc team` subcommand** that creates 3-pane tmux sessions for multi-bot collaboration. Each team session has three role-based panes (Planner | Executor | Reviewer) that work together in the same Telegram topic.

**Key Design Decision**: Team sessions are **completely separate** from standard CCC sessions to avoid conflicts. Users explicitly opt-in by using `ccc team new` instead of `ccc new`.

### Quick Start

```bash
# Create a team session (3 panes: Planner | Executor | Reviewer)
ccc team new feature-api --topic 12345

# In Telegram, route messages to specific panes:
/planner create a plan for the REST API
/executor run the tests
/reviewer check my changes

# Or just send a message (goes to executor by default)
run the tests

# Telegram shows which pane sent each message:
[Planner] Here's my plan...
[Executor] Tests passing!
[Reviewer] LGTM!
```

### Architecture at a Glance

```
Telegram Topic                Tmux Window (created by ccc team new)
──────────────────────────────────────────────────────────────────
┌────────────────────┐
│  /planner <msg>    │───► Pane 1 (Planner) ──► [Planner] response
│  /executor <msg>   │───► Pane 2 (Executor) ──► [Executor] response
│  /reviewer <msg>   │───► Pane 3 (Reviewer) ──► [Reviewer] response
│  <msg> (no cmd)    │───► Pane 2 (default)
└────────────────────┘
```

---

## Current Reality vs. This Design

## Current Reality vs. This Design

| This Document | Current Codebase Reality |
|--------------|--------------------------|
| 3-pane layout (Planner \| Executor \| Reviewer) | Single pane per session |
| `TmuxWindow`, `TmuxPane` types | Uses `SessionInfo` in `config.Sessions` |
| `config.TeamSessions` for team state | Only `config.Sessions` exists |
| `ccc team new` creates 3-pane windows | `ccc new` creates single-pane windows |
| `ccc team *` subcommand family | No `ccc team` subcommand exists |
| Team-specific hooks (3 per session) | Single hook per session |
| `/planner`, `/executor`, `/reviewer` Telegram commands | Not implemented (would conflict with existing flow if not isolated) |

---

## Code Reusability & Extensibility Architecture

### Design Principles

Based on architectural review (Opus, Codex 5.3, Gemini), the following principles guide the implementation:

1. **Maximize code reuse** — Single-pane logic should not be duplicated
2. **Enable easy extension** — Adding 4-pane, grid layouts, or custom roles should not require core changes
3. **Keep existing sessions stable** — Single-pane sessions must work unchanged
4. **Use interfaces and strategy patterns** — Enable pluggable routing and layouts

### Core Abstractions

#### 1. Session Type System

```go
// session_type.go - NEW FILE

type SessionKind string
const (
    SessionKindSingle SessionKind = "single"
    SessionKindTeam   SessionKind = "team"
)

type PaneRole string
const (
    RolePlanner  PaneRole = "planner"
    RoleExecutor PaneRole = "executor"
    RoleReviewer PaneRole = "reviewer"
    RoleStandard PaneRole = "standard" // For single-pane sessions
)

type PaneInfo struct {
    ClaudeSessionID string   `json:"claude_session_id,omitempty"`
    PaneID          string   `json:"pane_id,omitempty"`     // Tmux pane ID (%1, %2)
    Role            PaneRole `json:"role"`
}

type LayoutSpec struct {
    Name   string     // "single", "team-3pane"
    Panes  []PaneSpec // Configurable pane definitions
}

type PaneSpec struct {
    ID         string   // "planner", "executor", "reviewer"
    Index      int      // 0, 1, 2
    DefaultIn  bool     // Is this the default input target?
    Prefixes   []string // ["/planner", "/plan", "@planner"]
}
```

#### 2. Session Runtime Interface

```go
// runtime.go - NEW FILE

type SessionRuntime interface {
    // Create the tmux layout for this session type
    EnsureLayout(session *SessionInfo) error

    // Get tmux target for a specific role
    ResolveRoleTarget(session *SessionInfo, role PaneRole) (string, error)

    // Get default input target
    ResolveDefaultTarget(session *SessionInfo) (string, error)

    // Start/resume Claude in panes
    StartClaude(session *SessionInfo, workDir string, providerName string) error
}

// SinglePaneRuntime - wraps existing logic
type SinglePaneRuntime struct{}

func (r *SinglePaneRuntime) EnsureLayout(session *SessionInfo) error {
    return switchSessionInWindow(session.Name, session.Path,
        session.ProviderName, session.ClaudeSessionID, "", "", false, false)
}

// TeamRuntime - implements 3-pane layout
type TeamRuntime struct {
    Layout LayoutSpec
}

func (r *TeamRuntime) EnsureLayout(session *SessionInfo) error {
    // Create 3-pane layout using tmux split commands
    // Map roles to panes
}
```

#### 3. Message Routing Strategy

```go
// router.go - NEW FILE

// MessageRouter - inbound Telegram messages
type MessageRouter interface {
    RouteMessage(text string, layout LayoutSpec) (PaneRole, string, error)
    // Returns: target role, stripped message, error
}

// SinglePaneRouter - direct routing (no prefixes)
type SinglePaneRouter struct{}

func (r *SinglePaneRouter) RouteMessage(text string, layout LayoutSpec) (PaneRole, string, error) {
    return RoleStandard, text, nil
}

// TeamRouter - prefix-based routing
type TeamRouter struct{}

func (r *TeamRouter) RouteMessage(text string, layout LayoutSpec) (PaneRole, string, error) {
    // Parse prefixes: /planner, /executor, /reviewer
    // Default to executor if no prefix
}
```

#### 4. Hook Routing Strategy

```go
// hook_router.go - NEW FILE

type HookRouter interface {
    RouteHook(hookData HookData, session *SessionInfo) (PaneRole, error)
    // Returns: which role triggered this hook
}

// SinglePaneRouter - returns standard role
type SinglePaneHookRouter struct{}

func (r *SinglePaneHookRouter) RouteHook(hookData HookData, session *SessionInfo) (PaneRole, error) {
    return RoleStandard, nil
}

// TeamRouter - infers role from context
type TeamHookRouter struct{}

func (r *TeamHookRouter) RouteHook(hookData HookData, session *SessionInfo) (PaneRole, error) {
    // Infer role from:
    // 1. Transcript path (contains "planner"/"executor"/"reviewer")
    // 2. Environment variable (CCC_ROLE)
    // 3. Default to executor
}
```

#### 5. Layout Registry

```go
// layout_registry.go - NEW FILE

var BuiltinLayouts = map[string]LayoutSpec{
    "single": {
        Name: "single",
        Panes: []PaneSpec{
            {ID: "standard", Index: 0, DefaultIn: true},
        },
    },
    "team-3pane": {
        Name: "team-3pane",
        Panes: []PaneSpec{
            {ID: "planner", Index: 0, DefaultIn: false,
             Prefixes: []string{"/planner", "/plan", "@planner"}},
            {ID: "executor", Index: 1, DefaultIn: true,
             Prefixes: []string{"/executor", "/exec", "/e", "@executor"}},
            {ID: "reviewer", Index: 2, DefaultIn: false,
             Prefixes: []string{"/reviewer", "/rev", "/r", "@reviewer"}},
        },
    },
    // Future extensibility:
    // "team-4pane": {Name: "team-4pane", ...}
    // "grid-2x2":   {Name: "grid-2x2", ...}
}
```

### Extended SessionInfo Structure

```go
// types.go - MODIFIED

type SessionInfo struct {
    TopicID         int64                 `json:"topic_id"`
    Path            string                `json:"path"`
    ProviderName    string                `json:"provider_name,omitempty"`

    // NEW: Multi-pane support
    Type            SessionKind           `json:"type,omitempty"`         // "single" or "team"
    LayoutName      string                `json:"layout_name,omitempty"`  // "single", "team-3pane"
    DefaultPaneID   string                `json:"default_pane_id,omitempty"`
    Panes           map[PaneRole]*PaneInfo `json:"panes,omitempty"`    // role -> pane info

    // DEPRECATED: Keep for backward compatibility during migration
    ClaudeSessionID string                `json:"claude_session_id,omitempty"`
    WindowID        string                `json:"window_id,omitempty"`

    // ... existing fields ...
}
```

### Benefits of This Architecture

| Concern | Solution |
|---------|----------|
| **Reuse** | Single-pane logic wrapped in `SinglePaneRuntime`, unchanged |
| **Extension** | New layouts = new `LayoutSpec`, no code changes |
| **4-pane?** | Add `LayoutSpec{name: "team-4pane", ...}` to registry |
| **Grid layout?** | Add `LayoutSpec{name: "grid-2x2", ...}` to registry |
| **Custom roles?** | Extend `PaneSpec` with new roles/prefixes |
| **Type safety** | Interfaces enforce contracts, compile-time checking |

### File Organization

```
session/
├── types.go          # SessionKind, PaneRole, PaneInfo, LayoutSpec
├── runtime.go        # SessionRuntime interface + implementations
├── layout.go          # LayoutSpec registry
└── single.go         # SinglePaneRuntime (wraps existing logic)

routing/
├── message.go         # MessageRouter interface + strategies
└── hook.go           # HookRouter interface + strategies

[existing files - REUSE DIRECTLY]
├── tmux.go           # Tmux primitives (sendToTmux, ensureCccSession)
├── telegram.go        # Telegram API (sendMessage, createForumTopic)
├── ledger.go         # Message delivery tracking
├── config_load.go    # Config persistence
└── hooks.go          # Hook infrastructure (install, verify)
```

### Migration Path

1. **Add abstractions** (interfaces, types) — no behavior change
2. **Wrap existing logic** in `SinglePaneRuntime` — no behavior change
3. **Implement `TeamRuntime`** — new code, isolated
4. **Update command handlers** to use routers — incremental
5. **Test single-pane regression** — ensure stability
6. **Deploy team sessions** — opt-in feature

### Extensibility Examples

**Adding a 4-pane layout:**
```go
"team-4pane": {
    Name: "team-4pane",
    Panes: []PaneSpec{
        {ID: "planner", Index: 0, Prefixes: []string{"/p", "/planner"}},
        {ID: "executor", Index: 1, Prefixes: []string{"/e", "/executor"}},
        {ID: "reviewer", Index: 2, Prefixes: []string{"/r", "/reviewer"}},
        {ID: "observer", Index: 3, Prefixes: []string{"/o", "/observer"}},
    },
}
```

**Adding a grid layout:**
```go
"grid-2x2": {
    Name: "grid-2x2",
    Panes: []PaneSpec{
        {ID: "top-left", Index: 0},
        {ID: "top-right", Index: 1},
        {ID: "bottom-left", Index: 2},
        {ID: "bottom-right", Index: 3},
    },
}
```

---

| This Document | Current Codebase Reality |
|--------------|--------------------------|
| 3-pane layout (Planner \| Executor \| Reviewer) | Single pane per session |
| `TmuxWindow`, `TmuxPane` types | Uses `SessionInfo` in `config.Sessions` |
| `config.TeamSessions` for team state | Only `config.Sessions` exists |
| `ccc team new` creates 3-pane windows | `ccc new` creates single-pane windows |
| `ccc team *` subcommand family | No `ccc team` subcommand exists |
| Team-specific hooks (3 per session) | Single hook per session |
| `/planner`, `/executor`, `/reviewer` Telegram commands | Not implemented (would conflict with existing flow if not isolated) |

## Hierarchy Mapping

```
Telegram                   Tmux
─────────────────────────────────────────
Group                      Session (ccc)
└── Topic                  └── Window (topic name)
    └── Planner role          └── Pane 1 (left)
    └── Executor role         └── Pane 2 (middle)
    └── Reviewer role         └── Pane 3 (right)
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

### Pane 1 (Left) - Planner
- Receives and processes planning requests
- Creates structured plans
- Delegates to executor via @mention
- Shows planning context and history

### Pane 2 (Middle) - Executor
- Receives tasks from planner
- Executes code changes
- Runs commands and tests
- Shows working directory and git status

### Pane 3 (Right) - Reviewer
- Reviews changes from executor
- Provides feedback
- Shows code diffs and analysis
- Can request fixes from executor

## Telegram Integration

### Telegram → Tmux Routing

Users send messages via Telegram using slash commands to route to specific panes:

| Command | Target Pane | Example |
|---------|-------------|---------|
| `/planner <message>` | Pane 1 | `/planner create a plan for the API` |
| `/executor <message>` | Pane 2 | `/executor run the tests` |
| `/reviewer <message>` | Pane 3 | `/reviewer check my changes` |
| `<message>` (no command) | Pane 2 (default) | `run the tests` → goes to executor |

**Why executor as default?** Most user actions are execution tasks (run commands, apply changes, etc.).

### Tmux → Telegram Display

When a pane sends a message to Telegram, the pane name is prepended for clarity:

```
Telegram Topic View:
[Planner] I've created a plan for the REST API. @executor please implement.
[Executor] ACK - starting implementation now...
[Executor] @reviewer API endpoints are ready for review.
[Reviewer] LGTM! All endpoints look good.
```

**Pane name format**: `[Planner]`, `[Executor]`, `[Reviewer]`

This makes it immediately clear which pane sent each message, especially in multi-agent conversations.

### Message Flow Diagram

```
Telegram                      Tmux
─────────────────────────────────────────────────────
User types:
  /planner make a plan
       │
       ▼
  CCC routes to pane 0
       │
       ▼
┌──────────────────────────────────────┐
│ Pane 1 (Planner)                     │
│ "make a plan" appears as input      │
│ Claude processes and responds...     │
└──────────────────────────────────────┘
       │
       │ Claude's response sent back to Telegram
       ▼
  Telegram displays:
  [Planner] Here's my plan: ...

─────────────────────────────────────────────────────
User types:
  implement the API (no command = default)
       │
       ▼
  CCC routes to pane 1 (executor, default)
       │
       ▼
┌──────────────────────────────────────┐
│ Pane 2 (Executor)                    │
│ "implement the API" appears          │
│ Claude executes code...              │
└──────────────────────────────────────┘
```

## Inter-Pane Communication (Proposed Design)

### Architecture Decision: Hybrid Approach

After architectural review (Codex 5.3, Gemini 2.5, Opus 4.6), the consensus is:

> **Do NOT rely on SKILL.md instructions alone for message routing.** LLM instruction-following is probabilistic; message delivery must be deterministic.

**Solution**: Hybrid — Claude expresses intent via @mention, CCC's Go code handles delivery.

```
┌─────────────────────────────────────────────────────────────────┐
│                    Claude in Pane 1                             │
│  User: "Implement the API"                                       │
│  Claude: "I'll delegate this. @executor please implement the    │
│           REST API endpoints."                                    │
│      │                                                           │
│      │ CCC Stop Hook fires (existing Claude Code hook)           │
│      │                                                           │
│      ▼                                                           │
│  CCC Go Code:                                                    │
│  1. Parse response for @mentions                                 │
│  2. Extract: role=executor, msg="please implement the API"       │
│  3. Check pane 1 state (capture-pane, look for prompt)           │
│  4. If ready: tmux load-buffer + paste-buffer -t pane.1          │
│  5. If busy: queue message, retry in 10s                         │
│  6. Log delivery attempt                                         │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Pane 1 (Executor)                             │
│  Message appears: "please implement the REST API endpoints"     │
│  Claude processes and begins implementation...                   │
└─────────────────────────────────────────────────────────────────┘
```

### Why Not SKILL.md-Only?

| Issue | SKILL.md-Only | Hybrid (Skill + Go Router) |
|-------|--------------|---------------------------|
| Claude forgets to route | ❌ Message lost | ✅ Hook catches it |
| Wrong pane index | ❌ Goes to wrong bot | ✅ Go validates mapping |
| Shell quoting breaks | ❌ Malformed send-keys | ✅ load-buffer handles safely |
| Target pane busy | ❌ Message lost/corrupted | ✅ Detect, queue, retry |
| No delivery confirmation | ❌ Fire-and-pray | ✅ Log + retry on failure |
| No retry logic | ❌ Silent failure | ✅ Exponential backoff |

### Implementation: CCC Stop Hook

Use the **existing `Stop` hook** (already supported by Claude Code) — not a fictional `PostResponse` hook.

```go
// In hooks.go — extend handleStopHook()

func handleStopHook() error {
    // Read transcript to get Claude's last response
    transcript, err := readTranscript()
    if err != nil {
        return err
    }

    // Parse for @mentions: @executor, @reviewer, @planner
    mentions := parseMentions(transcript)
    if len(mentions) == 0 {
        return nil
    }

    // Route each mention deterministically via Go code
    for _, mention := range mentions {
        if err := routeToPane(mention.Role, mention.Message, mention.Context); err != nil {
            log.Printf("Failed to route to %s: %v", mention.Role, err)
        }
    }

    return nil
}

type Mention struct {
    Role    string // "executor", "reviewer", "planner"
    Message string // The message content after @role
    Context string // Surrounding context for disambiguation
}

func routeToPane(role string, message string, context string) error {
    // Get current window/pane info from tmux
    currentPane := os.Getenv("TMUX_PANE")
    if currentPane == "" {
        return fmt.Errorf("not in tmux")
    }

    // Find target pane by role
    targetPane, err := findPaneByRole(currentPane, role)
    if err != nil {
        return err
    }

    // Check if target pane has active Claude prompt
    if !tmuxPaneHasActiveClaudePrompt(targetPane) {
        // Queue for retry or report busy
        return queueMessage(targetPane, role, message)
    }

    // Safe delivery via tmux buffer (not raw send-keys)
    return sendToPaneSafely(targetPane, message)
}

func sendToPaneSafely(paneID string, message string) error {
    // Use load-buffer + paste-buffer to avoid shell quoting issues
    bufferName := fmt.Sprintf("ccc-msg-%s", randomID())

    // Load message into tmux buffer
    if err := tmux("load-buffer", "-b", bufferName, "-", []byte(message)); err != nil {
        return err
    }

    // Paste buffer into target pane
    if err := tmux("paste-buffer", "-b", bufferName, "-t", paneID, "-d"); err != nil {
        return err
    }

    // Send Enter to submit
    return tmux("send-keys", "-t", paneID, "Enter")
}

func findPaneByRole(currentPane string, role string) (string, error) {
    // Get all panes in current window
    panes, err := tmuxListPanes()
    if err != nil {
        return "", err
    }

    // Role-to-pane-index mapping (stable across sessions)
    roleToIndex := map[string]int{
        "planner":  0,
        "executor": 1,
        "reviewer": 2,
    }

    targetIndex, ok := roleToIndex[role]
    if !ok {
        return "", fmt.Errorf("unknown role: %s", role)
    }

    // Find pane with matching index
    for _, p := range panes {
        if p.Index == targetIndex {
            return p.ID, nil // Use stable pane ID (%12 format), not index
        }
    }

    return "", fmt.Errorf("pane for role %s not found", role)
}
```

### SKILL.md Role (Interface, Not Router)

The skill is simple — tells Claude what to say, not how to route:

```markdown
# Multi-Pane Collaboration

You are working in a multi-pane tmux session with three roles:
- **Pane 1**: @planner — creates plans, delegates work
- **Pane 2**: @executor — executes code, runs commands
- **Pane 3**: @reviewer — reviews changes, provides feedback

## Delegating Work

To delegate work to another role, use @mentions in your response:
- "@executor please implement the REST API"
- "@reviewer please review my changes"
- "@planner help me refine this approach"

**Important**: After you mention another role, CCC will automatically route your message to their pane. Do NOT attempt to send tmux commands manually.

## Receiving Work

When another role delegates to you, you'll receive their message as input. Process it and respond with your work.
```

## Proposed Go Implementation

### Window Structure

```go
// Tmux window represents a Telegram topic with 3 panes
type TmuxWindow struct {
    SessionName string   // "ccc"
    WindowName  string   // Topic-safe name (e.g., "feature-api-development")
    TopicID     int64    // Telegram topic ID
    Panes       [3]*TmuxPane
}

type TmuxPane struct {
    PaneID      int      // 0, 1, or 2
    Role        string   // "planner", "executor", "reviewer"
    PaneIndex   int      // Tmux pane index (0, 1, 2)
    PaneTmuxID  string   // Stable tmux pane ID (e.g., "%12")
    WorkingDir  string   // Shared working directory
    ClaudePID   int      // Claude process ID for health checking
}

// Create window with 3 panes for a topic
func CreateTopicWindow(topicID int64, topicName string) (*TmuxWindow, error) {
    windowName := tmuxSafeName(topicName)

    // Create new window in ccc session
    if err := tmux("new-window", "-n", windowName, "-t", "ccc:"); err != nil {
        return nil, err
    }

    window := &TmuxWindow{
        SessionName: "ccc",
        WindowName:  windowName,
        TopicID:     topicID,
        Panes:       [3]*TmuxPane{},
    }

    target := "ccc:" + windowName

    // Pane 1 (left): Planner - exists by default
    pane0Info, _ := tmuxCapturePaneInfo(target + ".0")
    window.Panes[0] = &TmuxPane{
        PaneID:     0,
        Role:       "planner",
        PaneIndex:  0,
        PaneTmuxID: pane0Info.ID,
        WorkingDir: sharedWorkDir,
    }

    // Pane 2 (middle): Executor - split vertical
    tmux("split-window", "-h", "-t", target)
    pane1Info, _ := tmuxCapturePaneInfo(target + ".1")
    window.Panes[1] = &TmuxPane{
        PaneID:     1,
        Role:       "executor",
        PaneIndex:  1,
        PaneTmuxID: pane1Info.ID,
        WorkingDir: sharedWorkDir,
    }

    // Pane 3 (right): Reviewer - split from pane 1
    tmux("select-pane", "-t", target + ".1")
    tmux("split-window", "-h", "-t", target)
    pane2Info, _ := tmuxCapturePaneInfo(target + ".2")
    window.Panes[2] = &TmuxPane{
        PaneID:     2,
        Role:       "reviewer",
        PaneIndex:  2,
        PaneTmuxID: pane2Info.ID,
        WorkingDir: sharedWorkDir,
    }

    // Equalize pane sizes
    tmux("select-layout", "-t", target, "even-horizontal")

    return window, nil
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

    if window, exists := topicWindows[topicID]; exists {
        return window, nil
    }

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
    if err := tmux("kill-window", "-t", target); err != nil {
        return err
    }

    delete(topicWindows, topicID)
    return nil
}
```

## User Experience

### High-Level View (Telegram)

Users interact via Telegram using slash commands to route messages:

```
Telegram Topic: feature-api-development

User: /planner create a plan for the REST API
[Planner] Here's my plan:
1. Define data models
2. Create endpoints
3. Add authentication
@executor please implement steps 1-3

User: /reviewer check the implementation
[Reviewer] Looking at the code... LGTM!

User: run the tests (no command = executor)
[Executor] Running tests... All passing!
```

### Low-Level View (Tmux)
```
You can drill down to see details:
- Pane 1: See planner's thinking process
- Pane 2: See executor's terminal output
- Pane 3: See reviewer's analysis and diffs
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

## Proposed Configuration

```json
{
  "tmux": {
    "session_name": "ccc",
    "pane_layout": "even-horizontal"
  },
  "team_sessions": {
    "feature-api": {
      "topic_id": 12345,
      "window_name": "feature-api",
      "panes": [
        {
          "role": "planner",
          "pane_index": 0,
          "claude_session_id": "planner-session-abc123",
          "provider_name": "anthropic"
        },
        {
          "role": "executor",
          "pane_index": 1,
          "claude_session_id": "executor-session-def456",
          "provider_name": "anthropic"
        },
        {
          "role": "reviewer",
          "pane_index": 2,
          "claude_session_id": "reviewer-session-ghi789",
          "provider_name": "anthropic"
        }
      ]
    }
  }
}
```

## CLI Commands: Separate `ccc team` Subcommand

The multi-pane architecture is implemented as a **separate `ccc team` subcommand** to avoid conflicts with existing single-pane functionality.

### Team Subcommand Structure

```bash
# Create a new team session (3-pane: Planner | Executor | Reviewer)
ccc team new <name> --topic <topic-id>

# List all active team sessions
ccc team list

# Send a message to a specific role's pane
ccc team send <team-name> <role> "<message>"

# Attach to a specific pane in a team session
ccc team attach <team-name> --role <planner|executor|reviewer>

# Stop a team session
ccc team stop <team-name>

# Delete a team session
ccc team delete <team-name>
```

### Examples

```bash
# Create a new team session for API development
ccc team new feature-api --topic 12345

# Send a message directly to the executor pane
ccc team send feature-api executor "run the tests"

# Attach to the planner pane to interact directly
ccc team attach feature-api --role planner

# List all active teams
ccc team list
# Output:
# feature-api (topic: 12345) - [Planner] [Executor] [Reviewer]
# bugfix-auth (topic: 12346) - [Planner] [Executor] [Reviewer]
```

### Separation from Standard CCC

| Aspect | Standard CCC (`ccc new`, `ccc worktree`) | Team CCC (`ccc team new`) |
|--------|-------------------------------------------|----------------------------|
| **Pane layout** | Single pane | 3 panes (Planner \| Executor \| Reviewer) |
| **Session tracking** | `config.Sessions` | `config.TeamSessions` |
| **Telegram routing** | Direct to single pane | Command-based (`/planner`, `/executor`, `/reviewer`) |
| **Hooks** | Single hook install | 3 hook installs (one per pane) |
| **Use case** | Individual work | Multi-agent collaboration |

**Key benefit**: Zero impact on existing CCC functionality. Team sessions are completely separate from standard sessions.

# Send a message to a specific role's pane
ccc send-to <session> <role> "<message>"

# Example: send to executor pane
ccc send-to feature-api executor "git status"

# Attach to a specific pane
ccc attach <session> --role executor
```

## Telegram Commands (Team Sessions Only)

The routing commands (`/planner`, `/executor`, `/reviewer`) **only apply to team sessions** created via `ccc team new`. Standard CCC sessions ignore these commands.

### Listen Loop Integration

```go
// In commands.go - listen() function

// Parse incoming Telegram message
text := update.Message.Text
topicID := update.Message.MessageThreadID

// Check if this is a team session
teamSession, isTeam := config.TeamSessions[topicID]

if isTeam {
    // Team session: parse routing commands
    var targetPane int
    var message string

    // Use fields for proper parsing
    fields := strings.Fields(text)
    if len(fields) == 0 {
        return // Empty message
    }

    switch strings.ToLower(fields[0]) {
    case "/planner":
        targetPane = 0
        message = strings.Join(fields[1:], " ")
    case "/executor":
        targetPane = 1
        message = strings.Join(fields[1:], " ")
    case "/reviewer":
        targetPane = 2
        message = strings.Join(fields[1:], " ")
    default:
        // No command = default to executor (pane 1)
        targetPane = 1
        message = text
    }

    // Route message to target pane via tmux
    if err := sendToPane(teamSession, targetPane, message); err != nil {
        log.Printf("Failed to route to pane %d: %v", targetPane, err)
    }
    return // Handled as team message
}

// Standard session: use existing single-pane logic
// ... (existing /new, /worktree, /continue handling)
```

### Key Benefits of Separation

1. **No conflicts**: Standard CCC commands (`/new`, `/worktree`, `/delete`) work unchanged
2. **Clean routing**: Team commands are only active in team sessions
3. **Easy migration**: Users opt-in to team sessions via `ccc team new`
4. **Separate config**: `config.Sessions` for standard, `config.TeamSessions` for teams

### Outgoing Message Format (Tmux → Telegram)

When sending from tmux to Telegram, prepend the pane name:

```go
func sendToTelegram(topicID int64, paneIndex int, message string) error {
    // Get pane name from index
    paneNames := []string{"[Planner]", "[Executor]", "[Reviewer]"}
    if paneIndex < 0 || paneIndex >= len(paneNames) {
        return fmt.Errorf("invalid pane index: %d", paneIndex)
    }

    // Prepend pane name to message
    formattedMessage := fmt.Sprintf("%s %s", paneNames[paneIndex], message)

    // Send to Telegram topic
    return telegram.SendMessage(topicID, formattedMessage)
}
```

## Message Routing Protocol

### Message Format

```
MSG <msgid> <from_role> <to_role> <timestamp>
<message_body>
/MSG
```

### Delivery Semantics

- **At-least-once delivery**: Retry until ack or timeout
- **Idempotent processing**: Duplicate msgid ignored
- **Busy-wait with backoff**: If target not ready, retry with exponential backoff
- **Delivery confirmation**: Log each attempt; success/failure

### Failure Handling

| Failure Mode | Handling |
|-------------|----------|
| Target pane doesn't exist | Fatal error, report to user |
| Target pane mid-execution | Queue message, retry in 10s |
| Target pane crashed | Report error, mark session unhealthy |
| Send command fails | Retry up to 3 times, then escalate |

## Implementation Phases

### Phase 0: Code Reorganization (NEW)
- [ ] Create `session/` package directory
- [ ] Create `routing/` package directory
- [ ] Move `SessionInfo` from `types.go` to `session/types.go`
- [ ] Move tmux utilities to `session/tmux.go` (reuse existing, no logic changes)
- [ ] Create placeholder files for new abstractions (leave empty/TODO)
- [ ] Verify all existing imports still work
- [ ] Run `ccc new` and `ccc list` to confirm single-pane sessions unchanged

**Why**: Refactoring first isolates changes and makes diffs easier to review. Adding new packages after reorganization is cleaner than mixing refactor + new code.

### Phase 1A: Data Model
- [ ] Add `session/session_type.go` with `SessionKind`, `PaneRole`, `PaneInfo`, `LayoutSpec`, `PaneSpec`
- [ ] Extend `SessionInfo` in `session/types.go` with multi-pane fields (`Type`, `LayoutName`, `DefaultPaneID`, `Panes map[PaneRole]*PaneInfo`)
- [ ] Add backward compatibility fields (`ClaudeSessionID`, `WindowID` deprecated but kept)
- [ ] Create `session/layout_registry.go` with `BuiltinLayouts` map
- [ ] Add JSON serialization tests for new types

### Phase 1B: Config Migration & Compatibility
- [ ] Extend `config.go` to add `TeamSessions map[int64]*SessionInfo` (separate from `Sessions`)
- [ ] Add `Config.IsTeamSession(topicID int64) bool` helper
- [ ] Implement config migration: existing `Sessions` remain unchanged
- [ ] Add validation: TeamSession must have exactly 3 panes for team-3pane layout
- [ ] Test: Load existing config, verify no data loss
- [ ] Test: Create team session config, save/load cycle preserves structure

### Phase 2A: Single-Pane Runtime Wrapper
- [ ] Create `session/runtime.go` with `SessionRuntime` interface
- [ ] Implement `SinglePaneRuntime` wrapping existing `switchSessionInWindow()`
- [ ] Add runtime registry: `map[SessionKind]SessionRuntime`
- [ ] Register single-pane runtime as default
- [ ] Unit test: SinglePaneRuntime.EnsureLayout() calls existing logic

### Phase 2B: Multi-Pane Team Runtime
- [ ] Implement `TeamRuntime` in `session/runtime.go`
- [ ] `EnsureLayout()`: Create 3-pane tmux layout (split-window commands)
- [ ] `ResolveRoleTarget()`: Map PaneRole → tmux pane target (e.g., `:.0`, `:.1`, `:.2`)
- [ ] `ResolveDefaultTarget()`: Return executor pane (role: `RoleExecutor`)
- [ ] `StartClaude()`: Launch Claude in each pane with `CCC_ROLE` env var set
- [ ] Unit test: Mock tmux commands, verify layout creation sequence
- [ ] Integration test: Create actual tmux window, verify 3 panes exist

### Phase 3A: Message Routing
- [ ] Create `routing/message.go` with `MessageRouter` interface
- [ ] Implement `SinglePaneRouter` (returns `RoleStandard`, passthrough text)
- [ ] Implement `TeamRouter` (parse `/planner`, `/executor`, `/reviewer` prefixes; default to executor)
- [ ] Add prefix registry (map prefix → PaneRole for extensibility)
- [ ] Unit test: Parse message with `/planner` prefix → returns `RolePlanner`, stripped message
- [ ] Unit test: Message without prefix → returns `RoleExecutor`, full message
- [ ] Unit test: Invalid prefix → error

### Phase 3B: Hook Routing
- [ ] Create `routing/hook.go` with `HookRouter` interface
- [ ] Implement `SinglePaneHookRouter` (returns `RoleStandard`)
- [ ] Implement `TeamHookRouter`:
  - Infer role from transcript path (`*planner.jsonl` → `RolePlanner`)
  - Infer role from `CCC_ROLE` env var
  - Default to `RoleExecutor` if inference fails
- [ ] Unit test: Parse transcript path, extract role correctly
- [ ] Unit test: Env var `CCC_ROLE=reviewer` → returns `RoleReviewer`
- [ ] Unit test: No context available → returns `RoleExecutor` (default)

### Phase 4: Team CLI Commands
- [ ] Add `commands/team.go` for `ccc team` subcommand
- [ ] `ccc team new <name> --topic <id>`: Create `TeamRuntime` session, register in `TeamSessions`
- [ ] `ccc team list`: List all team sessions with pane status
- [ ] `ccc team attach <name> [--role planner|executor|reviewer]`: Attach to specific pane
- [ ] `ccc team stop <name>`: Kill all 3 Claude processes, keep tmux window
- [ ] `ccc team delete <name>`: Kill tmux window, remove from `TeamSessions`
- [ ] Integration test: Create team session, verify 3 panes in tmux

### Phase 5: Telegram Integration (Team Sessions)
- [ ] Update `listen()` in `telegram.go` to check `Config.IsTeamSession(topicID)`
- [ ] If team session: Use `TeamRouter` to parse message, get target role
- [ ] Resolve target pane: `TeamRuntime.ResolveRoleTarget(role)`
- [ ] Send message to pane via existing `sendToTmux()` (reuse)
- [ ] If standard session: Use existing `SinglePaneRouter` (no behavior change)
- [ ] Update outgoing messages: Prepend role prefix `[Planner]`, `[Executor]`, `[Reviewer]`
- [ ] Integration test: Send `/planner test` via Telegram, verify appears in pane 0
- [ ] Integration test: Send message without prefix, verify appears in pane 1 (executor)

### Phase 6: Inter-Pane Communication (Stop Hook Extension)
- [ ] Extend `handleStopHook()` in `hooks.go`
- [ ] Parse transcript for `@planner`, `@executor`, `@reviewer` mentions
- [ ] For team sessions: Use `TeamHookRouter` to infer source role
- [ ] Route to target pane via tmux buffer (`load-buffer` + `paste-buffer`, not `send-keys`)
- [ ] Add retry queue: If target pane busy (no active prompt), queue message, retry in 10s
- [ ] Add hop count limit: Drop messages after 5 hops to prevent loops
- [ ] Log each delivery attempt to ledger
- [ ] Integration test: @executor mention triggers delivery to pane 1

### Phase 7: Ledger & State Tracking
- [ ] Update ledger strategy for multi-pane sessions
- [ ] Ledger key format: `{session-name}-{role}` (e.g., `feature-api-planner`)
- [ ] Separate ledger file per pane: `feature-api-planner.jsonl`, `feature-api-executor.jsonl`, `feature-api-reviewer.jsonl`
- [ ] Update `markDelivered()` to use pane-specific ledger keys
- [ ] Add undelivered message queue to session state
- [ ] Persist queue to config: `SessionInfo.UndeliveredMessages []QueuedMessage`
- [ ] Test: Send message while pane busy → queued → delivered after pane ready

### Phase 8: Comprehensive Testing
- [ ] **Unit tests** (already added in earlier phases): Layout creation, routing, config migration
- [ ] **Regression tests**: Verify all existing `ccc` commands work unchanged
- [ ] **Integration tests**: Full team session lifecycle (create → send messages → stop → delete)
- [ ] **End-to-end tests**: Telegram → Tmux → Claude response → Telegram
- [ ] **Load testing**: 3 team sessions simultaneously (9 Claude processes), verify <2s response time
- [ ] **Recovery tests**: Kill CCC, restart, verify team sessions restored from config

### Phase 9: Rollback & Migration
- [ ] Add feature flag: `ccc --enable-team-sessions` (default: false for safety)
- [ ] Document rollback procedure: Disable flag, delete `TeamSessions` from config
- [ ] Add health check: `ccc team doctor` validates tmux layout, pane connectivity
- [ ] Document migration path: Single-pane → Team session (manual opt-in only)

### Phase 10: Documentation
- [ ] Update README with `ccc team` command reference
- [ ] Document extensibility: How to add `team-4pane` or `grid-2x2` layouts
- [ ] Document troubleshooting: Pane stuck, message not delivered, recovery steps
- [ ] Create architecture diagrams (using existing visual layout section)
- [ ] Document ledger partitioning strategy

### Phase 11: Future Extensibility (Optional)
- [ ] Add `team-4pane` layout spec to registry
- [ ] Add `grid-2x2` layout spec (4 panes in 2x2 grid)
- [ ] Support custom layout definitions via user config file
- [ ] Add pane naming customization (user-defined role names)

## Open Questions

1. **Message persistence**: Should undelivered messages persist to disk?
   - **Answer**: Yes — use existing ledger system for undelivered messages. The `MessageRouter` can queue messages that fail due to pane busy state.

2. **Ordering guarantees**: Should messages be delivered in order?
   - **Answer**: Per-pane ordering maintained via pane-specific message queues. Global ordering across panes is not guaranteed (agents work in parallel).

3. **Loop prevention**: How to prevent @mention loops?
   - **Answer**: Hop count limit in message format. `MessageRouter` includes hop count; messages exceeding threshold (e.g., 5 hops) are dropped with warning.

4. **User intervention**: How should manual pane input interact with routed messages?
   - **Answer**: Manual input bypasses routing. User typing directly into a pane is like using terminal directly — no role prefix added to output.

5. **Session recovery**: What happens if tmux crashes but sessions are restorable?
   - **Answer**: `SessionInfo` persists to disk. On CCC restart, `ensureTeamLayout()` recreates tmux layout and restarts Claude processes using saved session IDs.

## Extensibility Answers

The architecture design addresses the core reusability and extensibility concerns:

| Concern | Solution |
|---------|----------|
| **Code duplication** | `SinglePaneRuntime` wraps existing logic unchanged. Team logic is new code. |
| **4-pane layouts** | Add new `LayoutSpec` to registry — no core code changes needed. |
| **Grid layouts** | Add `grid-2x2` layout with 4 pane specs — handled by same `TeamRuntime`. |
| **Custom roles** | Extend `PaneSpec` with custom IDs and prefixes — no routing changes. |
| **Different routing** | Implement new `MessageRouter` strategy — plug into existing interface. |
| **Hook routing** | Implement new `HookRouter` strategy — plug into existing hook infrastructure. |
| **Breaking changes** | None — single-pane sessions continue using existing paths via `SinglePaneRuntime`. |

---

## References

- **Fact-check**: Original design document contained non-existent hooks and types. This revision aligns with actual codebase.
- **Architectural Review #1**: Codex 5.3 (4/10), Gemini 2.5 (3/10), Opus 4.6 (2/10) — all rejected SKILL.md-only approach.
- **Architectural Review #2**: Codex 5.3, Gemini 2.0, Opus — consensus on `ccc team` subcommand separation and SessionRuntime interface pattern for reusability.
- **Existing Patterns**: CCC already has `tmux.go` with pane checking utilities; `hooks.go` with Stop hook handler; `telegram.go` with messaging primitives.
