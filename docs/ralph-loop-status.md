# Ralph Loop Status Report

## Iteration: 1 of 10
## Task: Multi-Bot Telegram Architecture Design

## Work Completed

### Design Documents Created
1. **multi-bot-design.md** (693 lines)
   - Architecture overview
   - Bot roles and responsibilities
   - Implementation approaches (A: 3 tokens, B: 3 binaries)
   - Router layer design with @mention parsing
   - Thread-safe state management (fixed after review)
   - Privacy mode considerations
   - Error handling strategies
   - Telegram API considerations
   - Two-phase rollout recommendation

2. **iteration-1-summary.md**
   - Summary of findings
   - Critical issues from Codex review
   - Fixes applied
   - Next iteration focus

3. **approach-comparison.md**
   - Visual comparison diagrams
   - Decision matrix
   - Conversation flow examples
   - Migration path from single to multi-bot

### Critical Issues Identified and Fixed

| Issue | Severity | Fix Applied |
|-------|----------|-------------|
| Telegram bot identity mismatch | P1 | Clarified 3 tokens required, not 1 |
| Privacy mode handling | P1 | Documented disable requirement with security warning |
| Race condition in state manager | P1 | Changed to `UpdateState()` pattern with locks |
| Concurrent message handling | P2 | Added per-topic processor queues |

## Architecture Decision

**Recommendation: Two-Phase Rollout**

### Phase 1: Single Bot (Recommended Start)
- Commands: `/planner`, `/executor`, `/reviewer`
- One bot token
- Privacy mode enabled
- Validates concept with lower risk

### Phase 2: Three Bots (If UX demands)
- @mentions: `@planner_bot`, `@executor_bot`, `@reviewer_bot`
- Three bot tokens
- Privacy mode disabled (private groups only)
- More natural conversation flow

## Current Design State

✅ Router layer architecture defined
✅ Thread-safe state management pattern
✅ Privacy mode requirements documented
✅ Error handling strategies outlined
✅ Concurrency model specified
✅ Two-phase implementation plan

## Next Steps Options

### Option A: Continue Ralph Loop
- Refine design further
- Add more implementation details
- Consider edge cases
- Explore alternatives

### Option B: Exit Loop and Implement
- Design is sufficiently detailed
- Critical issues identified and fixed
- Clear implementation path
- Ready to begin coding

### Option C: Validate with Prototype
- Implement Phase 1 (single bot)
- Test multi-role collaboration
- Gather user feedback
- Inform Phase 2 decisions

## Recommendation

**Three-Bot Architecture Selected**

After UX analysis considering reply clarity and unqualified message routing, the three-bot approach is selected.

### Why Three-Bot?

1. **Clear Identity**: `@planner_bot` IS the planner, `@executor_bot` IS the executor
2. **Visual Clarity**: Different usernames/avatars = no confusion
3. **Explicit Routing**: @mentions prevent accidental triggering
4. **Natural UX**: Mirrors how teams actually communicate

### What Was Wrong with Single-Bot?

1. **Reply Ambiguity**: All responses from same username - which "expert" is speaking?
2. **Unqualified Messages**: No clear default routing - where does "fix the bug" go?
3. **Hidden State**: One bot pretending to be 3 personas = confusing
4. **User Friction**: Must remember `/commands` instead of natural @mentions

### Implementation Path

1. Create 3 BotFather bots
2. Disable privacy mode (private groups only)
3. Router with 3-token polling
4. Tmux 3-pane layout
5. Thread-safe state management

## Files Created This Iteration

```
docs/
├── multi-bot-design.md          # Main design document (updated after review)
├── iteration-1-summary.md       # Summary of findings and fixes
├── approach-comparison.md       # Visual comparison of approaches
├── tmux-architecture.md         # Tmux 3-pane layout design (NEW)
└── ralph-loop-status.md         # This file
```

## Tmux Architecture Addition (User Enhancement)

### Key Insight
Each Telegram topic maps to **1 tmux window with 3 panes**:
- **Pane 0 (Left)**: Planner bot session
- **Pane 1 (Middle)**: Executor bot session
- **Pane 2 (Right)**: Reviewer bot session

### Benefits
1. **Dual Visibility**:
   - **Telegram**: High-level conversation flow between bots
   - **Tmux**: Low-level detail of each bot's work

2. **Observability**: See all 3 bots working simultaneously

3. **Debugging**: Easy to drill down into specific bot's session

4. **Interactive Intervention**: Can type into any pane to guide specific bot

### Hierarchy Mapping
```
Telegram Group    → Tmux Session
  └── Topic       →   └── Window (3 panes)
```

### Implementation File
`tmux-architecture.md` - Complete Go implementation for:
- Creating 3-pane windows
- Routing messages to specific panes
- Managing topic window lifecycle
- Pane switching and zooming

## Implementation Roadmap

### Step 1: Bot Creation
- Create 3 BotFather bots with distinct usernames
- Privacy mode can stay **ENABLED** (with internal event bus)
- Save tokens to config

### Step 2: Core Implementation
- **Internal event bus** (P0 - CRITICAL for bot-to-bot coordination)
- Router layer polling 3 bot tokens + internal bus listener
- Role handlers with HandleInternal methods
- Thread-safe state management
- Per-topic message queues

### Step 3: Tmux Integration
- 3-pane window creation per topic
- Message routing to correct pane
- Topic lifecycle management

### Step 4: Testing
- @mention routing flows
- Internal bot-to-bot handoffs via event bus
- State synchronization
- Error scenarios

### Step 5: Production Readiness
- Monitoring and observability
- Error handling and escalation
- Documentation

## Completion Assessment

The design is complete for iteration 1. Three-bot architecture selected based on UX analysis.
- ✅ Architecture soundness: Validated with fixes applied
- ✅ State management: Thread-safe pattern defined
- ✅ @mention routing: Clarified as requiring 3 tokens
- ✅ Error scenarios: Strategies documented
- ✅ Implementation complexity: Phased approach defined
- ✅ Security concerns: Documented with warnings
- ✅ **Bot-to-bot visibility**: Internal event bus architecture added (2026-03-20)
- ✅ **Privacy mode**: Can stay enabled with internal bus (2026-03-20)
- ✅ Better alternatives: Single-bot approach recommended as starting point
