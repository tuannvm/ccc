# Codex Review: Multi-Pane Tmux Architecture

**Reviewer:** Codex (GPT-5.3)
**Date:** 2026-03-20
**Branch:** worktree-multi-bot
**Commit:** 9274de6 "Use dedicated 'ccc-team' tmux session for team sessions"

---

## Executive Summary

The multi-pane tmux architecture implementation demonstrates **solid conceptual design** but contains **several critical bugs** that will break production use. The 3-pane layout creation logic is fundamentally sound, but routing and session identity issues create significant reliability risks.

**Overall Grade:** C
**Go/No-Go:** **NO-GO** - Critical issues must be fixed before Phases 6-7

---

## Critical Issues (Must Fix)

### 1. **CRITICAL: Role Prefix Routing Broken**
**File:** `routing/message.go:44-51`
**Severity:** Critical - Breaks primary UX path

**Issue:**
```go
prefix := strings.ToLower(fields[0])  // Keeps leading slash: "/planner"
prefixKey := strings.TrimPrefix(strings.ToLower(p), "/")  // Removes slash: "planner"
prefixMap[prefixKey] = session.PaneRole(pane.ID)  // Map key has no slash
if role, ok := prefixMap[prefix]; ok {  // Lookup with slash - NEVER MATCHES
```

**Impact:** Commands like `/planner`, `/executor`, `/reviewer` fail silently and route to executor. The entire multi-pane routing UX is non-functional.

**Fix:** Normalize consistently:
```go
prefix := strings.TrimPrefix(strings.ToLower(fields[0]), "/")  // Strip slash from input
```

### 2. **CRITICAL: First-Run Failure When Tmux Server Not Running**
**File:** `session/team_runtime.go:245-248`
**Severity:** Critical - Blocks new users

**Issue:**
```go
cmd := exec.CommandContext(ctx, r.tmuxPath, "list-sessions", "-F", "#{session_name}")
out, err := cmd.Output()
if err != nil {
    return fmt.Errorf("failed to list sessions: %w", err)  // Exits instead of creating session
}
```

**Impact:** First-time users with no tmux server get error instead of session creation.

**Fix:** Check error for "no server running" condition and proceed to creation:
```go
if err != nil && !strings.Contains(err.Error(), "no server running") {
    return fmt.Errorf("failed to list sessions: %w", err)
}
// Fall through to session creation
```

### 3. **HIGH: Session Identity Inconsistency**
**Files:** `team_commands.go:77`, `team_routing.go:110`
**Severity:** High - UX confusion and operational issues

**Issue:**
- User provides `<name>` in `ccc team new <name>` (line 77)
- But session identity is derived from `path basename` (line 110-114)
- Commands like `list/attach/stop/delete` search by derived name, not provided name

**Impact:** User-facing names are unreliable. Two sessions in same directory collide.

**Fix:** Add explicit `SessionName` field to `SessionInfo` and use it consistently:
```go
type SessionInfo struct {
    // ...
    SessionName  string `json:"session_name"`  // Explicit user-provided name
    // ...
}
```

### 4. **HIGH: CCC_ROLE Environment Variable Not Reliable**
**File:** `session/team_runtime.go:93`
**Severity:** High - Breaks hook-based role attribution

**Issue:**
```go
runCmd := fmt.Sprintf("CCC_ROLE=%s cd %s && ccc run", role, quotedWorkDir)
```

**Impact:** Environment variable applies to `cd` command, not `ccc run`. Role inference in hooks collapses to default executor.

**Fix:**
```go
runCmd := fmt.Sprintf("cd %s && CCC_ROLE=%s ccc run", quotedWorkDir, role)
// OR
runCmd := fmt.Sprintf("export CCC_ROLE=%s; cd %s && ccc run", role, quotedWorkDir)
```

### 5. **HIGH: Hook Router Not Integrated**
**File:** `routing/hook.go:88`
**Severity:** High - Incomplete Phase 4-5 implementation

**Issue:** `GetHookRouter()` is defined but never called. Hook-based role attribution doesn't work.

**Impact:** Multi-pane attribution from hooks is non-functional despite being in scope.

**Fix:** Wire hook router into actual hook processing path.

---

## Medium Issues

### 6. **Error Suppression in Window Switching**
**File:** `team_routing.go:104`
**Severity:** Medium

**Issue:**
```go
exec.Command(tmuxPath, "select-window", "-t", target).Run()  // Error ignored
return nil  // Always returns success
```

**Impact:** False success during partial failures. Harder debugging.

### 7. **Dead Code and Duplication**
**File:** `team_routing.go:135-194`
**Severity:** Medium

**Issue:** `isTeamSessionCommand()` and `parseTeamCommand()` duplicate router logic but appear unused.

**Impact:** Code drift risk, confusion.

### 8. **In-Memory State Not Synchronized**
**File:** `session/team_runtime.go:276-325`
**Severity:** Medium

**Issue:** `ActiveTeamSessions` tracking exists but isn't clearly synchronized with `SessionInfo.Panes`.

**Impact:** State drift between runtime and config.

---

## Positive Findings

### Correctness (Where It Works)
- ✅ 3-pane layout creation sequence is correct (split-h, select-pane, split-h, select-layout)
- ✅ Role-to-index mapping is consistent (Planner→0, Executor→1, Reviewer→2)
- ✅ Shell quoting prevents injection in paths

### Safety
- ✅ `ActiveTeamSessions` protected by `sync.RWMutex`
- ✅ Timeouts on all tmux operations (5-10s)
- ✅ Proper cleanup in `StopTeam` and `DeleteTeam`

### Architecture
- ✅ Clean separation: session/, routing/, main packages
- ✅ Interface-based design (SessionRuntime, MessageRouter, HookRouter)
- ✅ Backward compatible (standard sessions unaffected)

---

## Recommendations

### Immediate Actions (Before Phases 6-7)
1. **Fix router normalization bug** - 5 minute change, unblocks entire feature
2. **Fix tmux server bootstrap** - Handle "no server" case gracefully
3. **Fix CCC_ROLE propagation** - Ensure hooks can infer role correctly
4. **Add explicit session naming** - Remove path-based identity inference
5. **Wire hook router integration** - Complete Phase 4-5 scope

### Testing Strategy
1. Add integration tests for:
   - Team creation on fresh tmux (no server running)
   - `/planner`, `/executor`, `/reviewer` routing
   - Session identity collision (same dir, different names)
   - Partial failure behaviors (pane missing, tmux dead)

### Code Cleanup
1. Remove dead parsing helpers in `team_routing.go`
2. Add constants for hardcoded strings ("ccc-team")
3. Synchronize in-memory state with persisted state

---

## Grade Breakdown

| Dimension | Grade | Notes |
|-----------|-------|-------|
| Correctness | D | Routing bug breaks primary feature |
| Safety | B+ | Good concurrency, timeout handling |
| Completeness | C | Hook router not integrated |
| Practicality | C | Naming confusion, unreliable UX |
| Maintainability | B+ | Clean architecture, some dead code |
| Edge Cases | C | First-run failure, error suppression |

**Overall:** C

---

## Go/No-Go Recommendation

**🚨 NO-GO** for Phases 6-7

**Rationale:**
- Critical routing bug makes feature non-functional
- First-run failure blocks new users
- Session identity issues create operational confusion
- Hook router integration incomplete (Phase 4-5 scope)

**Required Actions Before Proceeding:**
1. Fix prefix routing normalization (5 min)
2. Fix tmux server bootstrap (10 min)
3. Fix CCC_ROLE environment variable (5 min)
4. Add explicit session naming (30 min)
5. Integrate hook router (20 min)

**Estimated Time to Go:** ~2 hours

---

## Sources

- Code analysis via Codex CLI (GPT-5.3)
- Local codebase inspection at `/home/tuannvm/Projects/cli/ccc/.claude/worktrees/multi-bot`
- Implementation comparison with architectural requirements in docs/
