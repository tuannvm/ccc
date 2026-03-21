# Opus Architectural Review
## Multi-Pane Tmux Architecture Implementation

**Reviewer**: Claude Opus 4.6 (Architectural Analysis)
**Date**: 2025-03-20
**Status**: ⚠️ Critical Issues Found

---

## Executive Summary

The implementation demonstrates solid architectural foundations with clean interface-based design. However, several critical issues require attention before production use:

1. **Circular Import Risk** - The `Session` interface creates tight coupling
2. **Incomplete Integration** - Stubs exist but aren't wired to main package
3. **State Management** - Team session state lacks proper lifecycle management
4. **Error Propagation** - Runtime implementations return stub errors

---

## Detailed Analysis

### ✅ Strengths

#### 1. Interface-Based Design (Excellent)
The `SessionRuntime`, `MessageRouter`, and `HookRouter` interfaces provide clean abstractions:

```go
// session/runtime.go
type SessionRuntime interface {
    EnsureLayout(session Session, workDir string) error
    GetRoleTarget(session Session, role PaneRole) (string, error)
    GetDefaultTarget(session Session) (string, error)
    StartClaude(session Session, workDir string) error
}
```

**Assessment**: This enables the strategy pattern successfully. Different session types can have different implementations without changing core logic.

#### 2. Registry Pattern (Good)
```go
var RuntimeRegistry = make(map[SessionKind]SessionRuntime)

func RegisterRuntime(kind SessionKind, runtime SessionRuntime) {
    RuntimeRegistry[kind] = runtime
}
```

**Assessment**: Clean dependency injection. Allows runtime extensibility.

#### 3. Separation of Concerns (Excellent)
- Session management → `session/` package
- Routing logic → `routing/` package
- CLI commands → `commands/` package

**Assessment**: Each package has a single, well-defined responsibility.

---

### ⚠️ Critical Issues

#### Issue 1: Circular Import via Session Interface (SEVERE)

**Location**: `types.go` vs `session/` package

The `Session` interface is defined in `session/` package but `SessionInfo` (main package) implements it:

```go
// session/runtime.go
type Session interface {
    GetName() string
    GetPath() string
    // ...
}

// main/types.go
func (s *SessionInfo) GetName() string { ... }
func (s *SessionInfo) GetPath() string { ... }
```

**Problem**: The main package imports `session/` package, but `session/` needs to reference main's types. This creates a dependency cycle.

**Recommendation**:
1. Move `Session` interface to a separate `session/types.go` that both packages import
2. OR use dependency inversion: define interface in main, have session package import it
3. OR use adapters/forwarding functions to break the cycle

#### Issue 2: Stub Implementations Return Errors (HIGH)

**Location**: `session/runtime.go`

```go
func ensureSinglePaneLayout(session Session, workDir string) error {
    // TODO: Call main.ensureProjectWindow + main.switchSessionInWindow
    return fmt.Errorf("not implemented: ensureSinglePaneLayout")
}
```

**Problem**: These stubs will cause runtime failures when called. The SinglePaneRuntime claims to wrap existing logic but doesn't actually work.

**Recommendation**: Either:
1. Implement the actual wrappers before registering the runtime
2. OR mark as "not yet supported" and return nil (no-op)
3. OR use a feature flag to disable multi-pane until fully implemented

#### Issue 3: Team Session State Lifecycle (MEDIUM)

**Location**: `session/team_runtime.go`

```go
var ActiveTeamSessions = make(map[string]*TeamWindowState)
var teamSessionsMutex sync.RWMutex
```

**Problem**:
- No cleanup mechanism when sessions are deleted
- State can become stale if tmux windows are killed externally
- No persistence across CCC restarts

**Recommendation**:
1. Add state refresh/reconciliation logic
2. Implement cleanup on session delete
3. Persist state to config for recovery

#### Issue 4: Type Conversion Overhead (LOW)

**Location**: `types.go`

```go
func (s *SessionInfo) GetPanes() map[session.PaneRole]*session.PaneInfo {
    result := make(map[session.PaneRole]*session.PaneInfo)
    for role, info := range s.Panes {
        result[role] = &session.PaneInfo{
            ClaudeSessionID: info.ClaudeSessionID,
            PaneID:          info.PaneID,
            Role:            info.Role,
        }
    }
    return result
}
```

**Problem**: Creates new map on every call. For frequently-called operations, this is wasteful.

**Recommendation**: Consider using the same type or adding a method that returns internal state directly.

---

### 🔍 Extensibility Analysis

#### Adding 4-Pane Layout

**Current Design**: ✅ Excellent

```go
// Just add to layout_registry.go
"team-4pane": {
    Name: "team-4pane",
    Panes: []PaneSpec{
        {ID: "planner", Index: 0, Prefixes: []string{"/p"}},
        {ID: "executor", Index: 1, Prefixes: []string{"/e"}},
        {ID: "reviewer", Index: 2, Prefixes: []string{"/r"}},
        {ID: "observer", Index: 3, Prefixes: []string{"/o"}},
    },
},
```

**Assessment**: No code changes needed. The `TeamRuntime` should handle any number of panes automatically.

#### Adding Grid Layout (2x2)

**Current Design**: ⚠️ Needs Work

The `TeamRuntime` assumes horizontal layout:
```go
// split-window -h creates horizontal splits
exec.Command(r.tmuxPath, "split-window", "-h", "-t", target).Run()
```

**Problem**: 2x2 grid requires different split commands and layout selection.

**Recommendation**: Add `LayoutStrategy` to `LayoutSpec`:
```go
type LayoutStrategy string
const (
    LayoutHorizontal LayoutStrategy = "horizontal"  // |-|-|
    LayoutGrid       LayoutStrategy = "grid-2x2"    // 2x2 grid
)

type LayoutSpec struct {
    Name     string
    Panes    []PaneSpec
    Strategy LayoutStrategy
}
```

Then branch in `TeamRuntime.EnsureLayout()` based on strategy.

---

### 📐 Code Reusability

#### Single-Pane Logic Duplication: ✅ Avoided

The `SinglePaneRuntime` correctly wraps existing functions rather than duplicating them:

```go
func (r *SinglePaneRuntime) EnsureLayout(session Session, workDir string) error {
    return ensureSinglePaneLayout(session, workDir)
}
```

**Assessment**: Good design, once the stubs are implemented.

---

## Recommendations

### Immediate (Before Production)

1. **Fix Circular Import** (SEVERE)
   - Move `Session` interface to shared package
   - OR restructure to avoid session package importing main

2. **Implement Runtime Wrappers** (HIGH)
   - Wire up `ensureSinglePaneLayout()` to actual main package functions
   - OR disable multi-pane feature until ready

3. **Add State Cleanup** (MEDIUM)
   - Implement `TeamRuntime.Cleanup()` method
   - Call it when sessions are deleted

### Future Improvements

1. **Layout Strategy Pattern**
   - Abstract layout creation into separate strategies
   - Enables grids, tabs, custom layouts

2. **Configuration-Driven Layouts**
   - Allow users to define custom layouts in config
   - Validate layouts at load time

3. **Health Checking**
   - Add `HealthCheck()` to `SessionRuntime`
   - Detect and recover from stale state

---

## Conclusion

**Architecture Grade**: B+ (Solid foundation, needs integration work)

The interface-based design is well-thought-out and extensible. The primary concerns are:
1. Resolving the circular import situation
2. Completing the stub implementations
3. Adding proper state lifecycle management

Once these are addressed, the architecture will be production-ready.

**Recommendation**: Complete the integration (Phases 4-7) before architectural review can pass fully. The abstractions are correct; the execution is incomplete.
