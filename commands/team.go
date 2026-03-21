package commands

import (
	"fmt"
	"os"

	"github.com/tuannvm/ccc/session"
)

// Session is a forward declaration to avoid circular imports
// The actual SessionInfo is defined in the main package
type Session interface {
	GetName() string
	GetPath() string
	GetTopicID() int64
	GetType() session.SessionKind
	GetLayoutName() string
	GetPanes() map[session.PaneRole]*session.PaneInfo
}

// TeamCommands handles the 'ccc team' subcommand family
type TeamCommands struct {
	// In a real implementation, this would have dependencies injected
	// For now, we'll use global functions from the main package
}

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

	// TODO: Implement actual team session creation
	// This requires calling the main package's config and tmux functions
	fmt.Printf("Creating team session '%s' with topic %d\n", name, topicID)
	fmt.Printf("  - Pane 0 (Planner): planning and architecture\n")
	fmt.Printf("  - Pane 1 (Executor): code execution\n")
	fmt.Printf("  - Pane 2 (Reviewer): code review\n")

	return nil
}

// ListTeams lists all active team sessions
func (tc *TeamCommands) ListTeams() error {
	// TODO: Implement actual team session listing
	// This requires loading config and iterating TeamSessions
	fmt.Println("Active team sessions:")
	fmt.Println("  (no team sessions found)")
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

	// TODO: Implement actual attach logic
	fmt.Printf("Attaching to team session '%s', role: %s\n", name, role)
	return nil
}

// StopTeam stops a team session (kills Claude processes, keeps tmux window)
func (tc *TeamCommands) StopTeam(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ccc team stop <name>")
	}

	name := args[0]

	// TODO: Implement actual stop logic
	fmt.Printf("Stopping team session '%s'\n", name)
	return nil
}

// DeleteTeam deletes a team session (kills tmux window, removes from config)
func (tc *TeamCommands) DeleteTeam(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ccc team delete <name>")
	}

	name := args[0]

	// TODO: Implement actual delete logic
	fmt.Printf("Deleting team session '%s'\n", name)
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
func validateTeamSession(sess Session) error {
	if sess.GetType() != session.SessionKindTeam {
		return fmt.Errorf("session type is not 'team'")
	}

	if sess.GetLayoutName() != "team-3pane" {
		return fmt.Errorf("invalid layout for team session: %s", sess.GetLayoutName())
	}

	panes := sess.GetPanes()
	if len(panes) != 3 {
		return fmt.Errorf("team session must have exactly 3 panes, got %d", len(panes))
	}

	requiredRoles := []session.PaneRole{
		session.RolePlanner,
		session.RoleExecutor,
		session.RoleReviewer,
	}

	for _, role := range requiredRoles {
		if _, exists := panes[role]; !exists {
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
