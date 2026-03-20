# Approach Comparison: Single Bot vs Three Bots

## Visual Comparison

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         SINGLE BOT APPROACH                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   Human Message                                                          │
│        │                                                                 │
│        ▼                                                                 │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │  @ccc_bot /planner build a REST API                           │     │
│   └──────────────────────────────────────────────────────────────┘     │
│                        │                                                │
│                        ▼                                                │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │  Command Parser: /planner → Planner Handler                  │     │
│   └──────────────────────────────────────────────────────────────┘     │
│                        │                                                │
│        ┌───────────────┼───────────────┐                              │
│        ▼               ▼               ▼                              │
│   ┌─────────┐   ┌─────────┐   ┌─────────┐                           │
│   │ Planner │   │Executor │   │Reviewer │                           │
│   │ Handler │   │ Handler │   │ Handler │                           │
│   └─────────┘   └─────────┘   └─────────┘                           │
│        │               │               │                              │
│        └───────────────┼───────────────┘                              │
│                        ▼                                                │
│               Shared Conversation State                                 │
│                                                                         │
│  ✅ Simpler Setup (1 bot token)                                         │
│  ✅ Privacy Mode Compatible                                             │
│  ✅ Lower Security Surface                                              │
│  ❌ Less Natural UX (/commands instead of @mentions)                    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                         THREE-BOT APPROACH                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   Human Message                                                          │
│        │                                                                 │
│        ▼                                                                 │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │  @planner_bot build a REST API                                 │     │
│   └──────────────────────────────────────────────────────────────┘     │
│                        │                                                │
│                        ▼                                                │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │  Telegram delivers to planner_bot token only                   │     │
│   └──────────────────────────────────────────────────────────────┘     │
│                        │                                                │
│                        ▼                                                │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │  Router: Source = planner → Planner Handler                   │     │
│   └──────────────────────────────────────────────────────────────┘     │
│                        │                                                │
│                        ▼                                                │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │  Planner: "Plan created! @executor_bot please implement..."   │     │
│   └──────────────────────────────────────────────────────────────┘     │
│                        │                                                │
│                        ▼                                                │
│   ┌──────────────────────────────────────────────────────────────┐     │
│   │  Human or Auto: @executor_bot is mentioned → executor token   │     │
│   └──────────────────────────────────────────────────────────────┘     │
│                        │                                                │
│        ┌───────────────┼───────────────┐                              │
│        ▼               ▼               ▼                              │
│   ┌─────────┐   ┌─────────┐   ┌─────────┐                           │
│   │ Planner │   │Executor │   │Reviewer │                           │
│   │ Handler │   │ Handler │   │ Handler │                           │
│   │ Token   │   │ Token   │   │ Token   │                           │
│   └─────────┘   └─────────┘   └─────────┘                           │
│        │               │               │                              │
│        └───────────────┼───────────────┘                              │
│                        ▼                                                │
│               Shared Conversation State                                 │
│                                                                         │
│  ✅ Natural @mention Conversation Flow                                  │
│  ✅ Clear Bot Identity (who's responding)                               │
│  ❌ Complex Setup (3 bot tokens, privacy mode disabled)                  │
│  ❌ Higher Security Surface (bots read all messages)                    │
│  ❌ Only Viable in Private Groups                                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Decision Matrix

| Criteria | Single Bot | Three Bots |
|----------|-----------|------------|
| **Setup Complexity** | ⭐ Simple | ⭐⭐⭐ Complex |
| **Security** | ⭐⭐⭐ Privacy mode OK | ⭐⭐ Privacy disabled |
| **UX Naturalness** | ⭐⭐ Commands required | ⭐⭐⭐ @mentions |
| **Deployment** | ⭐⭐⭐ Single service | ⭐⭐ Single service, 3 tokens |
| **Maintenance** | ⭐⭐⭐ Easy | ⭐⭐ Moderate |
| **Group Type** | ⭐⭐⭐ Public or Private | ⭐ Private only |
| **Development Time** | ⭐⭐⭐ Weeks | ⭐⭐ Weeks-Months |
| **Scalability** | ⭐⭐ Add new commands | ⭐ Add new bots/tokens |

## Conversation Flow Examples

### Single Bot Flow
```
You: /planner build a REST API for users

Planner: Here's my plan:
1. Design API endpoints
2. Create User model
3. Implement CRUD operations
4. Add authentication

Run /executor to implement steps 1-4.

You: /executor implement the plan

Executor: Implementing...
[Code changes]

Run /reviewer to review the changes.

You: /reviewer please review

Reviewer: Review complete. Found 2 issues:
1. Missing input validation
2. No rate limiting

Run /executor to fix issues.
```

### Three-Bot Flow
```
You: @planner_bot build a REST API for users

Planner: Here's my plan:
1. Design API endpoints
2. Create User model
3. Implement CRUD operations
4. Add authentication

@executor_bot Please implement steps 1-4.

Executor: Implementing...
[Code changes]

@reviewer_bot Please review the changes.

Reviewer: Review complete. Found 2 issues:
1. Missing input validation
2. No rate limiting

@executor_bot Please fix the issues.
```

## Recommendation

**Start with Single Bot.** Reasons:
1. Faster to implement and validate the concept
2. No security compromises (privacy mode stays on)
3. Works in any group type
4. Lower operational complexity

**Migrate to Three Bots if:**
- User testing shows @mention UX is critical
- Operating exclusively in private groups
- Team comfortable with security trade-offs

## Migration Path (Single → Three Bots)

```
Phase 1: Single Bot (Current)
  /planner, /executor, /reviewer commands
         │
         ▼
Phase 2: Hybrid (Transition)
  Support both commands AND @mentions
  - Single bot token
  - Parse @mentions in messages
  - Route to appropriate handler
         │
         ▼
Phase 3: Three Bots (Final)
  Separate bot tokens for each role
  - @planner_bot, @executor_bot, @reviewer_bot
  - Privacy mode disabled
  - Private groups only
```

This allows gradual migration without breaking existing workflows.
