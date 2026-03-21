package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/tuannvm/ccc/session"
)

// TeamCommands handles the 'ccc team' subcommand family
type TeamCommands struct{}

// NewTeamCommands creates a new TeamCommands instance
func NewTeamCommands() *TeamCommands {
	return &TeamCommands{}
}

// Run executes the team subcommand
func (tc *TeamCommands) Run(args []string) error {
	if len(args) == 0 {
		return tc.printUsage()
	}

	subcommand := args[0]
	subargs := args[1:]

	switch subcommand {
	case "new":
		return tc.NewTeam(subargs)
	case "list":
		return tc.ListTeams()
	case "attach":
		return tc.AttachTeam(subargs)
	case "start":
		return tc.StartTeam(subargs)
	case "stop":
		return tc.StopTeam(subargs)
	case "delete":
		return tc.DeleteTeam(subargs)
	default:
		fmt.Printf("Unknown team subcommand: %s\n", subcommand)
		return tc.printUsage()
	}
}

// printUsage prints the usage information
func (tc *TeamCommands) printUsage() error {
	fmt.Println("ccc team - Multi-pane team session management")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ccc team new <name> --topic <topic-id>     Create a new team session")
	fmt.Println("  ccc team list                               List all active team sessions")
	fmt.Println("  ccc team attach <name> [--role <role>]      Attach to a team session")
	fmt.Println("  ccc team start <name>                       Start Claude in a team session")
	fmt.Println("  ccc team stop <name>                        Stop a team session")
	fmt.Println("  ccc team delete <name>                      Delete a team session")
	fmt.Println()
	fmt.Println("Roles:")
	fmt.Println("  planner   - Planning and architecture")
	fmt.Println("  executor  - Code execution and commands")
	fmt.Println("  reviewer  - Code review and feedback")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ccc team new feature-api --topic 12345")
	fmt.Println("  ccc team attach feature-api --role planner")
	fmt.Println("  ccc team list")
	return nil
}

// NewTeam creates a new team session with 3 panes
func (tc *TeamCommands) NewTeam(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ccc team new <name> --topic <topic-id>")
	}

	name := args[0]
	var topicID int64

	// Parse --topic flag
	for i := 1; i < len(args); i++ {
		if args[i] == "--topic" && i+1 < len(args) {
			_, err := fmt.Sscanf(args[i+1], "%d", &topicID)
			if err != nil {
				return fmt.Errorf("invalid topic ID: %w", err)
			}
			break
		}
	}

	if topicID == 0 {
		return fmt.Errorf("topic ID is required (use --topic <id>)")
	}

	// Load config
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if team session already exists for this topic
	if config.IsTeamSession(topicID) {
		return fmt.Errorf("team session already exists for topic %d", topicID)
	}

	// Get working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Get provider
	providerName := config.ActiveProvider
	if providerName == "" {
		providerName = "anthropic"
	}

	// Create SessionInfo for team session
	sessInfo := &SessionInfo{
		TopicID:      topicID,
		Path:         cwd,
		ProviderName: providerName,
		Type:         "team",              // Using string directly, will convert to SessionKind
		LayoutName:   "team-3pane",
		Panes:        make(map[session.PaneRole]*PaneInfo),
	}

	// Initialize panes (empty for now, will be populated when runtime creates layout)
	sessInfo.Panes[session.RolePlanner] = &PaneInfo{Role: session.RolePlanner}
	sessInfo.Panes[session.RoleExecutor] = &PaneInfo{Role: session.RoleExecutor}
	sessInfo.Panes[session.RoleReviewer] = &PaneInfo{Role: session.RoleReviewer}

	// Save to config
	config.SetTeamSession(topicID, sessInfo)
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Ensure hooks are installed
	if err := ensureHooksForSession(config, name, sessInfo); err != nil {
		listenLog("[team new] Failed to install hooks: %v", err)
	}

	// Create the 3-pane tmux layout using TeamRuntime
	runtime := session.GetRuntime(session.SessionKindTeam)
	if runtime == nil {
		return fmt.Errorf("team runtime not registered")
	}

	// Ensure layout creates the 3-pane window
	if err := runtime.EnsureLayout(sessInfo, cwd); err != nil {
		// Rollback: remove from config
		config.DeleteTeamSession(topicID)
		saveConfig(config)
		return fmt.Errorf("failed to create team session layout: %w", err)
	}

	// Start Claude in each pane
	if err := runtime.StartClaude(sessInfo, cwd); err != nil {
		listenLog("[team new] Failed to start Claude: %v", err)
		// Non-fatal - Claude can be started manually
	}

	fmt.Printf("✅ Team session '%s' created!\n", name)
	fmt.Printf("  Topic ID: %d\n", topicID)
	fmt.Printf("  Path: %s\n", cwd)
	fmt.Printf("  Provider: %s\n", providerName)
	fmt.Printf("\n📱 Send messages in Telegram:\n")
	fmt.Printf("  /planner <msg>   - Send to planner\n")
	fmt.Printf("  /executor <msg>  - Send to executor\n")
	fmt.Printf("  /reviewer <msg>  - Send to reviewer\n")
	fmt.Printf("  <msg> (no cmd)   - Send to executor (default)\n")

	return nil
}

// ListTeams lists all active team sessions
func (tc *TeamCommands) ListTeams() error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(config.TeamSessions) == 0 {
		fmt.Println("Active team sessions:")
		fmt.Println("  (no team sessions found)")
		return nil
	}

	fmt.Println("Active team sessions:")
	for topicID, sessInfo := range config.TeamSessions {
		sessName := getSessionNameFromInfo(sessInfo)
		fmt.Printf("  • %s (topic: %d)\n", sessName, topicID)
		fmt.Printf("      Path: %s\n", sessInfo.Path)
		fmt.Printf("      Provider: %s\n", sessInfo.ProviderName)

		// Show pane status if we have pane IDs
		if len(sessInfo.Panes) > 0 {
			for role, paneInfo := range sessInfo.Panes {
				status := "stopped"
				if paneInfo.ClaudeSessionID != "" {
					status = "running"
				}
				fmt.Printf("      [%s] %s: %s\n", status, role, paneInfo.PaneID)
			}
		}
	}
	return nil
}

// StartTeam starts Claude in all panes of a team session
func (tc *TeamCommands) StartTeam(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ccc team start <name>")
	}

	name := args[0]

	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find team session by name
	var sessInfo *SessionInfo
	for _, sess := range config.TeamSessions {
		sessName := getSessionNameFromInfo(sess)
		if sessName == name {
			sessInfo = sess
			break
		}
	}

	if sessInfo == nil {
		return fmt.Errorf("team session not found: %s", name)
	}

	// Get the team runtime
	runtime := session.GetRuntime(session.SessionKindTeam)
	if runtime == nil {
		return fmt.Errorf("team runtime not registered")
	}

	// Start Claude in each pane
	if err := runtime.StartClaude(sessInfo, sessInfo.Path); err != nil {
		return fmt.Errorf("failed to start Claude: %w", err)
	}

	fmt.Printf("Starting Claude in all panes of team session '%s'\n", name)
	return nil
}

// AttachTeam attaches to a team session, optionally to a specific pane
func (tc *TeamCommands) AttachTeam(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ccc team attach <name> [--role <role>]")
	}

	name := args[0]
	role := session.RoleExecutor // Default to executor

	// Parse --role flag
	for i := 1; i < len(args); i++ {
		if args[i] == "--role" && i+1 < len(args) {
			roleStr := args[i+1]
			role = session.PaneRole(roleStr)
			break
		}
	}

	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find team session by name (path basename match)
	var sessInfo *SessionInfo
	for _, sess := range config.TeamSessions {
		sessName := getSessionNameFromInfo(sess)
		if sessName == name {
			sessInfo = sess
			break
		}
	}

	if sessInfo == nil {
		return fmt.Errorf("team session not found: %s", name)
	}

	// Get the team runtime
	runtime := session.GetRuntime(session.SessionKindTeam)
	if runtime == nil {
		return fmt.Errorf("team runtime not registered")
	}

	// Get target for the requested role
	target, err := runtime.GetRoleTarget(sessInfo, role)
	if err != nil {
		return fmt.Errorf("failed to get target for role %s: %w", role, err)
	}

	// Attach to tmux session and select the pane
	if err := attachToTmuxSession("ccc"); err != nil {
		return fmt.Errorf("failed to attach to tmux: %w", err)
	}

	// Select the specific pane
	exec.Command("tmux", "select-pane", "-t", target).Run()

	fmt.Printf("Attached to team session '%s', role: %s\n", name, role)
	return nil
}

// StopTeam stops a team session (kills Claude processes, keeps tmux window)
func (tc *TeamCommands) StopTeam(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ccc team stop <name>")
	}

	name := args[0]

	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find team session by name
	var sessInfo *SessionInfo
	var topicID int64
	for tid, sess := range config.TeamSessions {
		sessName := getSessionNameFromInfo(sess)
		if sessName == name {
			sessInfo = sess
			topicID = tid
			break
		}
	}

	if sessInfo == nil {
		return fmt.Errorf("team session not found: %s", name)
	}

	// Get the team runtime
	runtime := session.GetRuntime(session.SessionKindTeam)
	if runtime == nil {
		return fmt.Errorf("team runtime not registered")
	}

	// Kill Claude in each pane by sending Ctrl-C
	roles := []session.PaneRole{session.RolePlanner, session.RoleExecutor, session.RoleReviewer}
	for _, role := range roles {
		target, err := runtime.GetRoleTarget(sessInfo, role)
		if err != nil {
			listenLog("[team stop] Failed to get target for %s: %v", role, err)
			continue
		}

		// Send Ctrl-C to stop Claude
		exec.Command("tmux", "send-keys", "-t", target, "C-c").Run()

		// Clear the session ID
		if sessInfo.Panes != nil && sessInfo.Panes[role] != nil {
			sessInfo.Panes[role].ClaudeSessionID = ""
		}
	}

	// Save updated config
	config.SetTeamSession(topicID, sessInfo)
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Stopped Claude in all panes of team session '%s'\n", name)
	fmt.Printf("Tip: Use 'ccc team start %s' to restart Claude\n", name)
	return nil
}

// DeleteTeam deletes a team session (kills tmux window, removes from config)
func (tc *TeamCommands) DeleteTeam(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ccc team delete <name>")
	}

	name := args[0]

	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find team session by name
	var sessInfo *SessionInfo
	var topicID int64
	for tid, sess := range config.TeamSessions {
		sessName := getSessionNameFromInfo(sess)
		if sessName == name {
			sessInfo = sess
			topicID = tid
			break
		}
	}

	if sessInfo == nil {
		return fmt.Errorf("team session not found: %s", name)
	}

	// Kill the tmux window
	sessName := getSessionNameFromInfo(sessInfo)
	target := "ccc:" + sessName
	if err := exec.Command("tmux", "kill-window", "-t", target).Run(); err != nil {
		// Window might not exist, but that's okay - continue with cleanup
		listenLog("[team delete] Failed to kill window (may not exist): %v", err)
	}

	// Remove from config
	config.DeleteTeamSession(topicID)
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Deleted team session '%s'\n", name)
	return nil
}

// TeamSessionInfo holds information about a team session for display
type TeamSessionInfo struct {
	Name      string
	TopicID   int64
	Path      string
	Panes     []TeamPaneStatus
	Active    bool
	CreatedAt string
}

// TeamPaneStatus holds status information for a single pane
type TeamPaneStatus struct {
	Role       session.PaneRole
	PaneID     string
	ClaudePID  int
	ClaudeRunning bool
}

// formatTeamSessions formats team session info for display
func formatTeamSessions(sessions []TeamSessionInfo) string {
	if len(sessions) == 0 {
		return "  (no team sessions found)"
	}

	var result string
	for _, sess := range sessions {
		status := " "
		if sess.Active {
			status = "*"
		}
		result += fmt.Sprintf("%s %s (topic: %d)\n", status, sess.Name, sess.TopicID)
		for _, pane := range sess.Panes {
			claudeStatus := " "
			if pane.ClaudeRunning {
				claudeStatus = "C"
			}
			result += fmt.Sprintf("    [%s] Pane %s: %s (Claude: %s)\n",
				claudeStatus, pane.PaneID, pane.Role, claudeStatus)
		}
	}
	return result
}

// validateTeamSession validates that a team session has correct structure
func validateTeamSession(sess *SessionInfo) error {
	if sess.Type != session.SessionKindTeam {
		return fmt.Errorf("session type is not 'team'")
	}

	if sess.LayoutName != "team-3pane" {
		return fmt.Errorf("invalid layout for team session: %s", sess.LayoutName)
	}

	if len(sess.Panes) != 3 {
		return fmt.Errorf("team session must have exactly 3 panes, got %d", len(sess.Panes))
	}

	requiredRoles := []session.PaneRole{
		session.RolePlanner,
		session.RoleExecutor,
		session.RoleReviewer,
	}

	for _, role := range requiredRoles {
		if _, exists := sess.Panes[role]; !exists {
			return fmt.Errorf("team session missing required role: %s", role)
		}
	}

	return nil
}

// Exit codes for team commands
const (
	ExitSuccess     = 0
	ExitUsageError  = 1
	ExitConfigError = 2
	ExitTmuxError   = 3
)

// handleCommandError handles errors from team commands with appropriate exit codes
func handleCommandError(err error, defaultExitCode int) {
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(defaultExitCode)
}
