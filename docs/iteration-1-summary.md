# Ralph Loop Iteration 1 Summary

## Task
Design and analyze a multi-bot collaboration system for Telegram with 3 specialized AI bots (planner, executor, reviewer) collaborating via @mention routing.

## Design Document Created
`docs/multi-bot-design.md` - Comprehensive design covering:
- Architecture overview
- Bot roles and responsibilities
- Implementation approaches
- Router layer design
- State management
- Error handling
- Telegram API considerations

## Critical Findings from Codex-5.3-High Review

### 1. Telegram Bot Identity Mismatch (P1)
**Issue:** Initial design incorrectly assumed a single bot token could route @mentions for different bot usernames.

**Reality:** Each @mention (@planner_bot, @executor_bot, @reviewer_bot) requires:
- Its own bot account created via BotFather
- Its own bot token
- Separate polling/webhook handling

**Fix Applied:** Updated design to specify 3 separate bot tokens with router polling all 3.

### 2. Privacy Mode Handling (P1)
**Issue:** Default bot behavior only receives commands and @mentions. Router's fallback to handle unmentioned human messages won't work.

**Fix Applied:** Documented requirement to disable privacy mode for all 3 bots, with security warning to only use in private groups.

### 3. Race Condition in State Manager (P1)
**Issue:** `GetState()` returned mutable pointer after releasing mutex, allowing concurrent modification.

**Fix Applied:** Changed to:
- Return immutable copies via `DeepCopy()`
- Added `UpdateState()` method for atomic read-modify-write operations
- Added helper methods (`SetCurrentRole`, `SetPlan`, `AddReviewFinding`)

## Architecture Decision

**Recommended Approach: Two-Phase Rollout**

### Phase 1: Single Bot with Role Commands
- One bot token
- Commands: `/planner`, `/executor`, `/reviewer`
- Privacy mode enabled (more secure)
- Validates the multi-role collaboration concept
- Lower complexity

### Phase 2: Three-Bot Architecture (if UX demands)
- Three bot tokens
- @mention routing: `@planner_bot`, `@executor_bot`, `@reviewer_bot`
- Privacy mode disabled (private groups only)
- More natural conversation flow
- Higher complexity

## Design Highlights

### Router Layer
```go
type MessageRouter struct {
    config          *Config
    stateManager    *ConversationStateManager
    roleHandlers    map[string]RoleHandler
    updateChannels  map[string]chan TelegramUpdate
}
```

### Thread-Safe State Management
```go
// UpdateState executes a function under write lock for atomic updates
func (m *ConversationStateManager) UpdateState(topicID int64, fn func(*ConversationState)) error
```

### Concurrency Model
- Per-topic message processors
- Sequential processing within topic
- Concurrent processing across topics
- Message ID tracking to prevent duplicates

## Files Created/Modified
1. `docs/multi-bot-design.md` - Main design document (updated with fixes)
2. `docs/iteration-1-summary.md` - This file

## Next Iteration Focus
- Implement Phase 1 (single bot) prototype
- Validate multi-role collaboration concept
- Gather user feedback on UX
- Determine if Phase 2 (3-bot) is warranted

## Completion Status
Design document created and updated based on Codex review findings. Architecture clarified to address Telegram Bot API constraints and thread safety concerns.
