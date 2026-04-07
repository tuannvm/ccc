package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	TmuxPath   string // exported for testing
	CccPath    string
	ServerName string // Optional: for isolated tmux servers in testing
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
			return r.CreateThreePaneLayout(target, workDir)
		}
		return nil // Window exists with correct layout
	}

	return r.CreateThreePaneLayout(target, workDir)
}

// GetRoleTarget returns the tmux target for a specific role
// For team sessions: planner -> :.1, executor -> :.2, reviewer -> :.3
// Note: tmux uses 1-based pane indexing
func (r *TeamRuntime) GetRoleTarget(sess Session, role PaneRole) (string, error) {
	sessionName := r.getSessionName(sess)
	target := "ccc-team:" + sessionName

	// Map role to pane index (tmux uses 1-based indexing for panes)
	roleToIndex := map[PaneRole]int{
		RolePlanner:  1,
		RoleExecutor: 2,
		RoleReviewer: 3,
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
	if err := r.initCccPath(); err != nil {
		return err
	}

	// Roles to start Claude in
	roles := []PaneRole{RolePlanner, RoleExecutor, RoleReviewer}

	for _, role := range roles {
		paneTarget, err := r.GetRoleTarget(sess, role)
		if err != nil {
			return fmt.Errorf("failed to get target for role %s: %w", role, err)
		}

		// Build the ccc run command with CCC_ROLE environment variable and provider
		// We set CCC_ROLE to indicate which role this pane should use
		// We pass --provider to ensure the correct provider is used for this session
		// Use the resolved cccPath to ensure the binary is found
		// Use bash explicitly to avoid shell compatibility issues
		providerName := sess.GetProviderName()
		runCmd := fmt.Sprintf("bash -c \"export CCC_ROLE=%s; cd %s && exec %s run --provider %s\"", role, shellQuote(workDir), shellQuote(r.CccPath), shellQuote(providerName))

		base := r.tmuxBaseArgs()

		// Clear any existing content in the pane
		sendKeysArgs := append(base, "send-keys", "-t", paneTarget, "C-c")
		exec.Command(r.TmuxPath, sendKeysArgs...).Run()
		time.Sleep(50 * time.Millisecond)

		// Send command to the pane
		sendKeysArgs = append(base, "send-keys", "-t", paneTarget, runCmd, "C-m")
		if err := exec.Command(r.TmuxPath, sendKeysArgs...).Run(); err != nil {
			return fmt.Errorf("failed to start Claude in %s pane: %w", role, err)
		}

		// Wait a moment for the command to start
		time.Sleep(200 * time.Millisecond)
	}

	return nil
}

// initTmuxPath finds the tmux binary
func (r *TeamRuntime) initTmuxPath() error {
	if r.TmuxPath != "" {
		return nil
	}

	// Try PATH first
	if path, err := exec.LookPath("tmux"); err == nil {
		r.TmuxPath = path
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
			r.TmuxPath = p
			return nil
		}
	}

	return fmt.Errorf("tmux not found")
}

// tmuxBaseArgs returns the base tmux arguments, including -L serverName if set.
// This allows TeamRuntime to use isolated tmux servers for testing.
func (r *TeamRuntime) tmuxBaseArgs() []string {
	if r.ServerName == "" {
		return nil
	}
	return []string{"-L", r.ServerName}
}

// tmuxCmd builds a tmux command with optional server name flag.
func (r *TeamRuntime) tmuxCmd(args ...string) *exec.Cmd {
	base := r.tmuxBaseArgs()
	full := append(base, args...)
	return exec.Command(r.TmuxPath, full...)
}

// tmuxCmdContext builds a tmux command with context and optional server name flag.
func (r *TeamRuntime) tmuxCmdContext(ctx context.Context, args ...string) *exec.Cmd {
	base := r.tmuxBaseArgs()
	full := append(base, args...)
	return exec.CommandContext(ctx, r.TmuxPath, full...)
}

// initCccPath finds the ccc binary
func (r *TeamRuntime) initCccPath() error {
	if r.CccPath != "" {
		return nil
	}

	// First, try to get the current executable
	if exe, err := os.Executable(); err == nil {
		r.CccPath = exe
		return nil
	}

	// Try PATH
	if path, err := exec.LookPath("ccc"); err == nil {
		r.CccPath = path
		return nil
	}

	// Fallback to common paths
	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(home, "bin", "ccc"),
		filepath.Join(home, "go", "bin", "ccc"),
		"/usr/local/bin/ccc",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			r.CccPath = p
			return nil
		}
	}

	return fmt.Errorf("ccc binary not found")
}

// getSessionName extracts the session name from the Session interface
func (r *TeamRuntime) getSessionName(sess Session) string {
	// Use GetName() which handles SessionName field for team sessions
	name := sess.GetName()
	// Sanitize for tmux: replace dots with double underscores
	// This matches the behavior of tmuxSafeName() in the main package
	// Double underscores avoid conflicts with natural underscores in names
	return strings.ReplaceAll(name, ".", "__")
}

// windowExists checks if a tmux window exists (with 5s timeout)
func (r *TeamRuntime) windowExists(target string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	base := r.tmuxBaseArgs()
	listWinArgs := append(base, "list-windows", "-t", target, "-F", "#{window_name}")
	cmd := exec.CommandContext(ctx, r.TmuxPath, listWinArgs...)
	return cmd.Run() == nil
}

// hasThreePanes checks if a window has exactly 3 panes (with 5s timeout)
func (r *TeamRuntime) hasThreePanes(target string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	base := r.tmuxBaseArgs()
	listPanesArgs := append(base, "list-panes", "-t", target)
	cmd := exec.CommandContext(ctx, r.TmuxPath, listPanesArgs...)
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

	base := r.tmuxBaseArgs()
	killArgs := append(base, "kill-window", "-t", target)
	return exec.CommandContext(ctx, r.TmuxPath, killArgs...).Run()
}

// CreateThreePaneLayout creates a new 3-pane tmux window
// Layout: Planner (left) | Executor (middle) | Reviewer (right)
// Exported for integration testing.
func (r *TeamRuntime) CreateThreePaneLayout(target string, workDir string) error {
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

	base := r.tmuxBaseArgs()
	newWindowArgs := append(base, "new-window", "-t", sessName+":", "-n", windowName)
	if err := exec.CommandContext(ctx, r.TmuxPath, newWindowArgs...).Run(); err != nil {
		return fmt.Errorf("failed to create window: %w", err)
	}

	// Split to create 3 panes
	// tmux uses 1-based pane indexing:
	// - After new-window: 1 pane (index 1) - left (Planner)
	// - After first split: 2 panes (1, 2) - left (Planner), right (Executor)
	// - After second split: 3 panes (1, 2, 3) - left (Planner), middle (Executor), right (Reviewer)

	// Split horizontally to create pane 2 (right side, will be Executor)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	splitArgs := append(base, "split-window", "-h", "-t", target)
	if err := exec.CommandContext(ctx2, r.TmuxPath, splitArgs...).Run(); err != nil {
		// Clean up the partially created window
		r.killWindow(target)
		return fmt.Errorf("failed to split for pane 2: %w", err)
	}

	// Select pane 2 (right) and split to create pane 3 (rightmost, will be Reviewer)
	ctx3, cancel3 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel3()

	selectArgs := append(base, "select-pane", "-t", target+".2")
	if err := exec.CommandContext(ctx3, r.TmuxPath, selectArgs...).Run(); err != nil {
		r.killWindow(target)
		return fmt.Errorf("failed to select pane 2: %w", err)
	}

	ctx4, cancel4 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel4()

	if err := exec.CommandContext(ctx4, r.TmuxPath, splitArgs...).Run(); err != nil {
		r.killWindow(target)
		return fmt.Errorf("failed to split for pane 3: %w", err)
	}

	// Equalize pane sizes
	ctx5, cancel5 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel5()

	layoutArgs := append(base, "select-layout", "-t", target, "even-horizontal")
	if err := exec.CommandContext(ctx5, r.TmuxPath, layoutArgs...).Run(); err != nil {
		return fmt.Errorf("failed to equalize panes: %w", err)
	}

	// Name the panes according to their roles for better UX and role determination
	// Pane 1 (left) = Planner, Pane 2 (middle) = Executor, Pane 3 (right) = Reviewer
	ctx6, cancel6 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel6()

	nameArgs := append(base, "select-pane", "-t", target+".1", "-T", "Planner")
	if err := exec.CommandContext(ctx6, r.TmuxPath, nameArgs...).Run(); err != nil {
		return fmt.Errorf("failed to name pane 1: %w", err)
	}

	ctx7, cancel7 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel7()

	nameArgs = append(base, "select-pane", "-t", target+".2", "-T", "Executor")
	if err := exec.CommandContext(ctx7, r.TmuxPath, nameArgs...).Run(); err != nil {
		return fmt.Errorf("failed to name pane 2: %w", err)
	}

	ctx8, cancel8 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel8()

	nameArgs = append(base, "select-pane", "-t", target+".3", "-T", "Reviewer")
	if err := exec.CommandContext(ctx8, r.TmuxPath, nameArgs...).Run(); err != nil {
		return fmt.Errorf("failed to name pane 3: %w", err)
	}

	// Verify pane names were set correctly (for debugging)
	ctx9, cancel9 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel9()
	verifyArgs := append(base, "list-panes", "-t", target, "-F", "#{pane_index}: #{pane_name}")
	verifyCmd := exec.CommandContext(ctx9, r.TmuxPath, verifyArgs...)
	if out, err := verifyCmd.Output(); err == nil {
		fmt.Printf("✓ Team session panes named: %s\n", strings.TrimSpace(string(out)))
	}

	return nil
}

// ensureTeamSession ensures the ccc-team tmux session exists
func (r *TeamRuntime) ensureTeamSession(sessionName string) error {
	// Check if session exists
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	base := r.tmuxBaseArgs()
	listArgs := append(base, "list-sessions", "-F", "#{session_name}")
	cmd := exec.CommandContext(ctx, r.TmuxPath, listArgs...)
	out, err := cmd.Output()
	if err != nil {
		// If tmux server isn't running, that's OK - we'll create the session which will start the server
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "connection refused") {
			// Server not running, proceed to create session (which will start it)
		} else {
			return fmt.Errorf("failed to list sessions: %w", err)
		}
	} else {
		// Server is running, check if session already exists
		sessions := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, sess := range sessions {
			if sess == sessionName {
				return nil // Session exists
			}
		}
	}

	// Create the session
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	newSessArgs := append(base, "new-session", "-d", "-s", sessionName)
	if err := exec.CommandContext(ctx2, r.TmuxPath, newSessArgs...).Run(); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Enable mouse support
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()

	setArgs := append(base, "set-option", "-t", sessionName, "mouse", "on")
	exec.CommandContext(ctx3, r.TmuxPath, setArgs...).Run()

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
	base := r.tmuxBaseArgs()
	listPanesArgs := append(base, "list-panes", "-t", target, "-F", "#{pane_id}")
	cmd := exec.Command(r.TmuxPath, listPanesArgs...)
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
	base := r.tmuxBaseArgs()
	displayArgs := append(base, "display-message", "-t", paneTarget, "-p", "#{pane_id}")
	cmd := exec.Command(r.TmuxPath, displayArgs...)
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
