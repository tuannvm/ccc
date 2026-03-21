package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// shellQuote safely quotes a string for shell command arguments
func shellQuote(s string) string {
	// Replace single quotes with '\'' and wrap in single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// TeamRuntime implements SessionRuntime for 3-pane team sessions
// Creates a tmux window with 3 panes: Planner (left) | Executor (middle) | Reviewer (right)
type TeamRuntime struct {
	tmuxPath string
}

// EnsureLayout creates a 3-pane tmux layout for a team session
func (r *TeamRuntime) EnsureLayout(sess Session, workDir string) error {
	// Find tmux binary
	if err := r.initTmuxPath(); err != nil {
		return err
	}

	sessionName := r.getSessionName(sess)
	target := "ccc-team:" + sessionName

	// Check if window already exists
	if r.windowExists(target) {
		// Verify it has 3 panes, if not recreate
		if !r.hasThreePanes(target) {
			r.killWindow(target)
			return r.createThreePaneLayout(target, workDir)
		}
		return nil // Window exists with correct layout
	}

	return r.createThreePaneLayout(target, workDir)
}

// GetRoleTarget returns the tmux target for a specific role
// For team sessions: planner -> :.0, executor -> :.1, reviewer -> :.2
func (r *TeamRuntime) GetRoleTarget(sess Session, role PaneRole) (string, error) {
	sessionName := r.getSessionName(sess)
	target := "ccc-team:" + sessionName

	// Map role to pane index
	roleToIndex := map[PaneRole]int{
		RolePlanner:  0,
		RoleExecutor: 1,
		RoleReviewer: 2,
	}

	index, ok := roleToIndex[role]
	if !ok {
		return "", fmt.Errorf("unknown role: %s", role)
	}

	return fmt.Sprintf("%s.%d", target, index), nil
}

// GetDefaultTarget returns the executor pane (default input target)
func (r *TeamRuntime) GetDefaultTarget(sess Session) (string, error) {
	return r.GetRoleTarget(sess, RoleExecutor)
}

// StartClaude launches Claude in each pane with appropriate role context
func (r *TeamRuntime) StartClaude(sess Session, workDir string) error {
	if err := r.initTmuxPath(); err != nil {
		return err
	}

	// Roles to start Claude in
	roles := []PaneRole{RolePlanner, RoleExecutor, RoleReviewer}

	for _, role := range roles {
		paneTarget, err := r.GetRoleTarget(sess, role)
		if err != nil {
			return fmt.Errorf("failed to get target for role %s: %w", role, err)
		}

		// Build the ccc run command with CCC_ROLE environment variable
		// We set CCC_ROLE to indicate which role this pane should use
		// Use shell quoting for paths with spaces/special characters
		quotedWorkDir := shellQuote(workDir)
		runCmd := fmt.Sprintf("CCC_ROLE=%s cd %s && ccc run", role, quotedWorkDir)

		// Send command to the pane
		if err := exec.Command(r.tmuxPath, "send-keys", "-t", paneTarget, runCmd, "C-m").Run(); err != nil {
			return fmt.Errorf("failed to start Claude in %s pane: %w", role, err)
		}
	}

	return nil
}

// initTmuxPath finds the tmux binary
func (r *TeamRuntime) initTmuxPath() error {
	if r.tmuxPath != "" {
		return nil
	}

	// Try PATH first
	if path, err := exec.LookPath("tmux"); err == nil {
		r.tmuxPath = path
		return nil
	}

	// Fallback to common paths
	paths := []string{
		"/opt/homebrew/bin/tmux",
		"/usr/local/bin/tmux",
		"/usr/bin/tmux",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			r.tmuxPath = p
			return nil
		}
	}

	return fmt.Errorf("tmux not found")
}

// getSessionName extracts the session name from the Session interface
func (r *TeamRuntime) getSessionName(sess Session) string {
	// Use path basename as session name
	path := sess.GetPath()
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// windowExists checks if a tmux window exists (with 5s timeout)
func (r *TeamRuntime) windowExists(target string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.tmuxPath, "list-windows", "-t", target, "-F", "#{window_name}")
	return cmd.Run() == nil
}

// hasThreePanes checks if a window has exactly 3 panes (with 5s timeout)
func (r *TeamRuntime) hasThreePanes(target string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.tmuxPath, "list-panes", "-t", target)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return len(lines) == 3
}

// killWindow kills a tmux window (with 5s timeout)
func (r *TeamRuntime) killWindow(target string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return exec.CommandContext(ctx, r.tmuxPath, "kill-window", "-t", target).Run()
}

// createThreePaneLayout creates a new 3-pane tmux window
// Layout: Planner (left) | Executor (middle) | Reviewer (right)
func (r *TeamRuntime) createThreePaneLayout(target string, workDir string) error {
	// Parse target (format: "ccc-team:sessionname")
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid target format: %s", target)
	}

	sessName := parts[0]
	windowName := parts[1]

	// Ensure the ccc-team session exists
	if err := r.ensureTeamSession(sessName); err != nil {
		return fmt.Errorf("failed to ensure team session: %w", err)
	}

	// Create new window (with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := exec.CommandContext(ctx, r.tmuxPath, "new-window", "-t", sessName+":", "-n", windowName).Run(); err != nil {
		return fmt.Errorf("failed to create window: %w", err)
	}

	// Split to create 3 panes
	// Pane 0 (left) - Planner - exists by default
	// Split vertically to create Pane 1 (middle) - Executor
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	if err := exec.CommandContext(ctx2, r.tmuxPath, "split-window", "-h", "-t", target).Run(); err != nil {
		// Clean up the partially created window
		r.killWindow(target)
		return fmt.Errorf("failed to split for pane 1: %w", err)
	}

	// Select pane 1 and split to create Pane 2 (right) - Reviewer
	ctx3, cancel3 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel3()

	if err := exec.CommandContext(ctx3, r.tmuxPath, "select-pane", "-t", target+".1").Run(); err != nil {
		r.killWindow(target)
		return fmt.Errorf("failed to select pane 1: %w", err)
	}

	ctx4, cancel4 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel4()

	if err := exec.CommandContext(ctx4, r.tmuxPath, "split-window", "-h", "-t", target).Run(); err != nil {
		r.killWindow(target)
		return fmt.Errorf("failed to split for pane 2: %w", err)
	}

	// Equalize pane sizes
	ctx5, cancel5 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel5()

	if err := exec.CommandContext(ctx5, r.tmuxPath, "select-layout", "-t", target, "even-horizontal").Run(); err != nil {
		return fmt.Errorf("failed to equalize panes: %w", err)
	}

	return nil
}

// ensureTeamSession ensures the ccc-team tmux session exists
func (r *TeamRuntime) ensureTeamSession(sessionName string) error {
	// Check if session exists
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.tmuxPath, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Check if ccc-team session already exists
	sessions := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, sess := range sessions {
		if sess == sessionName {
			return nil // Session exists
		}
	}

	// Create the session
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	if err := exec.CommandContext(ctx2, r.tmuxPath, "new-session", "-d", "-s", sessionName).Run(); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Enable mouse support
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()

	exec.CommandContext(ctx3, r.tmuxPath, "set-option", "-t", sessionName, "mouse", "on").Run()

	return nil
}

// ActiveTeamSessions tracks active team session windows
var ActiveTeamSessions = make(map[string]*TeamWindowState)
var teamSessionsMutex sync.RWMutex

// TeamWindowState tracks the state of a team session
type TeamWindowState struct {
	SessionName string
	WindowName  string
	Panes       [3]*TeamPaneInfo
	CreateTime  int64
}

// TeamPaneInfo stores info about a single pane
type TeamPaneInfo struct {
	PaneID   string // tmux pane ID (%0, %1, etc.)
	Role     PaneRole
	ClaudePID int   // Claude process ID (0 if not running)
}

// GetOrCreateTeamWindow retrieves or creates a team session window
func GetOrCreateTeamWindow(sessionName string) (*TeamWindowState, error) {
	teamSessionsMutex.Lock()
	defer teamSessionsMutex.Unlock()

	if state, exists := ActiveTeamSessions[sessionName]; exists {
		return state, nil
	}

	state := &TeamWindowState{
		SessionName: sessionName,
		WindowName:  sessionName,
		Panes:       [3]*TeamPaneInfo{},
	}

	// Initialize pane info
	state.Panes[0] = &TeamPaneInfo{Role: RolePlanner}
	state.Panes[1] = &TeamPaneInfo{Role: RoleExecutor}
	state.Panes[2] = &TeamPaneInfo{Role: RoleReviewer}

	ActiveTeamSessions[sessionName] = state
	return state, nil
}

// DeleteTeamWindow removes a team session from tracking
func DeleteTeamWindow(sessionName string) {
	teamSessionsMutex.Lock()
	defer teamSessionsMutex.Unlock()

	delete(ActiveTeamSessions, sessionName)
}

// FindPaneByRole finds the pane ID for a given role
func (r *TeamRuntime) FindPaneByRole(sessionName string, role PaneRole) (string, error) {
	teamSessionsMutex.RLock()
	defer teamSessionsMutex.RUnlock()

	state, exists := ActiveTeamSessions[sessionName]
	if !exists {
		return "", fmt.Errorf("team session not found: %s", sessionName)
	}

	roleToIndex := map[PaneRole]int{
		RolePlanner:  0,
		RoleExecutor: 1,
		RoleReviewer: 2,
	}

	index, ok := roleToIndex[role]
	if !ok || index < 0 || index >= 3 {
		return "", fmt.Errorf("invalid role index: %s", role)
	}

	return state.Panes[index].PaneID, nil
}

// ListPanes gets all pane IDs for a window
func (r *TeamRuntime) ListPanes(target string) ([]string, error) {
	cmd := exec.Command(r.tmuxPath, "list-panes", "-t", target, "-F", "#{pane_id}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var panes []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			panes = append(panes, line)
		}
	}

	return panes, nil
}

// CapturePaneID gets the stable pane ID for a pane index
func (r *TeamRuntime) CapturePaneID(target string, index int) (string, error) {
	paneTarget := fmt.Sprintf("%s.%d", target, index)
	cmd := exec.Command(r.tmuxPath, "display-message", "-t", paneTarget, "-p", "#{pane_id}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// RefreshPaneIDs refreshes the pane IDs for a team session
// Uses write lock since this modifies state
func (r *TeamRuntime) RefreshPaneIDs(sessionName string) error {
	teamSessionsMutex.Lock()
	defer teamSessionsMutex.Unlock()

	state, exists := ActiveTeamSessions[sessionName]
	if !exists {
		return fmt.Errorf("team session not found: %s", sessionName)
	}

	target := "ccc-team:" + sessionName

	for i := 0; i < 3; i++ {
		paneID, err := r.CapturePaneID(target, i)
		if err != nil {
			return fmt.Errorf("failed to get pane ID for index %d: %w", i, err)
		}

		// Store the pane ID as-is (tmux format like %0, %1, etc.)
		state.Panes[i].PaneID = paneID
	}

	return nil
}
