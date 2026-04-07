package routing

import (
	"github.com/tuannvm/ccc/session"
	"os"
	"path/filepath"
	"strings"
)

// HookRouter defines the interface for routing hook events to panes
type HookRouter interface {
	// RouteHook determines which role triggered a hook event
	// Returns: the role that triggered this hook, error
	RouteHook(transcriptPath string, sess session.Session) (session.PaneRole, error)
}

// SinglePaneHookRouter implements HookRouter for single-pane sessions
// All hooks originate from the standard role
type SinglePaneHookRouter struct{}

// RouteHook returns the standard role for single-pane sessions
func (r *SinglePaneHookRouter) RouteHook(transcriptPath string, sess session.Session) (session.PaneRole, error) {
	return session.RoleStandard, nil
}

// TeamHookRouter implements HookRouter for multi-pane team sessions
// Infers the role from transcript path or environment variable
type TeamHookRouter struct{}

// RouteHook infers the role from multiple sources:
// 1. Transcript path (contains role name: "*planner.jsonl", "*executor.jsonl", etc.)
// 2. Environment variable CCC_ROLE
// 3. Defaults to executor if inference fails
func (r *TeamHookRouter) RouteHook(transcriptPath string, sess session.Session) (session.PaneRole, error) {
	// Try to infer from transcript path
	if transcriptPath != "" {
		if role := inferRoleFromPath(transcriptPath); role != "" {
			return role, nil
		}
	}

	// Try to infer from environment variable
	if role := inferRoleFromEnv(); role != "" {
		return role, nil
	}

	// Default to executor
	return session.RoleExecutor, nil
}

// inferRoleFromPath extracts the role from a transcript file path
// Only matches full role names: planner, executor, reviewer
// Returns empty string if no role is found
func inferRoleFromPath(path string) session.PaneRole {
	base := filepath.Base(path)

	// Check for full role names in filename (e.g., "session-planner.jsonl")
	for _, role := range []session.PaneRole{
		session.RolePlanner,
		session.RoleExecutor,
		session.RoleReviewer,
	} {
		if strings.Contains(base, string(role)) {
			return role
		}
	}

	return ""
}

// inferRoleFromEnv reads the CCC_ROLE environment variable
// Returns empty string if not set or invalid
func inferRoleFromEnv() session.PaneRole {
	roleStr := os.Getenv("CCC_ROLE")
	if roleStr == "" {
		return ""
	}

	role := session.PaneRole(strings.ToLower(roleStr))
	switch role {
	case session.RolePlanner, session.RoleExecutor, session.RoleReviewer:
		return role
	default:
		return ""
	}
}

// GetHookRouter returns the appropriate hook router for a session kind
func GetHookRouter(kind session.SessionKind) HookRouter {
	switch kind {
	case session.SessionKindTeam:
		return &TeamHookRouter{}
	default:
		return &SinglePaneHookRouter{}
	}
}
