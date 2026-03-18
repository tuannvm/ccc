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
// Prefers exact matches over sanitized matches to avoid collision ambiguity
func findSessionByWindowName(config *Config, windowName string) (string, int64) {
	if windowName == "" {
		return "", 0
	}
	// First pass: look for exact match (highest priority)
	for name, info := range config.Sessions {
		if name == "" || info == nil {
			continue
		}
		if name == windowName {
			return name, info.TopicID
		}
	}
	// Second pass: look for sanitized match (lower priority)
	// Window names match session names, but tmux sanitizes dots to "__"
	// This handles sessions like "my.project" whose tmux window is "my__project"
	var sanitizedMatch string
	var sanitizedTopicID int64
	for name, info := range config.Sessions {
		if name == "" || info == nil {
			continue
		}
		if tmuxSafeName(name) == windowName {
			// Found a match via sanitization
			if sanitizedMatch != "" {
				// Ambiguous! Multiple sessions sanitize to the same window name
				// This is a configuration error - log and skip
				hookLog("WARNING: Ambiguous window name '%s' matches multiple sessions: %s, %s",
					windowName, sanitizedMatch, name)
				return "", 0
			}
			sanitizedMatch = name
			sanitizedTopicID = info.TopicID
		}
	}
	return sanitizedMatch, sanitizedTopicID
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
// If multiple sessions have the same claude_session_id, prefers the one matching current window
func findSessionByClaudeID(config *Config, claudeSessionID string) (string, int64) {
	if claudeSessionID == "" {
		return "", 0
	}
	// First, check if there's a session with this claude_session_id that also matches the current window
	// This handles the case where multiple sessions accidentally have the same ID
	currentWindowName := getCurrentTmuxWindowName()
	if currentWindowName != "" {
		// Try direct match first
		if info, exists := config.Sessions[currentWindowName]; exists && info != nil && info.ClaudeSessionID == claudeSessionID {
			return currentWindowName, info.TopicID
		}
		// Try sanitized match (handles session names with dots like "foo.bar")
		// Check for ambiguous matches (multiple sessions sanitizing to same window name)
		var sanitizedMatch string
		var sanitizedTopicID int64
		for name, info := range config.Sessions {
			if info == nil || info.ClaudeSessionID != claudeSessionID {
				continue
			}
			if tmuxSafeName(name) == currentWindowName {
				if sanitizedMatch != "" {
					// Ambiguous! Multiple sessions with same ID sanitize to the same window name
					hookLog("WARNING: Ambiguous claude_session_id '%s' and window '%s' matches multiple sessions: %s, %s",
						claudeSessionID, currentWindowName, sanitizedMatch, name)
					return "", 0
				}
				sanitizedMatch = name
				sanitizedTopicID = info.TopicID
			}
		}
		if sanitizedMatch != "" {
			return sanitizedMatch, sanitizedTopicID
		}
	}
	// Fall back to first match (should be rare after persistClaudeSessionID deduplication)
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

// findSession matches by claude_session_id first (most reliable once persisted),
// then tmux window name (only as tiebreaker for duplicate IDs), then falls back to cwd
// Note: tmux window name reflects currently VIEWED window, not necessarily the hook's origin
func findSession(config *Config, cwd string, claudeSessionID string) (string, int64) {
	// First, try to match by claude session ID (most reliable indicator)
	// This correctly identifies the session even if user is viewing a different window
	if name, topicID := findSessionByClaudeID(config, claudeSessionID); name != "" {
		return name, topicID
	}
	// Then, try tmux window name (only reaches here if claude_session_id is empty/unknown)
	// This handles the first hook in a new session before claude_session_id is persisted
	if windowName := getCurrentTmuxWindowName(); windowName != "" {
		if name, topicID := findSessionByWindowName(config, windowName); name != "" {
			return name, topicID
		}
	}
	// Finally, fall back to cwd matching (least reliable for worktree sessions)
	return findSessionByCwd(config, cwd)
}
