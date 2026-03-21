# Multi-Pane Tmux Architecture Implementation Summary

## Implementation Status

### Completed Phases

#### Phase 0: Code Reorganization ✅
- Created `session/` package directory
- Created `routing/` package directory
- Added new types and interfaces while preserving existing code

#### Phase 1A: Data Model ✅
- Created `session/session_type.go` with:
  - `SessionKind` (single, team)
  - `PaneRole` (planner, executor, reviewer, standard)
  - `PaneInfo` (per-pane metadata)
  - `LayoutSpec` and `PaneSpec` (layout definitions)
- Extended main package's `SessionInfo` with multi-pane fields
- Made `SessionInfo` implement `session.Session` interface

#### Phase 1B: Config Migration ✅
- Extended `Config` struct with:
  - `TeamSessions map[int64]*SessionInfo` (separate from `Sessions`)
  - `IsTeamSession(topicID int64) bool` helper
  - `GetTeamSession()`, `SetTeamSession()`, `DeleteTeamSession()` helpers
- Maintains backward compatibility - existing `Sessions` unchanged

#### Phase 2A: Single-Pane Runtime Wrapper ✅
- Created `session/runtime.go` with:
  - `SessionRuntime` interface with methods:
    - `EnsureLayout()` - creates tmux layout
    - `GetRoleTarget()` - gets tmux target for role
    - `GetDefaultTarget()` - gets default input pane
    - `StartClaude()` - launches Claude in pane(s)
  - `RuntimeRegistry` mapping session kinds to runtimes
  - `SinglePaneRuntime` implementation (wraps existing logic)

#### Phase 2B: Multi-Pane Team Runtime ✅
- Created `session/team_runtime.go` with:
  - `TeamRuntime` implementing `SessionRuntime` for 3-pane sessions
  - 3-pane layout creation (Planner | Executor | Reviewer)
  - `ActiveTeamSessions` state tracking
  - `TeamWindowState` for tracking pane info
  - Pane ID refresh and lookup functions

#### Phase 3A: Message Routing ✅
- Created `routing/message.go` with:
  - `MessageRouter` interface
  - `SinglePaneRouter` - passthrough routing
  - `TeamRouter` - prefix-based routing (`/planner`, `/executor`, `/reviewer`)
  - `GetRouter()` factory function

#### Phase 3B: Hook Routing ✅
- Created `routing/hook.go` with:
  - `HookRouter` interface
  - `SinglePaneHookRouter` - returns standard role
  - `TeamHookRouter` - infers role from transcript path or `CCC_ROLE` env var
  - `GetHookRouter()` factory function

#### Phase 4: Team CLI Commands (Partial) ✅
- Created `commands/team.go` with:
  - `TeamCommands` struct and subcommand handlers
  - Usage/help text
  - Stub implementations for `new`, `list`, `attach`, `stop`, `delete`
  - `validateTeamSession()` helper
  - NOTE: Commands are stubs, not yet wired to main package

### Architecture Highlights

1. **Separation of Concerns**:
   - Session management → `session/` package
   - Message/hook routing → `routing/` package
   - CLI commands → `commands/` package
   - Main logic → main package (preserved)

2. **Interface-Based Design**:
   - `SessionRuntime` for pluggable session layouts
   - `MessageRouter` for pluggable message routing
   - `HookRouter` for pluggable hook routing
   - `Session` interface to avoid circular imports

3. **Extensibility**:
   - New layouts = add to `BuiltinLayouts` map
   - New routing strategies = implement router interface
   - 4-pane layouts possible without core changes

4. **Backward Compatibility**:
   - Single-pane sessions use existing code paths
   - `Sessions` map preserved
   - Team sessions in separate `TeamSessions` map

### Incomplete Phases

#### Phase 4: Team CLI Commands (Integration)
- Stubs exist but need to:
  - Wire up to main package's tmux functions
  - Call `TeamRuntime.EnsureLayout()`
  - Create/configure Telegram topics
  - Save to `config.TeamSessions`

#### Phase 5: Telegram Integration
- Need to modify `commands.go` listen() function:
  - Check `config.IsTeamSession(threadID)` before `getSessionByTopic()`
  - Use `TeamRouter` to parse routing prefixes
  - Route to specific panes instead of single window
  - Prepend `[Planner]`, `[Executor]`, `[Reviewer]` to outgoing messages

#### Phase 6: Inter-Pane Communication
- Need to extend `handleStopHook()` in `hooks.go`:
  - Parse transcript for `@planner`, `@executor`, `@reviewer`
  - Use `TeamHookRouter` to infer source role
  - Route via tmux buffer (`load-buffer` + `paste-buffer`)
  - Add retry queue with exponential backoff
  - Hop count limit to prevent loops

#### Phase 7: Ledger & State Tracking
- Need to update `ledger.go`:
  - Pane-specific ledger keys: `{session-name}-{role}`
  - Separate ledger files per pane
  - Update `markDelivered()` for pane-specific keys
  - Persist undelivered queue to session state

## Files Created/Modified

### New Files
- `session/session_type.go` - Core types
- `session/layout_registry.go` - Built-in layouts
- `session/runtime.go` - Runtime interface and registry
- `session/team_runtime.go` - Multi-pane layout implementation
- `routing/message.go` - Message routing
- `routing/hook.go` - Hook routing
- `commands/team.go` - Team CLI commands

### Modified Files
- `types.go` - Extended `SessionInfo` and `Config` with multi-pane support

## Next Steps for Full Implementation

1. Integrate `TeamRuntime` with main package's tmux functions
2. Wire up team CLI commands to actual tmux/config operations
3. Update `listen()` to check `IsTeamSession()` and use `TeamRouter`
4. Extend `handleStopHook()` for @mention routing between panes
5. Update ledger system for pane-specific tracking
6. Add tests for routing logic
7. Create integration tests for team session lifecycle

## Review Checklist

### Opus (Architectural Review)
- [ ] Interface abstractions are clean
- [ ] Code reusability (no duplication)
- [ ] Extensibility for 4-pane, grid layouts
- [ ] Separation of concerns (session vs routing)

### Codex 5.3 (Implementation Review)
- [ ] Error handling completeness
- [ ] Tmux command safety (buffer handling, quoting)
- [ ] Concurrent access safety (topicWindows mutex)
- [ ] Resource leaks (goroutines, file handles)

### Gemini (Practicality Review)
- [ ] CLI UX is intuitive
- [ ] Documentation completeness
- [ ] Error messages are user-friendly
- [ ] Rollback plan is documented
