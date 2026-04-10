# Action Items for Next PR

**Date**: 2026-03-21
**Status**: Ready for next phase after `worktree-multi-bot` merge

---

## Completed (This PR)

- ✅ Bug #1: Pane ID Collision Bug - Fixed
- ✅ Bug #2: Missing Nil Checks - Fixed
- ✅ Bug #3: Provider Selection Bug - Fixed
- ✅ Bug #4: Role Name Display Bug - Fixed
- ✅ Multi-pane team session architecture (Phases 0-5)
- ✅ Team CLI commands (`ccc team new`, `ccc team ls`)
- ✅ Telegram integration (`/team`, `/planner`, `/executor`, `/reviewer`)
- ✅ Comprehensive documentation

---

## Testing Gaps (P3 - Recommended)

### Unit Tests
1. **pkg/routing/message.go** - Test prefix parsing and routing
2. **pkg/routing/hook.go** - Test role inference from paths
3. **pkg/session/team_runtime.go** - Test tmux commands (with mock)
4. **pkg/lookup/session.go** - Test session finding logic

### Integration Tests
1. Team session creation flow
2. Message routing to correct panes
3. Config migration with TeamSessions preservation

### Concurrency Tests
1. Race conditions on ActiveTeamSessions access
2. Simultaneous team session operations

---

## Documentation (Phase 10)

1. **Update README** with `ccc team` command reference
2. **Document extensibility**: How to add `team-4pane` or `grid-2x2` layouts
3. **Document troubleshooting**: Pane stuck, message not delivered, recovery steps
4. **Architecture diagrams**: Using existing visual layout section
5. **Document ledger partitioning strategy** (for Phase 6)

---

## Code TODOs (Low Priority)

### pkg/session/runtime.go
```go
// Line 92, 97, 102
// TODO: Call main.ensureProjectWindow + main.switchSessionInWindow
// TODO: Call main.getCccWindowTarget
// TODO: Call main.switchSessionInWindow
```
**Context**: Improve tmux window management - currently using basic tmux commands, could integrate with existing window management functions.

### SinglePaneRuntime Stubs (P3)
**Option A**: Implement stub methods in SinglePaneRuntime
**Option B**: Document why not needed (single-pane sessions don't use SessionRuntime interface)

---

## Future Extensibility (Phase 11 - Optional)

1. **Add `team-4pane` layout spec** to registry
2. **Add `grid-2x2` layout spec** (4 panes in 2x2 grid)
3. **Support custom layout definitions** via user config file
4. **Add pane naming customization** (user-defined role names)

---

## Phase 6: Inter-Pane Communication (Next Major Feature)

See `docs/tmux-architecture.md` Phase 6 for details:
- @mention routing between panes
- Message ledger with hop counting
- Pane busy state handling

---

## Priority Summary

| Priority | Item | Type |
|----------|------|------|
| P2 | Unit tests for routing/session (pkg/) | Testing |
| P3 | Integration tests | Testing |
| P3 | Documentation updates | Docs |
| P3 | Code TODOs (window management) | Code |
| P4 | Future extensibility (Phase 11) | Feature |
| **Next** | Phase 6: Inter-pane communication | Feature |
