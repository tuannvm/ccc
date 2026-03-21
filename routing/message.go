package routing

import (
	"github.com/tuannvm/ccc/session"
	"strings"
)

// MessageRouter defines the interface for routing incoming messages to panes
type MessageRouter interface {
	// RouteMessage parses an incoming message and determines the target pane
	// Returns: target role, stripped message (without prefix), error
	RouteMessage(text string, layout session.LayoutSpec) (session.PaneRole, string, error)
}

// SinglePaneRouter implements MessageRouter for single-pane sessions
// All messages go to the standard role without modification
type SinglePaneRouter struct{}

// RouteMessage returns the standard role with the original text
func (r *SinglePaneRouter) RouteMessage(text string, layout session.LayoutSpec) (session.PaneRole, string, error) {
	return session.RoleStandard, text, nil
}

// TeamRouter implements MessageRouter for multi-pane team sessions
// Routes messages based on command prefixes (/planner, /executor, /reviewer)
// Default routing goes to the executor pane
type TeamRouter struct{}

// RouteMessage parses the message for routing prefixes
// Returns the target role, message without prefix, and error
func (r *TeamRouter) RouteMessage(text string, layout session.LayoutSpec) (session.PaneRole, string, error) {
	// Trim leading whitespace
	text = strings.TrimSpace(text)
	if text == "" {
		return session.RoleExecutor, "", nil // Empty message goes to executor
	}

	// Check for command prefix (first word)
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return session.RoleExecutor, text, nil // No prefix = executor
	}

	prefix := strings.ToLower(fields[0])

	// Build prefix-to-role mapping from layout spec
	prefixMap := make(map[string]session.PaneRole)
	for _, pane := range layout.Panes {
		for _, p := range pane.Prefixes {
			// Store prefix without leading slash for easier matching
			prefixKey := strings.TrimPrefix(strings.ToLower(p), "/")
			prefixMap[prefixKey] = session.PaneRole(pane.ID)
		}
	}

	// Check if the prefix matches a role
	if role, ok := prefixMap[prefix]; ok {
		// Strip the prefix from the message
		message := strings.Join(fields[1:], " ")
		return role, message, nil
	}

	// No prefix found = default to executor
	return session.RoleExecutor, text, nil
}

// GetRouter returns the appropriate router for a session kind
func GetRouter(kind session.SessionKind) MessageRouter {
	switch kind {
	case session.SessionKindTeam:
		return &TeamRouter{}
	default:
		return &SinglePaneRouter{}
	}
}
