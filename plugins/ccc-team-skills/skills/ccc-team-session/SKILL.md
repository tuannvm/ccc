---
name: ccc-team-session
description: CCC Team Session Management. Use when working in a 3-pane team session (planner/executor/reviewer). Covers role responsibilities, inter-role communication patterns, and session workflow.
---

# CCC Team Session Management

Manage 3-pane team sessions where each pane runs a separate Claude Code instance with a specialized role.

**Note:** The tmux pane creation and layout is handled by CCC's Go code (`session/team_runtime.go`). This skill focuses on **role interaction and communication patterns**.

## Architecture

```text
┌─────────┬─────────┬─────────┐
│Planner  │Executor │Reviewer │
│  Role   │  Role   │  Role   │
│   0     │   1     │   2     │
└─────────┴─────────┴─────────┘
```

Each pane has `CCC_ROLE` set to identify its role.

## Role Responsibilities

| Role | Responsibility | Communication |
|------|---------------|--------------|
| **Planner** | Task decomposition, delegation | `@executor` for tasks, `@reviewer` for review requests |
| **Executor** | Code implementation, testing | `@planner` for questions, `@reviewer` for review |
| **Reviewer** | Code review, approval | `@executor` for revisions, `@planner` for sign-off |

## Communication Flow

```text
Planner → @executor "Please implement feature X"
Executor → @reviewer "Review PR #123"
Reviewer → @executor "LGTM, merge when ready"
Executor → @planner "Feature X complete and merged"
```

## Session Lifecycle

```text
Create → Start → Work ←→ Pause ←→ Resume → Close
              ↓
         [3 panes running]
```

- **Pause**: `tmux detach` (Ctrl+b d)
- **Resume**: `tmux attach -t ccc-team:<name>`
- **Close**: `tmux kill-session -t ccc-team:<name>`

## Interacting with Roles

Use `@mention` to communicate between panes:

| From | To | Example |
|------|-----|---------|
| Planner | @executor | "Delegate a task" |
| Planner | @reviewer | "Request review" |
| Executor | @planner | "Ask for clarification" |
| Executor | @reviewer | "Submit for review" |
| Reviewer | @executor | "Request changes" |
| Reviewer | @planner | "Approve/reject" |

See `ccc-interpane` skill for the communication protocol (ACK/Done/NACK).

## CCC_ROLE Environment

Each pane has `CCC_ROLE` set when Claude starts:
- Pane 1: `CCC_ROLE=planner`
- Pane 2: `CCC_ROLE=executor`
- Pane 3: `CCC_ROLE=reviewer`

Example setup commands:
```bash
# Pane 1 - Planner
CCC_ROLE=planner claude

# Pane 2 - Executor
CCC_ROLE=executor claude

# Pane 3 - Reviewer
CCC_ROLE=reviewer claude
```

The SessionStart hook exports this to `CLAUDE_ENV_FILE` for session persistence.

## Auto-Load Setup

CCC automatically adds a SessionStart hook when you run `ccc install`. This hook detects `CCC_ROLE` at session startup and exports it to `CLAUDE_ENV_FILE` for session persistence. This enables the ccc-interpane skill to auto-load based on the role.

No manual configuration needed - the hook is managed by CCC.

## Integration with CCC Interpane Skill

The team-session skill provides the **role context**, while `ccc-interpane` provides the **messaging mechanism**:

1. CCC creates the 3-pane layout (Go code)
2. Each pane gets its `CCC_ROLE` set
3. SessionStart hook exports role to CLAUDE_ENV_FILE
4. ccc-interpane skill auto-loads based on role
5. Panes communicate via @mentions

See also: `ccc-interpane` skill for inter-pane messaging.

## Troubleshooting

| Issue | Cause | Solution |
|-------|-------|----------|
| Pane title empty | Not set during creation | `tmux select-pane -t :.1 -T "Planner"` |
| CCC_ROLE empty | Not set before claude start | Restart pane with env var |
| Cannot send to pane | Wrong pane index | Check with `tmux list-panes` |
| Session not found | Wrong session name | Check `tmux list-sessions` |
