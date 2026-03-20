# Multi-Bot Telegram Architecture Design

## Overview

A system of 3 specialized AI bots (planner, executor, reviewer) collaborating in a single Telegram topic via @mention routing.

## CRITICAL ARCHITECTURAL DECISION

**Three-Bot Architecture Selected**

After UX analysis, the three-bot approach (`@planner_bot`, `@executor_bot`, `@reviewer_bot`) is selected over single-bot with commands.

### Why Not Single-Bot?

The single-bot approach (`/planner`, `/executor`, `/reviewer`) was evaluated and rejected due to:

1. **Reply Ambiguity**: All responses come from `@your_bot` - user can't tell if planner, executor, or reviewer is speaking
2. **Unqualified Messages**: No clear routing for "fix this bug" - which role handles it?
3. **Hidden State**: One bot pretending to be 3 personas creates confusion
4. **User Friction**: Commands are less natural than @mentions
5. **Failed Core Promise**: The value proposition was "3 specialized bots collaborating" - single bot doesn't deliver that

### Why Three-Bot Works

1. **Clear Identity**: Each bot IS its role - `@planner_bot` = planner, etc.
2. **Visual Clarity**: Different usernames/avatars make roles obvious
3. **Explicit Routing**: @mentions only - no accidental triggering
4. **Natural UX**: Mirrors team communication - "@Jane review this"
5. **Delivers Promise**: Actual 3 specialized bots, not 1 bot with routing

### Telegram Bot Constraint

**You CANNOT use a single bot token for multiple @mention identities.**

Each bot username (@planner_bot, @executor_bot, @reviewer_bot) requires:
1. Its own bot account created via BotFather
2. Its own bot token
3. Separate polling/webhook handling

**Why?**
- Telegram Bot API routes messages based on which bot token receives the @mention
- A bot logged in as `@ccc_orchestrator` CANNOT receive messages for `@planner_bot`
- Each @mention must be registered to a specific bot account

**What this means:**
- You must create 3 separate bot accounts
- The single ccc binary will manage all 3 bot tokens
- Router layer polls 3 separate update streams
- State is shared across all 3 bot identities

### CRITICAL: Bot-to-Bot Visibility Limitation

**⚠️ TELEGRAM API CONSTRAINT: Bots cannot see messages from other bots.**

Regardless of privacy mode settings, **one bot cannot see another bot's messages via the API**.

#### The Problem

```
@planner_bot: "@executor_bot Please implement the API"
                │
                ▼
Telegram API delivers to: ✅ planner_bot (sender)
Telegram API delivers to: ❌ executor_bot (mentioned, but not a user message)
```

**Original design assumption:** When @planner_bot says "@executor_bot do X", the executor would see this via Telegram polling and respond.

**Reality:** The executor never receives that message. Bot-to-bot messages are NOT delivered via the API.

#### The Fix: Internal Event Bus

**Core Principle:** Telegram is the **UI layer**, not the message bus.

```
Human Message → Telegram API → Router → Role Handler
                                                │
                                                ▼
                                    Internal Event Bus (Go channels)
                                                │
                                                ▼
                              Bot-to-Bot Handoff (INTERNAL)
                                                │
                                                ▼
                                  Telegram Echo (for human visibility)
```

**How it works:**

```go
// Internal event structure
type BotEvent struct {
    FromRole string  // "planner", "executor", "reviewer"
    ToRole   string  // "planner", "executor", "reviewer"
    Message  string
    TopicID  int64
    Context  map[string]interface{}
}

type InternalEventBus struct {
    events chan BotEvent
}

// When Planner wants to hand off to Executor:
func (h *PlannerHandler) HandoffToExecutor(topicID int64, task string) {
    // 1. Send to Telegram for HUMAN visibility only
    sendMessage(config, topicID, "@executor_bot Please implement: " + task)

    // 2. ACTUAL handoff via internal bus
    h.eventBus.events <- BotEvent{
        FromRole: "planner",
        ToRole:   "executor",
        Message:  task,
        TopicID:  topicID,
    }
}

// Router listens to BOTH Telegram polling AND internal bus
func (r *MessageRouter) Start() {
    // Poll 3 bot tokens for HUMAN messages
    for role := range r.config.BotTokens {
        go r.pollBot(role)
    }

    // Listen to internal bus for BOT-TO-BOT handoffs
    go r.listenToInternalBus()
}
```

#### Key Implications

| Concern | Without Fix | With Internal Bus |
|---------|------------|-------------------|
| Bot handoffs | ❌ Don't work | ✅ Internal coordination |
| Privacy mode | ❌ Must disable | ✅ Can keep ENABLED |
| Coupling | ❌ Tied to Telegram | ✅ Telegram = UI only |
| Testing | ❌ Hard to test | ✅ Test logic separately |

**Privacy mode benefit:** With internal event bus, you can keep privacy mode ENABLED. Bots still see human @mentions, while bot-to-bot coordination happens internally—much more secure!

#### Architecture Update

The @mentions you see in Telegram are **UI echoes for humans**, not functional triggers. The real coordination happens via internal Go channels.

```
┌─────────────────────────────────────────────────────────────────┐
│                     Telegram Group (Display Layer)               │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  @planner_bot: "Plan created. @executor_bot implement" │   │
│  │  @executor_bot: "Implementing..."                       │   │
│  │  @reviewer_bot: "LGTM!"                                 │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                            │
                            │ Humans see this
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                  CCC Router (The Real System)                  │
│  ┌────────────────────────────────────────────────────────┐   │
│  │           Internal Event Bus (Go channels)              │   │
│  │  ┌──────────────────────────────────────────────────┐   │   │
│  │  │ Planner: "I'm done, handoff to executor"       │   │   │
│  │  │          └─────────────────────────────────────► │   │   │
│  │  │                                                   │   │   │
│  │  │              ┌────────────────────────────────┐ │   │   │
│  │  │              ▼                                │ │   │   │
│  │  │         Executor: "Executing task..."        │ │   │   │
│  │  │              └────────────────────────────────┘ │   │   │
│  │  └──────────────────────────────────────────────────┘   │   │
│  └────────────────────────────────────────────────────────┘   │
│                        │                                       │
│                        ▼                                       │
│  ┌──────────────┬──────────────┬──────────────┐                 │
│  │  Planner     │  Executor    │  Reviewer     │                 │
│  │  Handler     │  Handler     │  Handler     │                 │
│  │  ┌────────┐   │  ┌────────┐   │  ┌────────┐   │                 │
│  │  │ Tmux   │   │  │ Tmux   │   │  │ Tmux   │   │                 │
│  │  │ Pane 0 │   │  │ Pane 1 │   │  │ Pane 2 │   │                 │
│  │  └────────┘   │  └────────┘   │  └────────┘   │                 │
│  └──────────────┴──────────────┴──────────────┘                   │
└─────────────────────────────────────────────────────────────────┘
```

**Source:** Verified against official Telegram Bot API documentation: https://core.telegram.org/bots/features#privacy-mode

## Architecture

### Bot Roles

| Bot | Username | Purpose | Triggers On |
|-----|----------|---------|-------------|
| **Planner** | `@planner_bot` | Creates structured plans, breaks down tasks | `@planner`, direct human messages |
| **Executor** | `@executor_bot` | Executes tasks, implements code | `@executor`, planner handoff |
| **Reviewer** | `@reviewer_bot` | Reviews code, provides feedback | `@reviewer`, executor handoff |

### Interaction Flow

```
Human ──┐
        │
        ▼
    @planner_bot
        │
        │ (plan approved)
        ▼
    @executor_bot ──┐
        │           │
        │           │ (needs review)
        │           ▼
        │      @reviewer_bot ─┐
        │                    │
        └────────────────────┘ (feedback loop)
```

### Tmux Architecture

**Each Telegram topic maps to 1 tmux window with 3 panes:**

```
┌──────────────────────────────────────────────────────────────┐
│  TMUX WINDOW: feature-api-development                         │
├──────────────────┬──────────────────┬────────────────────────┤
│  PANE 0          │  PANE 1          │  PANE 2                │
│  Planner         │  Executor        │  Reviewer              │
│                  │                  │                        │
│  Planning steps  │  Executing code  │  Reviewing changes     │
│  for REST API    │  Running tests   │  Analyzing diffs       │
│                  │                  │                        │
└──────────────────┴──────────────────┴────────────────────────┘
```

**Dual Visibility:**
- **Telegram (High-Level)**: See conversation flow between bots via @mentions
- **Tmux (Low-Level)**: Drill down to see each bot's actual work/session

**Hierarchy Mapping:**
```
Telegram Group   → Tmux Session
  └── Topic      →   └── Window (3 panes: Planner | Executor | Reviewer)
```

**See Also:** `tmux-architecture.md` for complete tmux pane implementation details.

### Message Routing

**Key Rule:** Bots ONLY respond to messages with:
1. Direct @mention (`@planner_bot`, `@executor_bot`, `@reviewer_bot`)
2. Commands prefixed with their username (`/plan@planner_bot`)

**Ignored:** All other messages (including replies from other bots, human messages without @mention)

## Implementation Approaches

### Option A: Single Binary with 3 Bot Tokens (RECOMMENDED)

**CRITICAL:** Each @mention requires its own bot token. A single bot CANNOT receive @mentions meant for other bots.

**Structure:**
```
ccc (single binary)
├── 3 Bot Tokens: One per bot (@planner_bot, @executor_bot, @reviewer_bot)
├── Router: Polls all 3 bots or receives 3 webhooks
├── Role Detection: Each message arrives with its source bot identity
└── Shared State: Common conversation state across all 3 bot instances
```

**How @mention routing works:**
1. User message: "@planner_bot help me build X"
2. Telegram delivers to planner_bot token only
3. Router detects source = planner → activates planner handler
4. Planner response (sent via planner_bot token): "Plan created. @executor_bot please implement steps 1-3"
5. Executor responds when explicitly mentioned with @executor_bot

**Privacy Mode Handling:**
- **With internal event bus**: Privacy mode can stay **ENABLED** ✅
- Bots still receive human @mentions (works with privacy mode)
- Bot-to-bot coordination happens internally via Go channels
- If you need unqualified message handling: Disable privacy in BotFather (`/setprivacy` → "Disable")
- Only use in private groups if privacy is disabled

**Pros:**
- Single deployment, single service
- Shared state/memory across bots
- Easier debugging and monitoring
- True multi-bot identity for users

**Cons:**
- Must create and configure 3 separate bot accounts
- Privacy mode must be disabled (security consideration)
- Username collision if someone takes the name first
- Need to manage 3 bot tokens securely

**Config structure:**
```json
{
  "bot_tokens": {
    "planner": "123:ABC",
    "executor": "456:DEF",
    "reviewer": "789:GHI"
  },
  "bot_usernames": {
    "planner": "your_planner_bot",
    "executor": "your_executor_bot",
    "reviewer": "your_reviewer_bot"
  },
  "privacy_disabled": true,
  "conversation_state": {}
}
```

### Option B: Multiple Bot Instances

**Structure:**
```
ccc-planner   (separate binary)
ccc-executor  (separate binary)
ccc-reviewer  (separate binary)
```

**Pros:**
- True isolation between bots
- Can deploy separately
- Independent scaling

**Cons:**
- 3x deployment complexity
- State coordination challenges
- Higher cost (3 sets of API tokens)
- harder to debug

## Recommended Approach: Option A with Router Pattern

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│                    Telegram Group                        │
│  ┌──────────────────────────────────────────────────┐   │
│  │           Topic: Feature X Development            │   │
│  │                                                    │   │
│  │ Human: @planner_bot build a REST API              │   │
│  │ Planner: Here's the plan... @executor_bot         │   │
│  │ Executor: Implementing... @reviewer_bot           │   │
│  │ Reviewer: Found 3 issues... @executor_bot         │   │
│  │ Executor: Fixed! @reviewer_bot                    │   │
│  │ Reviewer: LGTM! ✓                                  │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│                  CCC Router (Go)                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │            @mention Parser                        │   │
│  │  - Detect @planner_bot, @executor_bot, @reviewer  │   │
│  │  - Route to appropriate role handler             │   │
│  └──────────────────────────────────────────────────┘   │
│                        │                                 │
│         ┌──────────────┼──────────────┐                 │
│         ▼              ▼              ▼                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐             │
│  │ Planner  │  │ Executor │  │ Reviewer │             │
│  │ Handler  │  │ Handler  │  │ Handler  │             │
│  └──────────┘  └──────────┘  └──────────┘             │
│         │              │              │                 │
│         └──────────────┼──────────────┘                 │
│                        ▼                                 │
│              Shared Conversation State                  │
│              - Plan history                              │
│              - Execution context                         │
│              - Review feedback                           │
└─────────────────────────────────────────────────────────┘
                        │
                        ▼
                 Claude Code API
```

### Key Components

#### 1. Router Layer

```go
// Internal event for bot-to-bot coordination
type BotEvent struct {
    FromRole string                    // "planner", "executor", "reviewer"
    ToRole   string                    // "planner", "executor", "reviewer"
    Message  string                    // The task/message
    TopicID  int64                     // Telegram topic ID
    Context  map[string]interface{}    // Additional context
}

// Internal event bus - CRITICAL for bot-to-bot coordination
// Telegram bots CANNOT see each other's messages via API
type InternalEventBus struct {
    events chan BotEvent
}

func NewInternalEventBus() *InternalEventBus {
    return &InternalEventBus{
        events: make(chan BotEvent, 100),
    }
}

// Message router handling 3 bot tokens + internal event bus
type MessageRouter struct {
    config          *Config
    stateManager    *ConversationStateManager
    roleHandlers    map[string]RoleHandler
    updateChannels  map[string]chan TelegramUpdate
    eventBus        *InternalEventBus  // CRITICAL: For bot-to-bot coordination
}

func (r *MessageRouter) Start() {
    // Start polling for all 3 bots (HUMAN messages)
    for role := range r.config.BotTokens {
        go r.pollBot(role)
    }

    // Start listening to internal event bus (BOT-TO-BOT messages)
    go r.listenToInternalBus()
}

func (r *MessageRouter) pollBot(role string) {
    token := r.config.BotTokens[role]
    offset := 0

    for {
        updates, err := getUpdates(token, offset, 30)
        if err != nil {
            time.Sleep(5 * time.Second)
            continue
        }

        for _, update := range updates {
            offset = update.UpdateID + 1
            update.SourceRole = role
            r.routeUpdate(update)
        }
    }
}

// CRITICAL: Listen for internal bot-to-bot events
// This is how bots actually coordinate (Telegram is just UI display)
func (r *MessageRouter) listenToInternalBus() {
    for event := range r.eventBus.events {
        // Get handler for target role
        handler, exists := r.roleHandlers[event.ToRole]
        if !exists {
            log.Printf("No handler for role: %s", event.ToRole)
            continue
        }

        // Handle the internal event
        if internalHandler, ok := handler.(InternalEventHandler); ok {
            err := internalHandler.HandleInternal(event)
            if err != nil {
                log.Printf("Error handling internal event: %v", err)
            }
        }
    }
}

func (r *MessageRouter) routeUpdate(update TelegramUpdate) error {
    msg := update.Message
    if msg == nil {
        return nil
    }

    sourceRole := update.SourceRole
    mentions := extractMentions(msg.Text)

    botUsername := r.config.BotUsernames[sourceRole]
    isMentioned := false
    for _, m := range mentions {
        if m == botUsername || m == sourceRole {
            isMentioned = true
            break
        }
    }

    if !isMentioned {
        return nil
    }

    handler, exists := r.roleHandlers[sourceRole]
    if !exists {
        return fmt.Errorf("no handler for role: %s", sourceRole)
    }

    return handler.Handle(msg)
}

func extractMentions(text string) []string {
    var mentions []string
    re := regexp.MustCompile(`@(\w+)`)
    matches := re.FindAllStringSubmatch(text, -1)
    for _, match := range matches {
        mentions = append(mentions, match[1])
    }
    return mentions
}
```

#### 2. Role Handlers

```go
// Interface for handlers that can receive internal bot-to-bot events
type InternalEventHandler interface {
    HandleInternal(event BotEvent) error
}

type RoleHandler interface {
    Handle(msg TelegramMessage) error
    CanHandle(msg TelegramMessage) bool
}

type PlannerHandler struct {
    state    *ConversationStateManager
    config   *Config
    eventBus *InternalEventBus  // CRITICAL: For bot-to-bot coordination
}

func (h *PlannerHandler) Handle(msg TelegramMessage) error {
    // Planner logic:
    // 1. Analyze request
    // 2. Create structured plan
    // 3. Hand off to executor via INTERNAL BUS
    prompt := msg.Text

    // Remove @planner_bot from prompt
    prompt = strings.ReplaceAll(prompt, "@planner_bot", "")
    prompt = strings.TrimSpace(prompt)

    // Create plan
    response, err := h.createPlan(prompt)
    if err != nil {
        return err
    }

    // Send response to Telegram
    sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, response)

    // If plan approved, hand off to executor
    if h.shouldProceed(msg) {
        planSummary := h.getPlanSummary()

        // 1. Send to Telegram for HUMAN visibility (echo only)
        handoffMsg := fmt.Sprintf("@executor_bot Please implement: %s", planSummary)
        sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, handoffMsg)

        // 2. ACTUAL handoff via internal event bus
        h.eventBus.events <- BotEvent{
            FromRole: "planner",
            ToRole:   "executor",
            Message:  planSummary,
            TopicID:  msg.Chat.ID,
            Context: map[string]interface{}{
                "message_id":    msg.MessageID,
                "thread_id":     msg.MessageThreadID,
                "plan_id":       h.getCurrentPlanID(),
            },
        }
    }

    return nil
}

// CRITICAL: Handle internal events from other bots
func (h *PlannerHandler) HandleInternal(event BotEvent) error {
    // Planner typically doesn't receive internal events
    // It's the entry point for human requests
    log.Printf("Planner received internal event from %s: %s", event.FromRole, event.Message)
    return nil
}

type ExecutorHandler struct {
    state    *ConversationStateManager
    config   *Config
    eventBus *InternalEventBus
}

func (h *ExecutorHandler) Handle(msg TelegramMessage) error {
    // Executor logic when @mentioned by HUMAN
    prompt := strings.ReplaceAll(msg.Text, "@executor_bot", "")
    prompt = strings.TrimSpace(prompt)

    response, err := h.executeTask(prompt)
    if err != nil {
        return err
    }

    sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, response)

    // If code changed, @mention reviewer
    if h.hasCodeChanges() {
        // 1. Send to Telegram for human visibility
        handoffMsg := "@reviewer_bot Please review the changes"
        sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, handoffMsg)

        // 2. ACTUAL handoff via internal bus
        h.eventBus.events <- BotEvent{
            FromRole: "executor",
            ToRole:   "reviewer",
            Message:  "Review requested",
            TopicID:  msg.Chat.ID,
            Context: map[string]interface{}{
                "thread_id":     msg.MessageThreadID,
                "changes":       h.getChangedFiles(),
            },
        }
    }

    return nil
}

// CRITICAL: Handle internal events from planner
func (h *ExecutorHandler) HandleInternal(event BotEvent) error {
    log.Printf("Executor received internal event from %s: %s", event.FromRole, event.Message)

    // This is the REAL trigger - not the Telegram @mention
    // Execute the task from planner
    response, err := h.executeTask(event.Message)
    if err != nil {
        return err
    }

    // Send response to Telegram (using executor bot token)
    threadID := int64(event.Context["thread_id"].(float64))
    sendMessage(h.config, event.TopicID, threadID, response)

    // If code changed, hand off to reviewer
    if h.hasCodeChanges() {
        // Telegram echo for humans
        handoffMsg := "@reviewer_bot Please review the changes"
        sendMessage(h.config, event.TopicID, threadID, handoffMsg)

        // Internal handoff
        h.eventBus.events <- BotEvent{
            FromRole: "executor",
            ToRole:   "reviewer",
            Message:  "Review requested",
            TopicID:  event.TopicID,
            Context:  event.Context,
        }
    }

    return nil
}

type ReviewerHandler struct {
    state    *ConversationStateManager
    config   *Config
    eventBus *InternalEventBus
}

func (h *ReviewerHandler) Handle(msg TelegramMessage) error {
    // Reviewer logic when @mentioned by HUMAN
    prompt := strings.ReplaceAll(msg.Text, "@reviewer_bot", "")
    prompt = strings.TrimSpace(prompt)

    review, err := h.reviewChanges(prompt)
    if err != nil {
        return err
    }

    sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, review)

    // If issues found, @mention executor
    if h.hasIssues() {
        issues := h.getIssues()
        handoffMsg := fmt.Sprintf("@executor_bot Please fix: %s", issues)
        sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, handoffMsg)

        // Internal handoff
        h.eventBus.events <- BotEvent{
            FromRole: "reviewer",
            ToRole:   "executor",
            Message:  issues,
            TopicID:  msg.Chat.ID,
            Context: map[string]interface{}{
                "thread_id": msg.MessageThreadID,
            },
        }
    }

    return nil
}

// CRITICAL: Handle internal events from executor
func (h *ReviewerHandler) HandleInternal(event BotEvent) error {
    log.Printf("Reviewer received internal event from %s: %s", event.FromRole, event.Message)

    // This is the REAL trigger - review the changes
    review, err := h.reviewChanges(event.Message)
    if err != nil {
        return err
    }

    threadID := int64(event.Context["thread_id"].(float64))
    sendMessage(h.config, event.TopicID, threadID, review)

    // If issues found, hand off to executor
    if h.hasIssues() {
        issues := h.getIssues()

        // Telegram echo
        handoffMsg := fmt.Sprintf("@executor_bot Please fix: %s", issues)
        sendMessage(h.config, event.TopicID, threadID, handoffMsg)

        // Internal handoff
        h.eventBus.events <- BotEvent{
            FromRole: "reviewer",
            ToRole:   "executor",
            Message:  issues,
            TopicID:  event.TopicID,
            Context:  event.Context,
        }
    }

    return nil
}
```

#### 3. Conversation State

```go
type ConversationState struct {
    TopicID        int64
    CurrentRole    string
    Plan           *Plan
    ExecutionState *ExecutionState
    ReviewState    *ReviewState
}

type Plan struct {
    ID          string
    Steps       []Step
    CreatedAt   time.Time
    Approved    bool
}

type ExecutionState struct {
    FilesModified []string
    CommandsRun   []string
    Status        string
}

type ReviewState struct {
    Findings   []Finding
    Approved   bool
    ReviewedAt time.Time
}

type ConversationStateManager struct {
    states map[int64]*ConversationState
    mu     sync.RWMutex
}

// GetState returns an immutable copy to prevent race conditions
func (m *ConversationStateManager) GetState(topicID int64) *ConversationState {
    m.mu.RLock()
    defer m.mu.RUnlock()

    state := m.states[topicID]
    if state == nil {
        return nil
    }

    // Return a deep copy to prevent races
    return state.DeepCopy()
}

// UpdateState executes a function under write lock for atomic updates
func (m *ConversationStateManager) UpdateState(topicID int64, fn func(*ConversationState)) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.states[topicID] == nil {
        m.states[topicID] = &ConversationState{TopicID: topicID}
    }

    fn(m.states[topicID])
    return nil
}

// Helper methods for safe state updates
func (m *ConversationStateManager) SetCurrentRole(topicID int64, role string) error {
    return m.UpdateState(topicID, func(s *ConversationState) {
        s.CurrentRole = role
    })
}

func (m *ConversationStateManager) SetPlan(topicID int64, plan *Plan) error {
    return m.UpdateState(topicID, func(s *ConversationState) {
        s.Plan = plan
    })
}

func (m *ConversationStateManager) AddReviewFinding(topicID int64, finding Finding) error {
    return m.UpdateState(topicID, func(s *ConversationState) {
        if s.ReviewState == nil {
            s.ReviewState = &ReviewState{}
        }
        s.ReviewState.Findings = append(s.ReviewState.Findings, finding)
    })
}
```

### Setup Instructions

1. **Create 3 bots via BotFather:**
   ```
   /newbot
   - your_project_planner (token: saved to config as bot_tokens.planner)
   - your_project_executor (token: saved to config as bot_tokens.executor)
   - your_project_reviewer (token: saved to config as bot_tokens.reviewer)
   ```

2. **CRITICAL: Disable privacy mode for all 3 bots:**
   ```
   /setprivacy
   Select: your_project_planner → "Disable"
   /setprivacy
   Select: your_project_executor → "Disable"
   /setprivacy
   Select: your_project_reviewer → "Disable"
   ```
   **Security Note:** Disabling privacy mode allows bots to read ALL group messages.
   Only do this in private, trusted groups. Consider:
   - Using a dedicated private group for development
   - Not adding sensitive channels/groups
   - BotFather can re-enable privacy anytime if needed

3. **Add all 3 bots to the group** as admins
   - All 3 bots need admin permissions to create topics, edit messages, etc.

4. **Configure ccc:**
   ```bash
   ccc setup-multibot
   # Prompts for:
   # - planner_bot token and username
   # - executor_bot token and username
   # - reviewer_bot token and username
   ```

5. **Start collaboration:**
   ```
   Human: @your_project_planner build a REST API for user management

   Planner: I'll create a plan for this...

   [Plan Details]

   @your_project_executor Please implement steps 1-3

   Executor: Implementing...

   [Code changes]

   @your_project_reviewer Please review

   Reviewer: Reviewing...

   [Review results]

   ✓ LGTM!
   ```

## Error Handling

### Bot Disagreements

If bots disagree (e.g., reviewer rejects, executor insists):
- System: Escalates to human with context
- Human: Can @mention specific bot to direct them
- Example: "@executor_bot please address the reviewer's concerns"

### Infinite Loops

Prevent loops between executor and reviewer:
- Max iteration counter per topic (default: 3)
- After N iterations, require human intervention
- State tracking: `ReviewState.IterationCount`

### State Conflicts

Multiple messages in flight:
- Message queue per topic
- Process sequentially per role
- Use `UpdateState()` with atomic operations
- Message ID tracking to prevent duplicate processing

**Concurrency Model:**
```go
// Each topic gets a serial processor
type TopicProcessor struct {
    queue chan TelegramMessage
    state *ConversationStateManager
}

// Messages for same topic are processed sequentially
// Messages for different topics are processed concurrently
func (p *TopicProcessor) Enqueue(msg TelegramMessage) {
    p.queue <- msg
}

func (p *TopicProcessor) Start() {
    for msg := range p.queue {
        p.handleMessage(msg)
    }
}

// Router manages per-topic processors
type Router struct {
    processors map[int64]*TopicProcessor
    mu         sync.RWMutex
}

func (r *Router) Route(msg TelegramMessage) {
    r.mu.Lock()
    topicID := msg.MessageThreadID
    if r.processors[topicID] == nil {
        r.processors[topicID] = &TopicProcessor{
            queue: make(chan TelegramMessage, 100),
            state: r.stateManager,
        }
        go r.processors[topicID].Start()
    }
    r.mu.Unlock()

    r.processors[topicID].Enqueue(msg)
}
```

## Telegram API Considerations

### Privacy Mode (CRITICAL)

**Default behavior:** Bots with privacy enabled only receive:
- Commands starting with `/`
- Messages that explicitly @mention the bot
- Replies to the bot's own messages

**Original concern:** Bots must see cross-bot @mentions (e.g., "@executor_bot do X" from @planner_bot)

**Solution: Internal Event Bus**

With the internal event bus architecture:
- Privacy mode can stay **ENABLED** ✅
- Bots still receive human @mentions (works with privacy mode)
- Bot-to-bot coordination happens **internally** via Go channels
- @mentions in Telegram are UI echoes for humans only

**Privacy Mode Decision Matrix:**

| Privacy Mode | Works with Internal Bus? | Use Case |
|--------------|--------------------------|----------|
| **Enabled** (recommended) | ✅ Yes | Most secure; human @mentions only |
| **Disabled** | ✅ Yes (but not needed) | If you want unqualified message handling |

**Recommendation:** Keep privacy mode **ENABLED**. Only disable if you have a specific need for unqualified message routing (e.g., "fix this" without @mention). Never disable privacy for bots in public groups.

### Rate Limits

- 30 messages/second per bot
- With 3 bots: 90 messages/second theoretical max
- Implement throttling: `time.Sleep(100 * time.Millisecond)` between sends
- Track per-bot message counts in state manager

### Webhook vs Polling

- **Webhook** recommended for multi-bot (faster, more efficient)
- Each bot needs its own webhook endpoint
- Single ccc binary can handle all 3 webhooks on different paths:
  - `/webhook/planner`
  - `/webhook/executor`
  - `/webhook/reviewer`

### Message Editing

- Each bot can only edit its own messages
- Track `message_id` + `bot_username` in state
- Cross-bot updates: send new message instead of edit

## Alternative: Single Bot with Role Commands (DEPRECATED)

**Decision:** After UX analysis, the single-bot approach has been **deprecated** in favor of three-bot.

**Reasoning:**
1. **Reply Clarity**: Single bot creates ambiguity - all responses come from same username/avatar
2. **Unqualified Messages**: No clear routing for messages without commands
3. **Hidden State**: Single bot pretending to be 3 personas = confusing UX
4. **User Expectation**: "Talk to the expert" not "Talk to a router that pretends to be 3 experts"

**Three-bot advantages:**
- ✅ Clear identity: Each bot IS its role
- ✅ Visual clarity: Different usernames/avatars
- ✅ No routing confusion: @mentions are explicit
- ✅ Unqualified messages go nowhere (must @mention to trigger)
- ✅ Natural conversation flow

### Hybrid Approach (Best of Both)

Single bot that understands both patterns:

```
/planner build API          # Explicit command
plan: build API             # Implicit (defaults to planner)
@botname plan for executor  # Mention-based routing
```

This allows gradual migration and supports users who prefer different interaction styles.

## Final Recommendation

**Three-Bot Architecture (Primary Approach)**

After UX analysis considering reply clarity and unqualified message routing, the three-bot approach is selected as the primary implementation path.

### Why Three-Bot?

1. **Clear Identity**: Each bot has its own username, avatar, and persona
   - `@planner_bot` = The Planner
   - `@executor_bot` = The Executor
   - `@reviewer_bot` = The Reviewer

2. **Visual Clarity**: Users always know who they're talking to
   - No ambiguity about which role is responding
   - Clear conversation flow visible in Telegram

3. **Explicit Routing**: @mentions prevent routing confusion
   - Unqualified messages ignored (no accidental triggering)
   - Users must intentionally @mention to engage a bot
   - Cross-bot handoffs are explicit and visible

4. **Natural Conversation**: Mirrors how teams communicate
   - "@Jane review this" → "@Reviewer check the changes"
   - No special syntax or commands to remember

### Implementation Roadmap

#### Step 1: Bot Setup
1. Create 3 bots via BotFather:
   - `your_project_planner` → token saved as `bot_tokens.planner`
   - `your_project_executor` → token saved as `bot_tokens.executor`
   - `your_project_reviewer` → token saved as `bot_tokens.reviewer`

2. **CRITICAL**: Disable privacy mode for all 3 bots:
   ```
   /setprivacy → your_project_planner → "Disable"
   /setprivacy → your_project_executor → "Disable"
   /setprivacy → your_project_reviewer → "Disable"
   ```

3. Add all 3 bots to your private group as admins

#### Step 2: Router Implementation
1. Router polls all 3 bot tokens
2. Each message tagged with source role
3. Route to appropriate handler based on @mention detection
4. Shared conversation state across all 3 bots

#### Step 3: Tmux Integration
1. Each Telegram topic = 1 tmux window with 3 panes
2. Planner → Pane 0 (left), Executor → Pane 1 (middle), Reviewer → Pane 2 (right)
3. Dual visibility: Telegram (conversation) + Tmux (detail)

#### Step 4: State Management
1. Thread-safe `UpdateState()` pattern
2. Per-topic message processors
3. Conversation state tracking across bot handoffs

#### Step 5: Error Handling
1. Max iteration counter (prevent executor-reviewer loops)
2. Human escalation on bot disagreement
3. Message ID tracking (prevent duplicate processing)

### Security Considerations

⚠️ **Private Groups Only**: Privacy mode disabled means bots read ALL group messages.
- Use dedicated private groups for development
- Never add these bots to public channels
- Consider group membership carefully

✅ **Mitigation**: All 3 bots require explicit @mention to respond, reducing accidental trigger surface.

## Next Steps

1. **Bot Creation:**
   - Create 3 BotFather bots
   - Save tokens to config
   - Disable privacy mode

2. **Implementation:**
   - Router layer with 3-token polling
   - Role handlers (planner, executor, reviewer)
   - Thread-safe state management
   - Per-topic message queues

3. **Tmux Integration:**
   - 3-pane window creation
   - Message routing to panes
   - Topic lifecycle management

4. **Testing:**
   - Test @mention routing flow
   - Test cross-bot handoffs
   - Test state synchronization
   - Test error scenarios

5. **Production:**
   - Monitoring and observability
   - Error handling and escalation
   - Documentation and runbooks
