# Team Session Known Issues

**Date:** 2026-03-21  
**Status:** ✅ All Issues Fixed + Improvements Applied  
**Priority:** High

## Issues Fixed

### 1. ✅ FIXED — Duplicate pane ClaudeSessionID

**Files:** `pkg/lookup/persist.go:85-91`

**Problem:**
- Persist logic sets the target pane ID but doesn't clear that ID from sibling panes
- Lookup then iterates maps and can pick the wrong pane/role prefix nondeterministically

**Fix Applied:**
```go
// pkg/lookup/persist.go:86-91 - Now clears from sibling panes
// Clear this claudeSessionID from all OTHER panes to prevent ambiguity
for otherRole, otherPane := range sessInfo.Panes {
    if otherRole != role && otherPane != nil && otherPane.ClaudeSessionID == claudeSessionID {
        otherPane.ClaudeSessionID = ""
        hookLog("cleared duplicate claude_session_id=%s from sibling pane role=%s", claudeSessionID, otherRole)
    }
}
```

**Note:** Map iteration while modifying struct fields is safe in Go (no key addition/removal).

---

### 2. ✅ FIXED — Retry path loses role prefix

**Files:** `pkg/hooks/transcript.go:315-330`

**Problem:**
- `handleStopRetry` calls `deliverUnsentTexts(..., "")` with empty claudeSessionID
- Fallback role resolution may fail in subprocess context
- Causes missing `[Planner]`/`[Executor]`/`[Reviewer]` prefix on resend

**Fix Applied:**
```go
// pkg/hooks/transcript.go:315-330 - Added improved transcript path inference fallback
// Fallback 1: Try to infer role from transcript path (improved pattern matching)
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

**Improvement:** Transcript path inference now uses case-insensitive substring matching instead of strict suffix matching, handling more naming patterns:
- `session-planner.jsonl`, `session_planner.jsonl`
- `planner.jsonl`, `planner-session.jsonl`
- `session.planner.jsonl`

---

### 3. ✅ FIXED — Inconsistent tmux name sanitization

**Files:** `pkg/session/team_runtime.go:148`, `pkg/listen/team.go:80,101`

**Problem:**
- One path maps `.` → `_`, another maps `.` → `__`
- Routing sometimes uses unsanitized session name
- Dotted team names can break targeting/switch checks

**Fix Applied:**
- `pkg/session/team_runtime.go:148`: Changed to use `__` (double underscore)
- `pkg/listen/team.go:80,101`: Added `tmuxSafeName()` sanitization

```go
// pkg/session/team_runtime.go:148 - Now uses double underscore
return strings.ReplaceAll(name, ".", "__")

// pkg/listen/team.go:80 - Now sanitizes
sanitizedName := tmuxSafeName(sessionName)
target := "ccc-team:" + sanitizedName
```

---

## Additional Improvements (Post-Review)

### 4. ✅ ADDED — Input validation for empty session names

**Files:** `pkg/listen/team.go:82,104`

**Improvement:** Added defensive checks for empty session names with clear error messages.

---

### 5. ✅ FIXED — Error handling in switchToTeamWindow

**Files:** `pkg/listen/team.go:106`

**Improvement:** Previously ignored tmux errors. Now returns error for proper debugging.

```go
// Before: exec.Command(tmuxPath, "select-window", "-t", target).Run()
// After:
if err := exec.Command(tmuxPath, "select-window", "-t", target).Run(); err != nil {
    return fmt.Errorf("failed to select window: %w", err)
}
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

4. **Various transcript naming patterns:**
   ```bash
   # Verify role inference works with different naming patterns
   ```

---

## References

- Original review: PR #11
- Related: `docs/multi-bot-design.md`, `docs/tmux-architecture.md`
- Commit 1: Initial bug fixes (2026-03-21)
- Commit 2: Review improvements (2026-03-21)
