# Final Architecture Review: Three-Bot Telegram System

**Date:** 2026-03-20
**Status:** ✅ RESOLVED - Fix incorporated into main design
**Decision:** Three-bot architecture with internal event bus (see multi-bot-design.md)

---

## Executive Summary

**⚠️ CRITICAL FINDING:** The current design has a **functional deal-breaker**.

**Issue:** Telegram bots **cannot see messages sent by other bots** via the API, regardless of privacy settings.

**Impact:** The handoff mechanism where `@planner_bot` says "@executor_bot implement this" will **NOT WORK**. The executor will never receive that message through Telegram's API.

---

## Review Panel

| Reviewer | Decision on Three-Bot | Key Concerns |
|-----------|------------------------|--------------|
| **Codex** (GPT-5.3) | Proceed with phased rollout | State bugs, operational complexity, privacy mode |
| **Gemini** (Brainstorm) | ⚠️ DEAL-BREAKER FOUND | Bot-to-bot visibility = functional blocker |
| **Claude** (Opus) | *Pending - model unavailable* | - |

---

## Critical Deal-Breaker: Bot Visibility

### The Problem

```
@planner_bot: "@executor_bot implement the API"

Telegram API delivers to: ✅ planner_bot (sender)
Telegram API delivers to: ❌ executor_bot (mentioned, but not a user message)
```

**Bots cannot see messages from other bots.** Period.

### Why This Matters

Your design assumes:
1. Planner says "@executor_bot do X"
2. Executor's polling sees this message
3. Executor responds

**Reality:**
1. Planner says "@executor_bot do X" ✅ (visible in chat)
2. Executor's polling **NEVER SEES THIS** ❌ (bot-to-bot message)
3. Executor never responds ❌

### The "Virtual Handoff" Fix

Gemini's recommended solution: **Internal Event Bus**

```go
// Bots don't talk via Telegram
// They talk via internal Go channels

type InternalEventBus struct {
    events chan BotEvent
}

type BotEvent struct {
    FromRole string  // "planner"
    ToRole   string  // "executor"
    Message  string
    TopicID  int64
}

// When Planner wants to hand off to Executor:
func (h *PlannerHandler) HandoffToExecutor(topicID int64, task string) {
    // Send to Telegram for human visibility
    sendMessage(config, topicID, "@executor_bot Please implement: " + task)

    // ACTUAL handoff via internal bus
    eventBus.events <- BotEvent{
        FromRole: "planner",
        ToRole:   "executor",
        Message:  task,
        TopicID:   topicID,
    }
}

// Router listens to bus AND Telegram polling
func (r *MessageRouter) Start() {
    // Poll 3 bot tokens for human messages
    for role := range r.config.BotTokens {
        go r.pollBot(role)
    }

    // Listen to internal bus for bot-to-bot handoffs
    go r.listenToBus()
}
```

**This changes everything:**
- Telegram becomes a **display layer only**
- Real handoffs happen **internally**
- The @mentions in Telegram are **UI echoes**, not functional triggers

---

## Updated Architecture: Internal Event Bus

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
│  ┌──────────────┬──────────────┬──────────────┐                     │
│  │  Planner     │  Executor    │  Reviewer     │                     │
│  │  Handler     │  Handler     │  Handler     │                     │
│  │  ┌────────┐   │  ┌────────┐   │  ┌────────┐   │                     │
│  │  │ Tmux   │   │  │ Tmux   │   │  │ Tmux   │   │                     │
│  │  │ Pane 0 │   │  │ Pane 1 │   │  │ Pane 2 │   │                     │
│  │  └────────┘   │  └────────┘   │  └────────┘   │                     │
│  └──────────────┴──────────────┴──────────────┘                     │
└─────────────────────────────────────────────────────────────────┘
```

---

## Complete Risk Assessment

### 1. Bot Visibility (CRITICAL - BLOCKER)
- **Risk:** Bots can't see each other's messages
- **Impact:** Handoffs don't work as designed
- **Fix:** Internal event bus for real coordination
- **Status:** ⚠️ REQUIRES ARCHITECTURE CHANGE

### 2. State Synchronization (HIGH)
- **Risk:** Shared state across 3 bots = race conditions
- **Impact:** Corrupted state, lost messages, inconsistent behavior
- **Mitigation:** `UpdateState()` with atomic locks, per-topic sequential processing
- **Status:** ✅ Design addresses this

### 3. Privacy Mode Security (HIGH)
- **Risk:** Disabled privacy = bots read ALL group messages
- **Impact:** Privacy leak if logging, accidental trigger surface
- **Mitigation:**
  - Private groups only
  - Strict @mention enforcement
  - Consider re-enabling privacy (internal bus makes it possible)
- **Status:** ⚠️ Requires operational discipline

### 4. Operational Complexity (MEDIUM)
- **Risk:** 3 tokens, 3 webhooks/pollers, identity drift
- **Impact:** Harder to operate, harder to debug
- **Mitigation:** Single binary, unified config, health checks
- **Status:** ⚠️ Acceptable trade-off for clarity

### 5. Single Point of Failure (HIGH)
- **Risk:** Router crashes = all 3 bots down
- **Impact:** Total system outage
- **Mitigation:** Systemd auto-restart, health monitoring
- **Status:** ✅ Existing service.go handles this

### 6. Message Ordering (MEDIUM)
- **Risk:** Out-of-order messages break state machine
- **Impact:** Wrong state, missed handoffs
- **Mitigation:** Per-topic sequential processing, message IDs
- **Status:** ✅ Design addresses this

### 7. Unqualified Message Routing (LOW)
- **Risk:** User types "fix this" without @mention
- **Impact:** Confusion about which bot handles it
- **Mitigation:** Require @mention - ignore everything else
- **Status:** ✅ This is a feature, not a bug

### 8. Rate Limiting (LOW)
- **Risk:** 3 bots = 90 msg/sec theoretical max
- **Impact:** Could hit limits in busy groups
- **Mitigation:** Throttling, per-bot tracking
- **Status:** ✅ Manageable

---

## What Will Break First in Production

1. **Bot handoffs** (if internal bus not implemented)
   - Planner says "@executor_bot do X"
   - Executor never responds
   - User confusion: "Is it broken?"

2. **State corruption** (under concurrent load)
   - Two messages arrive simultaneously
   - Race condition in `UpdateState()`
   - Inconsistent state across bots

3. **Privacy leaks** (if logging not filtered)
   - Debug logs capture ALL group messages
   - Sensitive data exposed
   - Compliance violation

4. **Router crash** (single point of failure)
   - All 3 bots go down
   - In-flight work lost
   - No error recovery

---

## Recommendations

### 1. IMPLEMENT INTERNAL EVENT BUS (REQUIRED)

**Don't rely on Telegram for bot-to-bot communication.**

```go
type EventRouter struct {
    botEvents     chan BotEvent
    roleHandlers   map[string]RoleHandler
    tmuxWindows   map[int64]*TmuxWindow
    stateManager  *ConversationStateManager
}

func (r *EventRouter) Start() {
    // Poll Telegram for human messages
    for role := range r.config.BotTokens {
        go r.pollTelegram(role)
    }

    // Handle internal bot-to-bot events
    go r.handleBotEvents()
}

func (r *EventRouter) handleBotEvents() {
    for event := range r.botEvents {
        // This is the REAL trigger
        handler := r.roleHandlers[event.ToRole]
        handler.HandleInternal(event)
    }
}
```

### 2. KEEP PRIVACY MODE IF POSSIBLE

With internal event bus, **you can re-enable privacy mode**:
- Bots still see human @mentions
- Bot-to-bot coordination happens internally
- Much more secure

**Revised recommendation:**
```
Step 1: Implement internal event bus
Step 2: Keep privacy mode ENABLED
Step 3: Only disable if you hit a limitation
```

### 3. PERSISTENT STATE (RECOMMENDED)

Current design uses in-memory state. Add persistence:

```go
type PersistentStateManager struct {
    db     *sql.DB
    cache  *ConversationStateManager  // in-memory cache
}

func (m *PersistentStateManager) UpdateState(topicID int64, fn func(*ConversationState)) error {
    // Start transaction
    tx := m.db.Begin()

    // Lock and update
    m.cache.UpdateState(topicID, fn, func(state *ConversationState) {
        // Persist to DB
        tx.Save(state)
    })

    tx.Commit()
}
```

**Benefits:**
- Survives restarts
- Audit trail
- Debugging support
- Replay capability

### 4. CIRCUIT BREAKERS (REQUIRED)

Add safety limits:

```go
type CircuitBreaker struct {
    MaxIterations int    // 3 loops max
    MaxCost       float64 // $5 max per task
    MaxDuration   time.Duration // 30 min max
}

func (b *CircuitBreaker) ShouldContinue(execution *ExecutionState) bool {
    if executionState.IterationCount >= b.MaxIterations {
        return false  // Require human intervention
    }
    if executionState.Cost >= b.MaxCost {
        return false  // Budget exceeded
    }
    return true
}
```

### 5. OBSERVABILITY (RECOMMENDED)

You're blind without it:

```go
type Telemetry struct {
    CorrelationID string
    StartTime     time.Time
    Events        []Event
    BotMetrics    map[string]BotMetrics
}

type BotMetrics struct {
    MessagesSent    int
    TokensUsed      int
    Errors          int
    AverageLatency  time.Duration
}

// Trace a handoff through the system
trace := telemetry.Start("plan-to-execution", topicID)
trace.Record("planner", "handoff", "executor")
trace.Record("executor", "started", task)
trace.Finish()
```

---

## Revised Architecture

### Core Principle

**Telegram is the UI, not the message bus.**

```
Human Message → Telegram API → Router → Role Handler
                                                │
                                                ▼
                                        Internal Event Bus
                                                │
                                                ▼
                                    Bot-to-Bot Handoff (Internal)
                                                │
                                                ▼
                                    Telegram Echo (for visibility)
```

### Key Changes

1. **Add Internal Event Bus** - Required for bot-to-bot coordination
2. **Keep Privacy Mode** - Now possible with internal bus
3. **Add State Persistence** - Survive restarts
4. **Add Circuit Breakers** - Prevent runaway loops
5. **Add Observability** - Know what's happening

---

## Final Verdict

### Decision

**Proceed with three-bot approach** BUT **implement internal event bus first.**

### Why This Works

| Concern | Original Design | Revised Design |
|----------|-----------------|----------------|
| Bot visibility | ❌ Breaks handoffs | ✅ Internal bus |
| Privacy mode | ❌ Must disable | ✅ Can stay enabled |
| State | ⚠️ In-memory only | ✅ Persistent option |
| Observability | ❌ Missing | ✅ Built in |
| Safety | ❌ No limits | ✅ Circuit breakers |

### Implementation Priority

1. **P0 (Must Have):** Internal event bus
2. **P0 (Must Have):** State persistence
3. **P1 (Should Have):** Circuit breakers
4. **P1 (Should Have):** Observability/telemetry
5. **P2 (Nice to Have):** Privacy mode re-enablement

---

## What the AIs Missed

### Codex
- Focused on implementation complexity
- Missed the bot visibility deal-breaker
- Good on state management risks

### Gemini
- ✅ **Identified bot visibility issue**
- ✅ Suggested virtual handoff / internal bus
- ✅ 15 creative ideas for improvement

### Claude (Opus)
- Model unavailable through Codex
- Would likely focus on state machine design

---

## Conclusion

The three-bot approach is **the right UX decision**, but the original design had a **fatal flaw** that would prevent it from working.

**The fix (internal event bus):**
- Makes bot-to-bot coordination actually work
- Allows keeping privacy mode enabled
- Adds complexity but necessary
- Separates UI from logic

**Recommendation:** Proceed with revised architecture incorporating internal event bus.

**Next Step:** Update design documents to reflect internal event bus architecture before coding begins.
