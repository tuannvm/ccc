# Consolidated Technical Review: Multi-Pane Tmux Architecture

**Date:** 2026-03-20
**Branch:** worktree-multi-bot
**Commit:** 9274de6 "Use dedicated 'ccc-team' tmux session for team sessions"
**Reviewers:** Codex (GPT-5.3), Gemini 2.5, Claude (Human Analysis)

---

## Executive Summary

Three independent AI models reviewed the multi-pane tmux architecture implementation. **Consensus identifies critical bugs that block production use**, but the core architecture is sound.

**Overall Grades:**
- **Codex:** C (Critical bugs found)
- **Gemini:** A- (Solid implementation, minor cleanup needed)
- **Claude (Human):** C+ (Functional but needs fixes)

**Consensus Grade:** C
**Go/No-Go:** **NO-GO** - Critical issues must be addressed

---

## Critical Issues (All Reviewers Agree)

### 1. 🔴 CRITICAL: Role Prefix Routing Broken
**Discovered by:** Codex
**Confirmed by:** Human analysis

**File:** `routing/message.go:44-51`

```go
prefix := strings.ToLower(fields[0])  // "/planner"
prefixKey := strings.TrimPrefix(strings.ToLower(p), "/")  // "planner"
prefixMap[prefixKey] = session.PaneRole(pane.ID)  // Key: "planner"
if role, ok := prefixMap[prefix]; ok {  // Lookup with "/planner" - FAILS
```

**Impact:** Commands `/planner`, `/executor`, `/reviewer` don't work. Primary UX path is broken.

**Fix:** 1 line change
```go
prefix := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
```

**Priority:** P0 - Must fix before any use

---

### 2. 🔴 CRITICAL: First-Run Failure When Tmux Server Not Running
**Discovered by:** Codex
**Confirmed by:** Human analysis

**File:** `session/team_runtime.go:245-248`

```go
cmd := exec.CommandContext(ctx, r.tmuxPath, "list-sessions", "-F", "#{session_name}")
out, err := cmd.Output()
if err != nil {
    return fmt.Errorf("failed to list sessions: %w", err)  // Exits
}
```

**Impact:** New users with no tmux server get error instead of session creation.

**Fix:** Handle "no server running" as non-fatal
```go
if err != nil && !strings.Contains(err.Error(), "no server running") {
    return fmt.Errorf("failed to list sessions: %w", err)
}
// Proceed to create session
```

**Priority:** P0 - Blocks new users

---

## High-Priority Issues

### 3. 🟠 HIGH: CCC_ROLE Environment Variable Not Reliably Propagated
**Discovered by:** Codex
**Confirmed by:** Human analysis

**File:** `session/team_runtime.go:93`

```go
runCmd := fmt.Sprintf("CCC_ROLE=%s cd %s && ccc run", role, quotedWorkDir)
```

**Issue:** Environment variable applies to `cd` command, not `ccc run`. Hook-based role inference fails.

**Fix:**
```go
runCmd := fmt.Sprintf("cd %s && CCC_ROLE=%s ccc run", quotedWorkDir, role)
```

**Priority:** P0 - Breaks hook attribution

---

### 4. 🟠 HIGH: Session Identity Inconsistency
**Discovered by:** Codex
**Severity:** High - UX confusion

**Files:** `team_commands.go:77`, `team_routing.go:110`

**Issue:** User provides `<name>` in `ccc team new <name>`, but session identity is derived from path basename. Commands search by derived name, not provided name.

**Impact:** User-facing names unreliable. Two sessions in same directory collide.

**Fix:** Add explicit `SessionName` field to `SessionInfo`

**Priority:** P1 - Significant UX issue

---

### 5. 🟠 HIGH: Hook Router Not Integrated
**Discovered by:** Codex
**Severity:** High - Incomplete Phase 4-5

**File:** `routing/hook.go:88`

**Issue:** `GetHookRouter()` defined but never called. Hook-based role attribution doesn't work.

**Impact:** Phase 4-5 scope incomplete.

**Fix:** Wire hook router into actual hook processing path

**Priority:** P1 - Completes Phase 4-5

---

## Medium-Priority Issues

### 6. 🟡 MEDIUM: Error Suppression in Window Switching
**Discovered by:** Codex

**File:** `team_routing.go:104`

```go
exec.Command(tmuxPath, "select-window", "-t", target).Run()  // Error ignored
return nil
```

**Impact:** False success during partial failures

**Priority:** P1 - Debugging difficulty

---

### 7. 🟡 MEDIUM: Dead Code and Duplication
**Discovered by:** Gemini
**Severity:** Medium

**File:** `team_routing.go:135-194`

**Issue:** `isTeamSessionCommand()` and `parseTeamCommand()` duplicate router logic but appear unused.

**Impact:** Code drift risk

**Priority:** P2 - Code cleanliness

---

### 8. 🟡 MEDIUM: No Progressive Disclosure (UX)
**Discovered by:** Gemini
**Severity:** Medium

**Issue:** All 3 panes visible immediately. Intimidating for new users.

**Recommendation:** Implement "Focus Mode" to zoom into single pane

**Priority:** P2 - UX improvement

---

## Positive Findings (All Reviewers Agree)

### Architecture ✅
- Clean separation: session/, routing/, main packages
- Interface-based design (SessionRuntime, MessageRouter, HookRouter)
- Backward compatible (standard sessions unaffected)
- Extensible for future layouts (4-pane, custom)

### Correctness ✅
- 3-pane layout creation sequence is correct
- Role-to-index mapping consistent (Planner→0, Executor→1, Reviewer→2)
- Shell quoting prevents injection in paths

### Safety ✅
- `ActiveTeamSessions` protected by `sync.RWMutex`
- Timeouts on all tmux operations (5-10s)
- Proper cleanup in `StopTeam` and `DeleteTeam`

### Completeness ✅
- Full CLI lifecycle: new, list, attach, start, stop, delete
- Routing infrastructure complete (MessageRouter, HookRouter)
- Telegram integration working

---

## Disagreements Between Reviewers

### Issue: Hook Router Integration
- **Codex:** Says not integrated, critical issue
- **Gemini:** Says infrastructure exists, minor cleanup
- **Reality:** `GetHookRouter()` exists but call sites not found

**Resolution:** Agree with Codex - need to verify call sites exist or add them

---

### Issue: Overall Grade
- **Codex:** C (critical bugs)
- **Gemini:** A- (solid, minor cleanup)
- **Claude:** C+ (functional but needs fixes)

**Resolution:** Consensus is C - critical routing bug dominates assessment

---

## Prioritized Action Items

### Before Phases 6-7 (P0 - Must Fix)
1. **Fix prefix routing normalization** (5 min)
   - File: `routing/message.go:44`
   - Change: `prefix := strings.TrimPrefix(strings.ToLower(fields[0]), "/")`
   - Test: `/planner`, `/executor`, `/reviewer` commands

2. **Fix tmux server bootstrap** (10 min)
   - File: `session/team_runtime.go:245`
   - Handle "no server running" as non-fatal
   - Test: Fresh tmux install, no server running

3. **Fix CCC_ROLE environment variable** (5 min)
   - File: `session/team_runtime.go:93`
   - Change: `cd %s && CCC_ROLE=%s ccc run`
   - Test: Verify hooks receive correct role

### Before Production (P1 - Should Fix)
4. **Add explicit session naming** (30 min)
   - Add `SessionName` field to `SessionInfo`
   - Use consistently across all commands
   - Test: Multiple sessions in same directory

5. **Wire hook router integration** (20 min)
   - Find where hooks are processed
   - Add `GetHookRouter()` call
   - Test: Hook events with correct role attribution

6. **Fix error suppression** (15 min)
   - File: `team_routing.go:104`
   - Propagate `select-window` errors
   - Test: Invalid window names, missing sessions

### Code Cleanup (P2 - Nice to Have)
7. **Remove dead code** (15 min)
   - Remove `isTeamSessionCommand()`, `parseTeamCommand()`
   - Verify no call sites exist

8. **Add constants** (10 min)
   - Replace "ccc-team" with `const TeamSessionName = "ccc-team"`
   - Replace hardcoded role prefixes

9. **UX improvements** (2-4 hours)
   - Add `ccc team doctor` command
   - Add `/help` command in team sessions
   - Add confirmation prompts for destructive actions
   - Consider "Focus Mode" for single-pane view

---

## Testing Recommendations

### Integration Tests (Must Have)
```go
func TestTeamSessionCreation(t *testing.T) {
    // Test: Fresh tmux (no server)
    // Expected: Session created successfully
}

func TestRolePrefixRouting(t *testing.T) {
    // Test: /planner, /executor, /reviewer
    // Expected: Routes to correct pane
}

func TestSessionIdentityCollision(t *testing.T) {
    // Test: Two sessions, same directory, different names
    // Expected: Both sessions accessible by name
}
```

### Manual Testing Checklist
- [ ] Create team session on fresh tmux install
- [ ] Test `/planner`, `/executor`, `/reviewer` routing
- [ ] Test default routing (no prefix → executor)
- [ ] Test `ccc team list` shows correct status
- [ ] Test `ccc team attach` with `--role` flag
- [ ] Test `ccc team stop` kills Claude processes
- [ ] Test `ccc team delete` removes window and config
- [ ] Test hook events carry correct role
- [ ] Test multiple sessions in same directory
- [ ] Test partial failure behaviors (pane missing, tmux dead)

---

## Go/No-Go Recommendation

### Current Status: 🚨 NO-GO

**Rationale:**
1. **Critical routing bug** makes primary feature non-functional
2. **First-run failure** blocks new users
3. **Environment variable bug** breaks hook attribution
4. **Session identity issues** create operational confusion
5. **Hook router not integrated** - Phase 4-5 incomplete

### Path to GO: ~2-3 Hours

**Immediate Actions (P0):**
1. Fix prefix routing (5 min)
2. Fix tmux bootstrap (10 min)
3. Fix CCC_ROLE propagation (5 min)
4. Add integration tests (30 min)
5. Manual testing (30 min)

**Before Production (P1):**
6. Add explicit session naming (30 min)
7. Wire hook router (20 min)
8. Fix error suppression (15 min)

**Estimated Timeline:**
- **Today:** P0 fixes (2 hours) → Feature functional
- **This Week:** P1 fixes (1 hour) → Production ready
- **Next Sprint:** P2 UX improvements → Excellent UX

---

## Conclusion

The multi-pane tmux architecture has **solid foundations** but contains **critical implementation bugs** that block production use. The good news: all critical issues are **quick fixes** (5-10 minutes each).

**Recommendation:**
1. **Stop:** Don't proceed to Phases 6-7 yet
2. **Fix:** Address P0 issues (2-3 hours)
3. **Test:** Verify fixes with integration tests
4. **Proceed:** Continue to Phases 6-7 with confidence

**Key Takeaway:** The architecture is sound. The bugs are superficial and easily fixed. Once corrected, this will be a robust, production-ready multi-pane system.

---

## Reviewer Perspectives

### Codex (GPT-5.3)
**Focus:** Implementation safety, correctness
**Grade:** C
**Quote:** "Solid conceptual design but critical bugs break production use. Fix P0 issues first."

### Gemini 2.5
**Focus:** User experience, practicality
**Grade:** A-
**Quote:** "Implementation is solid and functional. Minor cleanup needed, but ready for testing phase."

### Claude (Human Analysis)
**Focus:** Architecture, integration, completeness
**Grade:** C+
**Quote:** "Core architecture is excellent. Critical routing bug dominates assessment. Quick fixes needed."

---

## Next Steps

1. **Create tracking issue** for P0 fixes
2. **Assign priority** to each fix
3. **Create PR** with P0 fixes only
4. **Add integration tests** to prevent regression
5. **Update documentation** with known issues
6. **Schedule review** after P0 fixes complete

**Target:** Re-review in 1 week with all P0 issues resolved

---

## Sources

- Codex analysis via `/codex` tool (GPT-5.3)
- Gemini analysis via `/gemini` tool (Gemini 2.5)
- Human code review at `/home/tuannvm/Projects/cli/ccc/.claude/worktrees/multi-bot`
- Architectural requirements in `docs/` directory
