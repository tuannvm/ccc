package main

import (
	"os/exec"
	"strings"
)

// getCurrentTmuxWindowName returns the current tmux window name, or empty string if not in tmux
func getCurrentTmuxWindowName() string {
	if tmuxPath == "" {
		return ""
	}
	cmd := exec.Command(tmuxPath, "display-message", "-p", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// findSessionByWindowName finds a session by its tmux window name
// Handles tmux name sanitization where dots are replaced with "__"
func findSessionByWindowName(config *Config, windowName string) (string, int64) {
	if windowName == "" {
		return "", 0
	}
	// Window names match session names, but tmux sanitizes dots to "__"
	// So we need to compare both the raw name and sanitized name
	for name, info := range config.Sessions {
		if name == "" || info == nil {
			continue
		}
		// Direct match
		if name == windowName {
			return name, info.TopicID
		}
		// Match with tmux sanitization (dots -> "__")
		if tmuxSafeName(name) == windowName {
			return name, info.TopicID
		}
	}
	return "", 0
}

// getSessionByTopic finds a session name by its Telegram topic ID
func getSessionByTopic(config *Config, topicID int64) string {
	for name, info := range config.Sessions {
		if info != nil && info.TopicID == topicID {
			return name
		}
	}
	return ""
}

// findSessionByClaudeID matches a claude session ID to a configured session
func findSessionByClaudeID(config *Config, claudeSessionID string) (string, int64) {
	if claudeSessionID == "" {
		return "", 0
	}
	for name, info := range config.Sessions {
		if name == "" || info == nil {
			continue
		}
		if info.ClaudeSessionID == claudeSessionID {
			return name, info.TopicID
		}
	}
	return "", 0
}

// findSessionByCwd matches a hook's cwd to a configured session (fallback)
func findSessionByCwd(config *Config, cwd string) (string, int64) {
	for name, info := range config.Sessions {
		if name == "" || info == nil {
			continue
		}
		if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") || strings.HasSuffix(cwd, "/"+name) {
			return name, info.TopicID
		}
	}
	return "", 0
}

// findSession matches by tmux window name first, then claude_session_id, then falls back to cwd
// The window name check is critical for worktree sessions since they run from the base directory
// but have their own tmux windows named after the worktree session name
func findSession(config *Config, cwd string, claudeSessionID string) (string, int64) {
	// First, try to match by tmux window name (most reliable for worktree sessions)
	if windowName := getCurrentTmuxWindowName(); windowName != "" {
		if name, topicID := findSessionByWindowName(config, windowName); name != "" {
			return name, topicID
		}
	}
	// Then, try to match by claude session ID (reliable once persisted)
	if name, topicID := findSessionByClaudeID(config, claudeSessionID); name != "" {
		return name, topicID
	}
	// Finally, fall back to cwd matching (least reliable for worktree sessions)
	return findSessionByCwd(config, cwd)
}
