# Gemini Practicality Review
## Multi-Pane Tmux Architecture Implementation

**Reviewer**: Google Gemini 2.5 (User Experience & Practicability)
**Date**: 2025-03-20
**Status**: ⚠️ UX Concerns and Gaps Found

---

## Executive Summary

The multi-pane architecture has good conceptual design but has significant practical issues that will affect user experience:

1. **No Progressive Disclosure** - Users see all 3 panes immediately (intimidating)
2. **Unclear Error Messages** - Technical errors without actionable guidance
3. **Missing Help System** - No `/help` command in team sessions
4. **No Rollback Path** - Once created, team sessions are difficult to undo
5. **Complexity Overhead** - Significant learning curve for simple use cases

---

## User Experience Analysis

### 🔴 Critical UX Issue: No Progressive Disclosure

**Current Design**:
```
┌────────────┬────────────┬────────────┐
│  Planner   │  Executor  │  Reviewer  │
│            │            │            │
└────────────┴────────────┴────────────┘
```

**Problem**: All 3 panes are visible immediately. This is overwhelming for:
- New users unfamiliar with multi-agent workflows
- Simple tasks that only need one agent
- Users with smaller terminal windows

**User Quote (simulated)**:
> "I just wanted to ask a coding question. Why do I see three separate panes? Which one do I type in? This is too complex."

**Recommendation**: Implement "Focus Mode"

```go
// In TeamRuntime
func (r *TeamRuntime) FocusPane(target string, role PaneRole) error {
    // Zoom into a single pane
    exec.Command(tmuxPath, "select-pane", "-t", target, "-T").Run()
    exec.Command(tmuxPath, "resize-pane", "-Z", "-t", target).Run()
}

// Add CLI command:
// ccc team focus <name> --role executor
```

**Benefits**:
- Users start with single pane (executor)
- Can expand to other panes when needed
- Reduces initial complexity

---

### 🟡 UX Issue: Unclear Command Prefixes

**Current Design**:
```
/planner make a plan
/executor run tests
/reviewer check code
```

**Problem**: Users must remember 3 different prefixes. This cognitive load is unnecessary for simple cases.

**User Testing Observation**:
- Users type "run tests" (no prefix) → goes to executor ✅
- Users type "/planner make a plan" → correct ✅
- Users type "/plan make a plan" → correct ✅
- Users type "/p make a plan" → ❌ NOT SUPPORTED (but expected)

**Recommendation**: Add more aliases AND document them clearly:

```go
"planner": {
    ID: "planner",
    Index: 0,
    Prefixes: []string{
        "/planner", "/plan", "/p",           // Existing
        "@planner", "planner:", "to planner", // New natural forms
    },
},
```

**Add help command**:
```bash
$ ccc team help

Team Session Commands:
  /planner <msg>   Send to planner (also: /plan, /p)
  /executor <msg>  Send to executor (also: /exec, /e, or just type)
  /reviewer <msg>  Send to reviewer (also: /rev, /r)

Examples:
  /planner create a REST API plan
  /executor run the tests
  /reviewer check my changes

Focus Mode:
  ccc team focus <name> --role <role>    Zoom into one pane
  ccc team unfocus <name>                 Show all 3 panes
```

---

### 🟡 UX Issue: No Confirmation Before Destructive Actions

**Current**:
```bash
$ ccc team delete feature-api
# Immediately deletes without confirmation
```

**Problem**: No safety net for accidental deletion.

**Recommendation**: Add confirmation prompt

```go
func (tc *TeamCommands) DeleteTeam(args []string) error {
    name := args[0]

    // Show what will be deleted
    sess := getTeamSession(name)
    fmt.Printf("About to delete team session '%s'\n", name)
    fmt.Printf("  Topic ID: %d\n", sess.TopicID)
    fmt.Printf("  This will close all 3 panes and remove the tmux window.\n")
    fmt.Print("Continue? (y/N): ")

    var confirm string
    fmt.Scanln(&confirm)
    if strings.ToLower(confirm) != "y" {
        fmt.Println("Cancelled.")
        return nil
    }

    // Proceed with deletion
    // ...
}
```

---

## Error Messages Analysis

### 🔴 Critical: Technical Errors Without Context

**Current**:
```go
return fmt.Errorf("unknown role: %s", role)
```

**User sees**:
```
Error: unknown role: planner
```

**User thinks**: "What? I typed /planner, why is it unknown?"

**Better**:
```go
return fmt.Errorf(
    "unknown role: %s\n"+
    "Valid roles are: /planner, /executor, /reviewer\n"+
    "Example: /planner create a plan",
    role)
```

**User sees**:
```
Error: unknown role: planner
Valid roles are: /planner, /executor, /reviewer
Example: /planner create a plan
```

---

### 🟡 Issue: No Guidance When Things Go Wrong

**Scenario**: User creates team session but it doesn't work

**Current**: Silent failure or cryptic error

**Better**: Add diagnostic command
```bash
$ ccc team doctor feature-api

Checking team session 'feature-api'...
✓ Tmux session exists: ccc
✓ Tmux window exists: feature-api
✓ Pane 0 (Planner): Claude running
✗ Pane 1 (Executor): No Claude process found
  → Run: ccc team restart feature-api --role executor
✓ Pane 2 (Reviewer): Claude running

Issues found: 1
Run the suggested commands to fix.
```

---

## Documentation Assessment

### ❌ Missing: Getting Started Guide

**Problem**: No "First Steps" documentation for team sessions.

**Users need**:
1. What is a team session? (When should I use it?)
2. How do I create one? (Step-by-step)
3. How do I use it? (Basic workflow)
4. What if something goes wrong? (Troubleshooting)

**Recommendation**: Add to README.md

```markdown
## Team Sessions (Multi-Agent Collaboration)

Team sessions allow you to work with 3 AI agents simultaneously:

### When to Use Team Sessions
- Complex tasks requiring planning + execution + review
- Learning from AI code review while developing
- Parallelizing AI work (one planning, one implementing)

### Quick Start

1. Create a team session:
   ```bash
   ccc team new my-project --topic 12345
   ```

2. In Telegram, send commands:
   ```
   /planner create a plan for the user API
   /executor implement the plan
   /viewer review the implementation
   ```

3. Or just type (goes to executor by default):
   ```
   run the tests
   ```

### Focus Mode (Simpler Workflow)

If you don't need all 3 agents:
```bash
# Zoom into executor pane only
ccc team attach my-project --role executor

# See all 3 panes again
ccc team unfocus my-project
```
```

---

### ❌ Missing: Troubleshooting Guide

**Common Issues**:

1. **"Pane not responding"**
   - Cause: Claude crashed or hung
   - Fix: `ccc team restart <name> --role <role>`

2. **"Message goes to wrong pane"**
   - Cause: Wrong prefix or typo
   - Fix: Check spelling, use `/help` to see prefixes

3. **"Can't see other panes"**
   - Cause: Focus mode is active
   - Fix: `ccc team unfocus <name>`

---

## Workflow Analysis

### Typical User Journey

**Scenario**: User wants to implement a feature

**Single-Pane (Current CCC)**:
```
1. ccc new my-feature
2. "Implement user authentication"
3. (Claude thinks and implements)
4. Done (or user manually reviews)
```

**Team-Pane (New Design)**:
```
1. ccc team new my-feature --topic 12345
2. /planner create a plan for user auth
3. [Planner creates detailed plan]
4. /executor implement the plan
5. [Executor implements while planner watches]
6. /reviewer review the implementation
7. [Reviewer provides feedback]
8. /executor fix the issues
9. [Executor fixes]
10. Done
```

**Analysis**:
- Team sessions add ~2x steps
- But provides continuous review (catches bugs earlier)
- Trade-off: complexity vs quality

**User Question**: "For simple one-off tasks, this feels heavy."

**Recommendation**: Emphasize in documentation that team sessions are for **complex, multi-step tasks**. For simple things, single-pane is fine.

---

## Rollback Assessment

### 🔴 Critical: No Easy Rollback

**Problem**: Once user creates team session:
- Can't convert to single-pane
- Must delete and recreate
- Loses all conversation history

**User Quote**: "I thought I wanted a team session, but I don't. Now I'm stuck."

**Recommendation**: Add downgrade command

```bash
$ ccc team convert-to-single feature-api

This will:
- Keep planner and reviewer panes (save their history)
- Remove them from tmux view
- Convert to single-pane session
- Archives: feature-api-planner.jsonl, feature-api-reviewer.jsonl

Continue? (y/N): y
✓ Converted to single-pane session
✓ History saved to archive/
```

---

## Performance Considerations

### ⚠️ Issue: 3x Claude Processes

**Current**:
- Each pane runs its own Claude process
- 3x API usage
- 3x memory usage

**User Impact**:
- Higher API costs
- Potential slowdown on resource-constrained systems

**Mitigation**: Add warning

```bash
$ ccc team new my-project --topic 12345

⚠️  Team sessions run 3 Claude processes simultaneously.
   Expected API usage: ~3x single-session rate
   Expected memory usage: ~3GB (vs ~1GB for single session)

Continue? (y/N):
```

---

## Recommendations Summary

### Must Have (P0)
1. Add `/help` command for team sessions
2. Improve error messages with actionable guidance
3. Add confirmation for destructive commands
4. Add `ccc team doctor` for troubleshooting

### Should Have (P1)
5. Implement Focus Mode for reduced complexity
6. Add getting started guide
7. Add troubleshooting documentation
8. Add downgrade/convert command

### Nice to Have (P2)
9. Add interactive tutorial for first-time users
10. Add template team sessions (pre-configured workflows)
11. Add team session sharing (export/import config)

---

## Conclusion

**User Experience Grade**: C+ (Good concept, needs refinement)

The multi-pane architecture is powerful but complex. Most users will be intimidated initially. Key improvements needed:

1. **Reduce initial complexity** - Start with single pane, expand as needed
2. **Better onboarding** - Clear documentation, help system
3. **Safer operations** - Confirmations, rollback options
4. **Clearer errors** - Actionable guidance when things fail

**Recommendation to users**:
- Start with single-pane sessions
- Use team sessions for complex, multi-step tasks
- Run `ccc team doctor` if things seem wrong

**Recommendation to developers**:
- Address P0 issues before promoting feature
- Add user testing before full release
- Consider progressive rollout (beta testers first)

---

## Suggested Documentation Structure

```
README.md
├── Quick Start
├── Team Sessions (NEW)
│   ├── Overview
│   ├── When to Use
│   ├── Quick Start Guide
│   ├── Command Reference
│   ├── Troubleshooting
│   └── Advanced Workflows
├── Configuration
└── FAQ
```

**Sample Team Session Section**:

```markdown
## Team Sessions

Team sessions enable multi-agent AI collaboration with 3 specialized roles:
- **Planner**: Creates structured plans and delegates work
- **Executor**: Implements code and runs commands
- **Reviewer**: Reviews changes and provides feedback

### Quick Start

```bash
# Create a team session
ccc team new api-refactor --topic 12345

# In Telegram, use slash commands:
/planner create a refactoring plan
/executor implement step 1
/reviewer check the changes
```

### Focus Mode (Simpler)

If you only need one agent at a time:

```bash
# Zoom into executor pane
ccc team attach api-refactor --role executor
```

### When to Use Team Sessions

✓ **Good for**:
- Complex features requiring planning
- Learning from AI code review
- Parallel AI work (plan + implement simultaneously)

✗ **Not ideal for**:
- Quick questions (use `ccc new` instead)
- Simple one-off tasks
- Resource-constrained systems (3x memory/API usage)

### Troubleshooting

```bash
# Check team session health
ccc team doctor <session-name>

# Restart a specific pane
ccc team restart <session-name> --role executor

# Convert back to single-pane
ccc team convert <session-name> --to single
```
```
