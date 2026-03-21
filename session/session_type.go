package session

// SessionKind represents the type of session (single-pane or multi-pane team)
type SessionKind string

const (
	SessionKindSingle SessionKind = "single" // Standard single-pane session
	SessionKindTeam   SessionKind = "team"   // Multi-pane team session
)

// PaneRole represents the role of a pane in a multi-pane session
type PaneRole string

const (
	RolePlanner  PaneRole = "planner"  // Creates plans, delegates work
	RoleExecutor PaneRole = "executor" // Executes code, runs commands
	RoleReviewer PaneRole = "reviewer" // Reviews changes, provides feedback
	RoleStandard PaneRole = "standard" // For single-pane sessions
)

// PaneInfo stores information about a single pane in a session
type PaneInfo struct {
	ClaudeSessionID string   `json:"claude_session_id,omitempty"` // Claude session ID for this pane
	PaneID          string   `json:"pane_id,omitempty"`           // Tmux pane ID (%1, %2, etc.)
	Role            PaneRole `json:"role"`                        // Role of this pane
}

// PaneSpec defines a pane in a layout specification
type PaneSpec struct {
	ID         string   // Unique identifier (e.g., "planner", "executor")
	Index      int      // Pane index (0, 1, 2, ...)
	DefaultIn  bool     // Is this the default input target?
	Prefixes   []string // Command prefixes for routing (e.g., "/planner", "@planner")
}

// LayoutSpec defines a tmux layout with multiple panes
type LayoutSpec struct {
	Name  string     // Layout name (e.g., "single", "team-3pane")
	Panes []PaneSpec // Pane definitions
}
