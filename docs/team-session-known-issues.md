# Team Session Known Issues

**Date:** 2026-03-21  
**Status:** ✅ All Issues Fixed  
**Priority:** High

## Issues Fixed

### 1. ✅ FIXED — Duplicate pane ClaudeSessionID

**Files:** `session_persist.go:73`, `hooks.go:296`

**Problem:**
- Persist logic sets the target pane ID but doesn't clear that ID from sibling panes
- Lookup then iterates maps and can pick the wrong pane/role prefix nondeterministically

**Fix Applied:**
```go
// session_persist.go:85-91 - Now clears from sibling panes
// Clear this claudeSessionID from all OTHER panes to prevent ambiguity
for otherRole, otherPane := range sessInfo.Panes {
    if otherRole != role && otherPane != nil && otherPane.ClaudeSessionID == claudeSessionID {
        otherPane.ClaudeSessionID = ""
        hookLog("cleared duplicate claude_session_id=%s from sibling pane role=%s", claudeSessionID, otherRole)
    }
}
```

---

### 2. ✅ FIXED — Retry path loses role prefix

**Files:** `hooks.go:542`, `hooks.go:315`

**Problem:**
- `handleStopRetry` calls `deliverUnsentTexts(..., "")` with empty claudeSessionID
- Fallback role resolution may fail in subprocess context
- Causes missing `[Planner]`/`[Executor]`/`[Reviewer]` prefix on resend

**Fix Applied:**
```go
// hooks.go:315-330 - Added transcript path inference fallback
// Fallback 1: Try to infer role from transcript path
if rolePrefix == "" && transcriptPath != "" {
    role := inferRoleFromTranscriptPathForPrefix(transcriptPath)
    if role != "" {
        rolePrefix = rolePrefixes[role]
    }
}
// Fallback 2: check CCC_ROLE environment variable
if rolePrefix == "" {
    // ... existing CCC_ROLE fallback
}
```

---

### 3. ✅ FIXED — Inconsistent tmux name sanitization

**Files:** `session/team_runtime.go:148`, `session.go:32`, `team_routing.go:80`

**Problem:**
- One path maps `.` → `_`, another maps `.` → `__`
- Routing sometimes uses unsanitized session name
- Dotted team names can break targeting/switch checks

**Fix Applied:**
- `session/team_runtime.go:148`: Changed to use `__` (double underscore)
- `team_routing.go:80, 101`: Added `tmuxSafeName()` sanitization

```go
// session/team_runtime.go:148 - Now uses double underscore
return strings.ReplaceAll(name, ".", "__")

// team_routing.go:80 - Now sanitizes
sanitizedName := tmuxSafeName(sessionName)
target := "ccc-team:" + sanitizedName
```

---

## Testing Verification

After fixes, verify:

1. **Duplicate ClaudeSessionID cleared:**
   ```bash
   # Create team session
   ccc team new test-team
   # Check config - each pane should have unique claude_session_id
   cat ~/.config/ccc/config.json | jq '.team_sessions[] | select(.session_name == "test-team") | .panes | to_entries[] | {role: .key, id: .value.claude_session_id}'
   ```

2. **Role prefix preserved on retry:**
   ```bash
   # Send message, wait for response, check [Planner]/[Executor]/[Reviewer] prefix is present
   ```

3. **Dotted names work:**
   ```bash
   ccc team new my.dotted.project
   # Verify all operations work correctly
   ```

---

## References

- Original review: PR #11
- Related: `docs/multi-bot-design.md`, `docs/tmux-architecture.md`
- Commit: Team session bug fixes (2026-03-21)
