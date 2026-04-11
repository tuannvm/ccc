package session

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
// Returns nil if no runtime is registered for the kind.
// Note: single-pane sessions do not use the runtime system — they call
// tmux.SwitchSessionInWindow directly. Only team sessions use GetRuntime.
func GetRuntime(kind SessionKind) SessionRuntime {
	return RuntimeRegistry[kind]
}

// init registers the built-in runtime implementations
func init() {
	// Single-pane sessions bypass the runtime system entirely.
	// They use tmux.SwitchSessionInWindow directly via the listen package.
	RegisterRuntime(SessionKindTeam, &TeamRuntime{})
}
