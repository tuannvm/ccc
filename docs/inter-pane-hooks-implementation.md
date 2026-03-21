# Inter-Pane @Mention Hooks - Implementation Action Items

**Document Version:** 1.0
**Date:** 2026-03-20
**Status:** PRE-IMPLEMENTATION

## Overview

Implement two new CCC hooks (`PostResponse`, `SessionStart`) to enable inter-pane @mention routing in tmux multi-bot windows.

**Current CCC Hooks:** PreToolUse, Stop, PostToolUse, UserPromptSubmit, Notification
**New Hooks Needed:** PostResponse, SessionStart

---

## Phase 1: Data Structures & Shared Inbox

### 1.1 Add Inter-Pane Message Types to `types.go`

```go
// InterPaneMessage represents a message sent between tmux panes
type InterPaneMessage struct {
    ID        string    `json:"id"`         // Unique ID for deduplication
    FromPane  int       `json:"from_pane"`  // 0=planner, 1=executor, 2=reviewer
    ToPane    int       `json:"to_pane"`    // Target pane index
    Content   string    `json:"content"`    // Message content (without @mention)
    Timestamp int64     `json:"timestamp"`  // Unix milliseconds
    Read      bool      `json:"read"`       // Whether target pane has read this
}

// PaneInbox stores messages for a specific tmux pane
type PaneInbox struct {
    PaneIndex int                `json:"pane_index"`
    Messages  []InterPaneMessage `json:"messages"`
    mutex     sync.RWMutex
}

// GetUnread returns unread messages
func (p *PaneInbox) GetUnread() []InterPaneMessage {
    p.mutex.RLock()
    defer p.mutex.RUnlock()
    var unread []InterPaneMessage
    for _, m := range p.Messages {
        if !m.Read {
            unread = append(unread, m)
        }
    }
    return unread
}

// MarkRead marks a message as read
func (p *PaneInbox) MarkRead(id string) {
    p.mutex.Lock()
    defer p.mutex.Unlock()
    for i := range p.Messages {
        if p.Messages[i].ID == id {
            p.Messages[i].Read = true
            break
        }
    }
}
```

### 1.2 Shared Inbox Manager in `hooks.go`

```go
// inboxPath returns the path for inter-pane message inbox
func inboxPath(tmuxWindow string) string {
    return filepath.Join(cacheDir(), "interpane-inbox-"+tmuxWindow+".json")
}

// loadInbox loads or creates a PaneInbox for a specific pane
func loadInbox(tmuxWindow string, paneIndex int) *PaneInbox {
    path := inboxPath(tmuxWindow)
    var inboxes map[int]*PaneInbox

    data, err := os.ReadFile(path)
    if err == nil {
        json.Unmarshal(data, &inboxes)
    }

    if inboxes == nil {
        inboxes = make(map[int]*PaneInbox)
    }

    if inboxes[paneIndex] == nil {
        inboxes[paneIndex] = &PaneInbox{
            PaneIndex: paneIndex,
            Messages:  []InterPaneMessage{},
        }
    }

    return inboxes[paneIndex]
}

// saveInbox saves all inboxes for a tmux window
func saveInbox(tmuxWindow string, inboxes map[int]*PaneInbox) error {
    path := inboxPath(tmuxWindow)
    data, err := json.MarshalIndent(inboxes, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(path, data, 0600)
}

// sendMessageToPane sends a message to another pane's inbox
func sendMessageToPane(tmuxWindow string, fromPane, toPane int, content string) error {
    inboxes := make(map[int]*PaneInbox)

    // Load existing inboxes
    path := inboxPath(tmuxWindow)
    data, _ := os.ReadFile(path)
    if len(data) > 0 {
        json.Unmarshal(data, &inboxes)
    }

    // Ensure target inbox exists
    if inboxes[toPane] == nil {
        inboxes[toPane] = &PaneInbox{PaneIndex: toPane, Messages: []InterPaneMessage{}}
    }

    // Add message
    msg := InterPaneMessage{
        ID:        fmt.Sprintf("%d-%d", time.Now().UnixNano(), toPane),
        FromPane:  fromPane,
        ToPane:    toPane,
        Content:   content,
        Timestamp: time.Now().UnixMilli(),
        Read:      false,
    }
    inboxes[toPane].Messages = append(inboxes[toPane].Messages, msg)

    return saveInbox(tmuxWindow, inboxes)
}
```

---

## Phase 2: PostResponse Hook Implementation

### 2.1 Add HookData Fields for PostResponse

**File: `types.go`**

```go
type HookData struct {
    // ... existing fields ...
    AssistantText string `json:"assistant_text"` // NEW: For PostResponse hook
}
```

### 2.2 Implement PostResponse Handler

**File: `hooks.go`**

```go
// handlePostResponseHook detects @mentions and routes to other panes
func handlePostResponseHook() error {
    defer func() { recover() }()

    rawData, _ := readHookStdin()
    if len(rawData) == 0 {
        return nil
    }

    hookData, err := parseHookData(rawData)
    if err != nil || hookData.AssistantText == "" {
        return nil
    }

    // Only run in tmux windows named ccc-*
    tmuxWindow, currentPane := detectTmuxContext()
    if !strings.HasPrefix(tmuxWindow, "ccc-") {
        return nil // Not a multi-bot window
    }

    // Parse @mentions from assistant's response
    mentions := parseMentions(hookData.AssistantText)

    // Route each mention to target pane
    for _, mention := range mentions {
        targetPane := roleToPaneIndex(mention.Role) // "executor" -> 1, etc.
        if targetPane >= 0 && targetPane != currentPane {
            sendMessageToPane(tmuxWindow, currentPane, targetPane, mention.Content)
        }
    }

    return nil
}

// Mention represents an @mention detected in text
type Mention struct {
    Role    string // "planner", "executor", "reviewer"
    Content string // Message content after the @mention
}

// parseMentions extracts @mentions from text
func parseMentions(text string) []Mention {
    var mentions []Mention

    // Pattern: @executor do this, @reviewer please check
    re := regexp.MustCompile(`@(planner|executor|reviewer)[ _]+(.+?)(?:\n|$|@)`)
    matches := re.FindAllStringSubmatch(text, -1)

    for _, match := range matches {
        if len(match) >= 3 {
            mentions = append(mentions, Mention{
                Role:    match[1],
                Content: strings.TrimSpace(match[2]),
            })
        }
    }

    return mentions
}

// roleToPaneIndex converts role name to pane index
func roleToPaneIndex(role string) int {
    switch role {
    case "planner":
        return 0
    case "executor":
        return 1
    case "reviewer":
        return 2
    default:
        return -1
    }
}

// detectTmuxContext detects current tmux window and pane
func detectTmuxContext() (tmuxWindow string, paneIndex int) {
    tmuxWindow = os.Getenv("TMUX_WINDOW_NAME")
    paneStr := os.Getenv("TMUX_PANE")

    // Parse pane index from TMUX_PANE (format: "%window.pane")
    if paneStr != "" {
        parts := strings.Split(paneStr, ".")
        if len(parts) >= 2 {
            fmt.Sscanf(parts[1], "%d", &paneIndex)
        }
    }

    return tmuxWindow, paneIndex
}
```

### 2.3 Register PostResponse Hook Command

**File: `main.go`**

Add to the switch statement:

```go
case "hook-post-response":
    if err := handlePostResponseHook(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
```

---

## Phase 3: SessionStart Hook Implementation

### 3.1 Add HookData Fields for SessionStart

**File: `types.go`**

```go
type HookData struct {
    // ... existing fields ...
    // SessionStart uses existing fields (Cwd, SessionID, TranscriptPath)
}
```

### 3.2 Implement SessionStart Handler

**File: `hooks.go`**

```go
// sessionListenerRegistry tracks active background listeners
var sessionListenerRegistry = make(map[string]bool)
var sessionListenerMutex sync.RWMutex

// handleSessionStartHook starts background listener for incoming messages
func handleSessionStartHook() error {
    defer func() { recover() }()

    rawData, _ := readHookStdin()
    if len(rawData) == 0 {
        return nil
    }

    hookData, err := parseHookData(rawData)
    if err != nil {
        return nil
    }

    // Only run in tmux windows named ccc-*
    tmuxWindow, currentPane := detectTmuxContext()
    if !strings.HasPrefix(tmuxWindow, "ccc-") {
        return nil
    }

    sessionKey := fmt.Sprintf("%s:%d", tmuxWindow, currentPane)

    // Check if listener already running
    sessionListenerMutex.RLock()
    exists := sessionListenerRegistry[sessionKey]
    sessionListenerMutex.RUnlock()

    if exists {
        return nil // Already running
    }

    // Start background listener
    sessionListenerMutex.Lock()
    sessionListenerRegistry[sessionKey] = true
    sessionListenerMutex.Unlock()

    go runInboxListener(sessionKey, tmuxWindow, currentPane)

    return nil
}

// runInboxListener polls inbox and displays incoming messages
func runInboxListener(sessionKey, tmuxWindow string, paneIndex int) {
    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()

    lastDisplayedCount := 0

    for {
        select {
        case <-ticker.C:
            // Check if session still active
            sessionListenerMutex.RLock()
            active := sessionListenerRegistry[sessionKey]
            sessionListenerMutex.RUnlock()

            if !active {
                return // Session ended, stop listener
            }

            // Check for new messages
            inbox := loadInbox(tmuxWindow, paneIndex)
            unread := inbox.GetUnread()

            if len(unread) > lastDisplayedCount {
                // Display new messages
                for i := lastDisplayedCount; i < len(unread); i++ {
                    msg := unread[i]
                    displayIncomingMessage(msg)
                    inbox.MarkRead(msg.ID)
                }
                saveInbox(tmuxWindow, map[int]*PaneInbox{paneIndex: inbox})
                lastDisplayedCount = len(unread)
            }
        }
    }
}

// displayIncomingMessage shows a message from another pane
func displayIncomingMessage(msg InterPaneMessage) {
    fromRole := paneIndexToRole(msg.FromPane)

    fmt.Printf("\n")
    fmt.Printf("╔─────────────────────────────────────────────────────────────╗\n")
    fmt.Printf("║ 💬 Message from %s%-10s║\n", fromRole, " ")
    fmt.Printf("╠─────────────────────────────────────────────────────────────╣\n")
    fmt.Printf("║ %s%-65s║\n", msg.Content, " ")
    fmt.Printf("╚─────────────────────────────────────────────────────────────╝\n")
    fmt.Printf("\n")
}

// paneIndexToRole converts pane index to role name
func paneIndexToRole(pane int) string {
    switch pane {
    case 0:
        return "planner"
    case 1:
        return "executor"
    case 2:
        return "reviewer"
    default:
        return fmt.Sprintf("pane-%d", pane)
    }
}
```

### 3.3 Register SessionStart Hook Command

**File: `main.go`**

Add to the switch statement:

```go
case "hook-session-start":
    if err := handleSessionStartHook(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
```

---

## Phase 4: Hook Installation

### 4.1 Add New Hooks to cccHooks Map

**File: `hooks.go`** - `installHooksToPath()` function

Add to `cccHooks` map:

```go
cccHooks := map[string][]interface{}{
    // ... existing hooks ...

    "PostResponse": {
        map[string]interface{}{
            "hooks": []interface{}{
                map[string]interface{}{
                    "command": cccPath + " hook-post-response",
                    "type":    "command",
                },
            },
        },
    },
    "SessionStart": {
        map[string]interface{}{
            "hooks": []interface{}{
                map[string]interface{}{
                    "command": cccPath + " hook-session-start",
                    "type":    "command",
                },
            },
        },
    },
}
```

### 4.2 Update Hook Type Lists

Update `allHookTypes` arrays to include new hooks:

```go
allHookTypes := []string{
    "Stop", "Notification", "PermissionRequest", "PostToolUse", "PreToolUse",
    "UserPromptSubmit", "PostResponse", "SessionStart", // NEW
}
```

### 4.3 Update Required Hooks List

**File: `hooks.go`** - `installHooksForProject()` function

```go
requiredHooks := []string{
    "PreToolUse", "Stop", "UserPromptSubmit", "Notification",
    "PostResponse", "SessionStart", // NEW - for inter-pane routing
}
```

---

## Phase 5: Tmux Integration

### 5.1 Detect Tmux Window Name

Ensure tmux window names follow `ccc-*` pattern:

```bash
# When creating multi-bot windows:
tmux new-window -n "ccc-feature-api" -t ccc-sessions
tmux new-window -n "ccc-bugfix-auth" -t ccc-sessions
```

### 5.2 Set Environment Variables

Ensure CCC session sets these environment variables:

```go
// In session creation code
os.Setenv("TMUX_WINDOW_NAME", windowName)     // e.g., "ccc-feature-api"
os.Setenv("TMUX_PANE", paneIndex)            // e.g., "0", "1", "2"
```

### 5.3 Tmux Pane Titles

Set pane titles for clarity:

```bash
tmux select-pane -t ccc-feature-api.0 -T "[Planner] @planner_bot"
tmux select-pane -t ccc-feature-api.1 -T "[Executor] @executor_bot"
tmux select-pane -t ccc-feature-api.2 -T "[Reviewer] @reviewer_bot"
```

---

## Phase 6: Testing

### 6.1 Unit Tests

```go
// hooks_test.go
func TestParseMentions(t *testing.T) {
    tests := []struct {
        input    string
        expected []Mention
    }{
        {
            input:    "@executor implement the API",
            expected: []Mention{{"executor", "implement the API"}},
        },
        {
            input:    "@reviewer please check this\n@executor fix the bug",
            expected: []Mention{
                {"reviewer", "please check this"},
                {"executor", "fix the bug"},
            },
        },
    }

    for _, tt := range tests {
        result := parseMentions(tt.input)
        assert.Equal(t, tt.expected, result)
    }
}

func TestRoleToPaneIndex(t *testing.T) {
    tests := map[string]int{
        "planner":  0,
        "executor": 1,
        "reviewer": 2,
        "unknown":  -1,
    }

    for role, expected := range tests {
        result := roleToPaneIndex(role)
        assert.Equal(t, expected, result)
    }
}
```

### 6.2 Integration Test

```bash
# Setup
cd /tmp/test-multi-bot
mkdir -p .claude
ccc install-hooks

# Create tmux session with 3 panes
tmux new-session -d -s ccc-test -n "ccc-test-window"
tmux split-window -h -t ccc-test
tmux select-pane -t ccc-test:0.1
tmux split-window -h -t ccc-test

# In pane 0 (planner):
echo "@executor write a hello world function"

# Verify pane 1 (executor) receives the message
# Check ~/.cache/ccc/interpane-inbox-ccc-test-window.json
```

### 6.3 Manual Test Checklist

- [ ] PostResponse hook fires after Claude response
- [ ] @mentions are detected and parsed correctly
- [ ] Messages are routed to correct pane inbox
- [ ] SessionStart hook fires on session start
- [ ] Background listener polls inbox
- [ ] Incoming messages are displayed in target pane
- [ ] Only works in `ccc-*` windows
- [ ] Gracefully handles non-tmux sessions
- [ ] Messages are marked as read after display
- [ ] No message duplication

---

## Implementation Order

1. **Phase 1** - Data structures (types.go, hooks.go)
2. **Phase 2** - PostResponse hook (hooks.go, main.go)
3. **Phase 3** - SessionStart hook (hooks.go, main.go)
4. **Phase 4** - Hook installation (hooks.go)
5. **Phase 5** - Tmux integration (tmux setup)
6. **Phase 6** - Testing (hooks_test.go, manual)

---

## Dependencies

- Requires `PostResponse` and `SessionStart` hook support in Claude Code
- These hooks may not exist in current Claude Code - need to verify or request feature

---

## Open Questions

1. **Hook Availability**: Does Claude Code currently support `PostResponse` and `SessionStart` hooks?
   - If not, need to file feature request or find alternative approach

2. **Message Persistence**: Should messages persist across CCC restarts?
   - Current implementation uses file-based persistence

3. **Cleanup**: When should inbox files be cleaned up?
   - Consider: on session end, on tmux window close, or TTL-based

4. **Cross-Session Routing**: Should messages work across different CCC sessions?
   - Current implementation: same tmux window only

---

## References

- Tmux Architecture: `docs/tmux-architecture.md`
- Multi-Bot Design: `docs/multi-bot-design.md`
- CCC Hook Pattern: `hooks.go` lines 934-1061
- HookData Structure: `types.go` lines 150-199
