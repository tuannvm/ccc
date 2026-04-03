package session

import (
	"fmt"
)

// Session is a forward declaration to avoid circular imports
// The actual SessionInfo is defined in the main package
type Session interface {
	GetName() string
	GetPath() string
	GetTopicID() int64
	GetProviderName() string
	GetType() SessionKind
	GetLayoutName() string
	GetPanes() map[PaneRole]*PaneInfo
}

// SessionRuntime defines the interface for creating and managing tmux sessions
// Different implementations support different layouts (single-pane, multi-pane team, etc.)
type SessionRuntime interface {
	// EnsureLayout creates or verifies the tmux layout for this session type
	EnsureLayout(session Session, workDir string) error

	// GetRoleTarget returns the tmux target for a specific pane role
	// For single-pane sessions, this returns the window target regardless of role
	// For multi-pane sessions, this returns the specific pane target (e.g., "ccc:session.0")
	GetRoleTarget(session Session, role PaneRole) (string, error)

	// GetDefaultTarget returns the tmux target for the default input pane
	GetDefaultTarget(session Session) (string, error)

	// StartClaude launches Claude in the appropriate pane(s)
	StartClaude(session Session, workDir string) error
}

// RuntimeRegistry maps session kinds to their runtime implementations
var RuntimeRegistry = make(map[SessionKind]SessionRuntime)

// RegisterRuntime registers a runtime implementation for a session kind
func RegisterRuntime(kind SessionKind, runtime SessionRuntime) {
	RuntimeRegistry[kind] = runtime
}

// GetRuntime retrieves the runtime implementation for a session kind
// Returns nil if no runtime is registered for the kind
func GetRuntime(kind SessionKind) SessionRuntime {
	return RuntimeRegistry[kind]
}

// init registers the built-in runtime implementations
func init() {
	// Register single-pane runtime (wraps existing logic)
	RegisterRuntime(SessionKindSingle, &SinglePaneRuntime{})
	// Register team runtime (multi-pane layout)
	RegisterRuntime(SessionKindTeam, &TeamRuntime{})
}

// SinglePaneRuntime implements SessionRuntime for standard single-pane sessions.
// It is registered for completeness but not invoked at runtime — single-pane
// sessions are handled directly by the main package's tmux functions. See the
// stub function documentation below for details.
type SinglePaneRuntime struct{}

// EnsureLayout creates or verifies a single-pane tmux window
// This wraps the existing switchSessionInWindow function
func (r *SinglePaneRuntime) EnsureLayout(session Session, workDir string) error {
	// The main package's switchSessionInWindow handles this
	// We'll call it via the main package function
	return ensureSinglePaneLayout(session, workDir)
}

// GetRoleTarget returns the window target for any role in single-pane sessions
// Since there's only one pane, all roles map to the same target
func (r *SinglePaneRuntime) GetRoleTarget(session Session, role PaneRole) (string, error) {
	// For single-pane sessions, return the window target
	return getSinglePaneTarget(session)
}

// GetDefaultTarget returns the window target
func (r *SinglePaneRuntime) GetDefaultTarget(session Session) (string, error) {
	return getSinglePaneTarget(session)
}

// StartClaude starts Claude in the single pane
func (r *SinglePaneRuntime) StartClaude(session Session, workDir string) error {
	return startClaudeInPane(session, workDir)
}

// SinglePaneRuntime is registered in RuntimeRegistry but not currently invoked.
// Single-pane sessions are managed directly by the main package using tmux commands
// (switchSessionInWindow, ensureProjectWindow, getCccWindowTarget), which cannot be
// called from this package due to Go's circular import restrictions. The SessionRuntime
// abstraction exists for extensibility — if future session types are added, they follow
// the pattern established by TeamRuntime. Single-pane sessions bypass this interface.
//
// These stubs satisfy the SessionRuntime interface but will return errors if ever called.
// This is intentional: the caller (main package) should use its own tmux functions directly
// for single-pane sessions rather than routing through this indirection.

func ensureSinglePaneLayout(session Session, workDir string) error {
	return fmt.Errorf("single-pane layout is managed directly by the main package; use switchSessionInWindow instead")
}

func getSinglePaneTarget(session Session) (string, error) {
	return "", fmt.Errorf("single-pane target is managed directly by the main package; use getCccWindowTarget instead")
}

func startClaudeInPane(session Session, workDir string) error {
	return fmt.Errorf("single-pane Claude startup is managed directly by the main package; use switchSessionInWindow instead")
}
