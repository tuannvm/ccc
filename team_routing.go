package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/tuannvm/ccc/routing"
	"github.com/tuannvm/ccc/session"
)

// isBuiltinCommand checks if a message is a built-in CCC command that should bypass team routing
// These commands are handled by CCC directly, not sent to Claude in a team session
func isBuiltinCommand(text string) bool {
	// Get the first word (command)
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}

	command := strings.ToLower(fields[0])

	// List of built-in commands that should bypass team routing
	builtinCommands := []string{
		"/stop",
		"/delete",
		"/resume",
		"/providers",
		"/provider",
		"/new",
		"/worktree",
		"/team",
		"/cleanup",
		"/c",
		"/stats",
		"/update",
		"/version",
		"/auth",
		"/restart",
		"/continue",
	}

	for _, cmd := range builtinCommands {
		if command == cmd || strings.HasPrefix(command, cmd+" ") {
			return true
		}
	}

	return false
}

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
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	// Sanitize session name for tmux (dots become double underscores)
	sanitizedName := tmuxSafeName(sessionName)
	target := "ccc-team:" + sanitizedName

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
	if sessionName == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	// Sanitize session name for tmux (dots become double underscores)
	sanitizedName := tmuxSafeName(sessionName)
	target := "ccc-team:" + sanitizedName

	// Select the window to make it active (optional for headless tmux)
	// In headless environments (no attached client), select-window fails but send-keys still works
	if err := exec.Command(tmuxPath, "select-window", "-t", target).Run(); err != nil {
		// Log but don't fail - send-keys will work even if select-window fails
		listenLog("[team routing] select-window failed (may be headless): %v", err)
	}

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
