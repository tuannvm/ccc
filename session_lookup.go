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

// findSession matches by tmux window name first (represents current user context),
// then claude_session_id (fast lookup for persisted sessions), then falls back to cwd
// The window name check is critical for worktree sessions and for handling session switches
func findSession(config *Config, cwd string, claudeSessionID string) (string, int64) {
	// First, try to match by tmux window name (most reliable indicator of current session)
	// This is checked first because it represents the user's actual current context
	// Window name lookup handles: worktree sessions, session switches via `ccc attach`,
	// and cases where ClaudeSessionID is stale or manually set via `/session`
	if windowName := getCurrentTmuxWindowName(); windowName != "" {
		if name, topicID := findSessionByWindowName(config, windowName); name != "" {
			return name, topicID
		}
	}
	// Then, try to match by claude session ID (fast map lookup, reliable once persisted)
	// This is a fallback for when tmux is not available or window name doesn't match
	if name, topicID := findSessionByClaudeID(config, claudeSessionID); name != "" {
		return name, topicID
	}
	// Finally, fall back to cwd matching (least reliable for worktree sessions)
	return findSessionByCwd(config, cwd)
}
