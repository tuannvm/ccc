package tmux

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/tuannvm/ccc/pkg/session"
)

// GetTeamRoleTarget returns the tmux target for a specific role in a team session.
func GetTeamRoleTarget(sessionName string, role session.PaneRole) (string, error) {
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	sanitizedName := SafeName(sessionName)
	target := "ccc-team:" + sanitizedName

	roleToIndex := map[session.PaneRole]int{
		session.RolePlanner:  1,
		session.RoleExecutor: 2,
		session.RoleReviewer: 3,
	}

	index, ok := roleToIndex[role]
	if !ok {
		return "", fmt.Errorf("unknown role: %s (valid: planner, executor, reviewer)", role)
	}

	return fmt.Sprintf("%s.%d", target, index), nil
}

// SwitchToTeamWindow switches to a team session window and selects the pane for the given role.
func SwitchToTeamWindow(sessionName string, role session.PaneRole) error {
	if sessionName == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	sanitizedName := SafeName(sessionName)
	windowTarget := "ccc-team:" + sanitizedName

	// First switch to the window
	if err := exec.Command(TmuxPath, "select-window", "-t", windowTarget).Run(); err != nil {
		return fmt.Errorf("failed to select window: %w", err)
	}

	// Then select the pane for the given role
	paneTarget, err := GetTeamRoleTarget(sessionName, role)
	if err != nil {
		return nil // window switch succeeded, role selection is best-effort
	}
	exec.Command(TmuxPath, "select-pane", "-t", paneTarget).Run()

	return nil
}

// GetSessionNameFromPath extracts a session name from a path (basename fallback).
func GetSessionNameFromPath(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
