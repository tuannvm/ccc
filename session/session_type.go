package session

import (
	"path/filepath"
	"strings"
)

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

// InferRoleFromTranscriptPath extracts the role from a transcript file path.
// Returns empty string if no role is found.
//
// Handles multiple transcript naming patterns:
//   - session-planner.jsonl, session_planner.jsonl
//   - planner.jsonl, planner-session.jsonl
//   - session.planner.jsonl (with dot separator)
func InferRoleFromTranscriptPath(transcriptPath string) PaneRole {
	if transcriptPath == "" {
		return ""
	}
	base := filepath.Base(transcriptPath)
	// Remove extensions - handle multiple extensions safely
	for {
		newBase := strings.TrimSuffix(base, ".jsonl")
		if newBase == base {
			newBase = strings.TrimSuffix(base, ".json")
		}
		if newBase == base {
			break // No more extensions to remove
		}
		base = newBase
	}

	// Convert to lowercase for case-insensitive matching
	baseLower := strings.ToLower(base)

	// Check for role keywords anywhere in the filename
	// Order matters: check for longer substrings first to avoid false matches
	if strings.Contains(baseLower, "planner") {
		return RolePlanner
	}
	if strings.Contains(baseLower, "executor") {
		return RoleExecutor
	}
	if strings.Contains(baseLower, "reviewer") {
		return RoleReviewer
	}

	// No role found in path
	return ""
}
