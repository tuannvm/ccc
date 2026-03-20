# Multi-Bot Telegram Architecture Design

## Overview

A system of 3 specialized AI bots (planner, executor, reviewer) collaborating in a single Telegram topic via @mention routing.

## CRITICAL ARCHITECTURAL DECISION

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
- Bot privacy mode MUST be disabled for all 3 bots
- In BotFather: `/setprivacy` → "Disable" for each bot
- This ensures bots receive ALL group messages, not just @mentions
- Router then filters messages based on intended recipient

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
// New: Message router handling 3 bot tokens
type MessageRouter struct {
    config          *Config
    stateManager    *ConversationStateManager
    roleHandlers    map[string]RoleHandler
    // Each bot has its own update channel
    updateChannels  map[string]chan TelegramUpdate
}

func (r *MessageRouter) Start() {
    // Start polling for all 3 bots
    for role := range r.config.BotTokens {
        go r.pollBot(role)
    }
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
            // Tag update with source role for routing
            update.SourceRole = role
            r.routeUpdate(update)
        }
    }
}

func (r *MessageRouter) routeUpdate(update TelegramUpdate) error {
    msg := update.Message

    // Skip if no message (e.g., callback query handled separately)
    if msg == nil {
        return nil
    }

    // CRITICAL: Each message already has a source role (which bot received it)
    // We only handle messages where the sender explicitly @mentioned the receiving bot
    sourceRole := update.SourceRole

    // Extract @mentions to find intended recipient
    mentions := extractMentions(msg.Text)

    // Check if this bot was mentioned
    botUsername := r.config.BotUsernames[sourceRole]
    isMentioned := false
    for _, m := range mentions {
        if m == botUsername || m == sourceRole {
            isMentioned = true
            break
        }
    }

    // Only process if this bot was explicitly mentioned
    if !isMentioned {
        return nil
    }

    // Get handler for source role
    handler, exists := r.roleHandlers[sourceRole]
    if !exists {
        return fmt.Errorf("no handler for role: %s", sourceRole)
    }

    return handler.Handle(msg)
}

func extractMentions(text string) []string {
    // Extract @mentions like @planner_bot, @executor_bot, @reviewer_bot
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
type RoleHandler interface {
    Handle(msg TelegramMessage) error
    CanHandle(msg TelegramMessage) bool
}

type PlannerHandler struct {
    state   *ConversationStateManager
    config  *Config
}

func (h *PlannerHandler) Handle(msg TelegramMessage) error {
    // Planner logic:
    // 1. Analyze request
    // 2. Create structured plan
    // 3. @mention executor to implement
    prompt := msg.Text

    // Remove @planner_bot from prompt
    prompt = strings.ReplaceAll(prompt, "@planner_bot", "")
    prompt = strings.TrimSpace(prompt)

    // Create plan
    response, err := h.createPlan(prompt)
    if err != nil {
        return err
    }

    // Send response
    sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, response)

    // If plan approved, @mention executor
    if h.shouldProceed(msg) {
        handoff := fmt.Sprintf("@executor_bot Please implement: %s", h.getPlanSummary())
        sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, handoff)
    }

    return nil
}

type ExecutorHandler struct {
    state   *ConversationStateManager
    config  *Config
}

func (h *ExecutorHandler) Handle(msg TelegramMessage) error {
    // Executor logic:
    // 1. Parse task from @mention
    // 2. Execute implementation
    // 3. @mention reviewer if needed
    prompt := strings.ReplaceAll(msg.Text, "@executor_bot", "")
    prompt = strings.TrimSpace(prompt)

    response, err := h.executeTask(prompt)
    if err != nil {
        return err
    }

    sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, response)

    // If code changed, @mention reviewer
    if h.hasCodeChanges() {
        handoff := "@reviewer_bot Please review the changes"
        sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, handoff)
    }

    return nil
}

type ReviewerHandler struct {
    state   *ConversationStateManager
    config  *Config
}

func (h *ReviewerHandler) Handle(msg TelegramMessage) error {
    // Reviewer logic:
    // 1. Review code/changes
    // 2. Provide feedback
    // 3. @mention executor for fixes OR approve
    prompt := strings.ReplaceAll(msg.Text, "@reviewer_bot", "")
    prompt = strings.TrimSpace(prompt)

    review, err := h.reviewChanges(prompt)
    if err != nil {
        return err
    }

    sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, review)

    // If issues found, @mention executor
    if h.hasIssues() {
        handoff := "@executor_bot Please fix the following issues..."
        sendMessage(h.config, msg.Chat.ID, msg.MessageThreadID, handoff)
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

**Our requirement:** Bots must see all messages to detect cross-bot handoffs.

**Solution:** Disable privacy mode for all 3 bots in BotFather.

**Trade-offs:**
| Privacy Mode | Pros | Cons |
|--------------|------|------|
| **Enabled** (default) | More private, bot sees less | Cannot detect all @mentions in conversation |
| **Disabled** (required) | Full message visibility, works as designed | Bot reads ALL group messages (only use in private groups) |

**Recommendation:** Use dedicated private groups for development. Never disable privacy for bots in public groups.

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

## Alternative: Single Bot with Role Prefix (Simpler Alternative)

If creating 3 bots is too complex or privacy mode is a concern, use a single bot with role-based commands:

```
/planner build a REST API
/executor implement the plan
/reviewer review the changes
```

**Pros:**
- Single bot token (simpler setup)
- Privacy mode can stay enabled
- Lower security surface
- Easier to deploy and maintain

**Cons:**
- Loses explicit @mention routing UX
- All commands must use `/role` prefix
- Less natural conversation flow
- User must remember command prefixes

**Recommendation:** Start with single-bot approach to validate the concept, then migrate to 3-bot architecture if the UX is critical.

### Hybrid Approach (Best of Both)

Single bot that understands both patterns:

```
/planner build API          # Explicit command
plan: build API             # Implicit (defaults to planner)
@botname plan for executor  # Mention-based routing
```

This allows gradual migration and supports users who prefer different interaction styles.

## Final Recommendation

**Start with Single Bot (Alternative), then scale to 3-Bot if UX demands it:**

### Phase 1: Single Bot with Role Commands (Recommended Starting Point)

1. Create 1 bot account via BotFather
2. Implement role-based commands: `/planner`, `/executor`, `/reviewer`
3. Add conversation state management
4. Test the multi-role collaboration concept
5. Privacy mode can stay enabled (more secure)

### Phase 2: Scale to 3-Bot Architecture (If UX is critical)

1. Create 3 bot accounts via BotFather
2. Disable privacy mode for all 3 (use only in private groups)
3. Implement router layer polling 3 bot tokens
4. Add per-topic processor queues for concurrency
5. Use `UpdateState()` pattern for thread-safe state updates

**Decision Criteria:**
- Go with 3-bot if: Natural @mention conversation flow is essential, using private trusted groups
- Stay with 1-bot if: Simplicity and security are higher priority, command-based UX is acceptable

## Next Steps

1. **Phase 1 (Immediate):**
   - Create 1 BotFather bot
   - Implement `/planner`, `/executor`, `/reviewer` commands
   - Add basic state management
   - Test single-bot collaboration flow

2. **Phase 2 (If UX validates):**
   - Create additional BotFather bots for separate identities
   - Disable privacy mode (private groups only!)
   - Implement router with 3-token polling
   - Add per-topic message queues
   - Implement thread-safe `UpdateState()` pattern
   - Test full @mention routing flow

3. **Phase 3 (Production):**
   - Add error handling and escalation
   - Implement retry logic for failed handoffs
   - Add monitoring and observability
   - Document operational procedures
