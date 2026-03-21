package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/tuannvm/ccc/routing"
	"github.com/tuannvm/ccc/session"
)

// handleTeamSessionMessage routes a Telegram message to the appropriate pane in a team session
// Returns true if the message was handled (team session), false if not (standard session)
func handleTeamSessionMessage(config *Config, text string, topicID int64, chatID int64, threadID int64) bool {
	// Check if this is a team session
	if !config.IsTeamSession(topicID) {
		return false // Not a team session, use standard handling
	}

	// Get the team session info
	sessInfo, exists := config.GetTeamSession(topicID)
	if !exists || sessInfo == nil {
		sendMessage(config, chatID, threadID, "❌ Team session not found. Use /team new to create one.")
		return true // Handled (with error)
	}

	// Get the layout for this session
	layout, ok := session.GetLayout(sessInfo.GetLayoutName())
	if !ok {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Unknown layout: %s", sessInfo.GetLayoutName()))
		return true
	}

	// Get the appropriate router for this session kind
	router := routing.GetRouter(sessInfo.GetType())

	// Route the message to get target role and stripped message
	role, messageText, err := router.RouteMessage(text, layout)
	if err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Routing error: %v", err))
		return true
	}

	// Get session name from the session info
	sessionName := getSessionNameFromInfo(sessInfo)

	// Get the tmux target for this role
	target, err := getTeamRoleTarget(sessionName, role)
	if err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to get pane target: %v", err))
		return true
	}

	// Check if we need to switch sessions
	currentSession := getCurrentSessionName()
	needsSwitch := currentSession != tmuxSafeName(sessionName)

	if needsSwitch {
		// Switch to the session (but don't restart Claude, just select the window)
		if err := switchToTeamWindow(sessionName, role); err != nil {
			sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to switch to session: %v", err))
			return true
		}
	}

	// Send the message to the target pane
	if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessionName), messageText); err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to send message: %v", err))
		return true
	}

	// Log the delivery
	listenLog("[team-session] Topic:%d Role:%s Session:%s: %s", topicID, role, sessionName, messageText)

	return true // Handled as team session
}

// getTeamRoleTarget returns the tmux target for a specific role in a team session
func getTeamRoleTarget(sessionName string, role session.PaneRole) (string, error) {
	target := "ccc-team:" + sessionName

	// Map role to pane index (tmux uses 1-based indexing)
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

// switchToTeamWindow switches to a team session window and selects the appropriate pane
func switchToTeamWindow(sessionName string, role session.PaneRole) error {
	// For now, just select the window
	// In a full implementation, this would also select the specific pane
	target := "ccc-team:" + sessionName

	// Select the window to make it active
	exec.Command(tmuxPath, "select-window", "-t", target).Run()

	return nil
}

// getSessionNameFromInfo extracts the session name from SessionInfo
func getSessionNameFromInfo(info *SessionInfo) string {
	// For team sessions, use the SessionName field if available
	if info.SessionName != "" {
		return info.SessionName
	}
	// Fallback to path basename for backward compatibility
	path := info.Path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// prependRolePrefix adds the role name prefix to outgoing messages from team sessions
func prependRolePrefix(role session.PaneRole, message string) string {
	// Map role to display name
	roleNames := map[session.PaneRole]string{
		session.RolePlanner:  "[Planner]",
		session.RoleExecutor: "[Executor]",
		session.RoleReviewer: "[Reviewer]",
		session.RoleStandard: "", // No prefix for standard sessions
	}

	if prefix, ok := roleNames[role]; ok && prefix != "" {
		return prefix + " " + message
	}
	return message
}

// isTeamSessionCommand checks if a text message is a team-specific command
func isTeamSessionCommand(text string) bool {
	teamCommands := []string{
		"/planner", "/plan", "/p",
		"/executor", "/exec", "/e",
		"/reviewer", "/rev", "/r",
		"@planner", "@executor", "@reviewer",
	}

	textLower := strings.ToLower(strings.TrimSpace(text))
	for _, cmd := range teamCommands {
		if strings.HasPrefix(textLower, cmd+" ") || textLower == cmd {
			return true
		}
	}

	return false
}

// parseTeamCommand extracts the role and message from a team command
// Returns: role, message, isTeamCommand
func parseTeamCommand(text string) (session.PaneRole, string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return session.RoleExecutor, text, false
	}

	// Check for command prefixes
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return session.RoleExecutor, text, false
	}

	prefix := strings.ToLower(fields[0])

	// Map prefixes to roles
	prefixToRole := map[string]session.PaneRole{
		"/planner":   session.RolePlanner,
		"/plan":      session.RolePlanner,
		"/p":         session.RolePlanner,
		"@planner":   session.RolePlanner,
		"/executor":  session.RoleExecutor,
		"/exec":      session.RoleExecutor,
		"/e":         session.RoleExecutor,
		"@executor":  session.RoleExecutor,
		"/reviewer":  session.RoleReviewer,
		"/rev":       session.RoleReviewer,
		"/r":         session.RoleReviewer,
		"@reviewer":  session.RoleReviewer,
	}

	if role, ok := prefixToRole[prefix]; ok {
		// Strip the prefix from the message
		message := strings.Join(fields[1:], " ")
		return role, message, true
	}

	// No team command prefix - goes to executor by default
	return session.RoleExecutor, text, false
}
