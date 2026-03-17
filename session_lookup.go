package main

import "strings"

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

// findSession matches by claude_session_id first, then falls back to cwd
func findSession(config *Config, cwd string, claudeSessionID string) (string, int64) {
	if name, topicID := findSessionByClaudeID(config, claudeSessionID); name != "" {
		return name, topicID
	}
	return findSessionByCwd(config, cwd)
}

// findPaneByClaudeID finds the session and pane index for a given Claude session ID
// Checks panes map first, then falls back to legacy SessionInfo.ClaudeSessionID
// For new panes (first hook), finds the uninitialized pane in the session
func findPaneByClaudeID(config *Config, claudeSessionID string) (string, string, int64) {
	if claudeSessionID == "" {
		return "", "", 0
	}
	for name, info := range config.Sessions {
		if name == "" || info == nil {
			continue
		}
		// Check panes first (multi-pane sessions)
		if info.Panes != nil {
			// First try to find exact match
			for paneIndex, paneInfo := range info.Panes {
				if paneInfo != nil && paneInfo.ClaudeSessionID == claudeSessionID {
					return name, paneIndex, info.TopicID
				}
			}
			// Not found - this might be the first hook from a new pane
			// Find the uninitialized pane (no ClaudeSessionID set) in this session
			// Prefer ActivePane which was set by createPane for the most recently created pane
			// This handles the race condition where a hook arrives before we persist the session ID
			if info.ActivePane != "" {
				if paneInfo, exists := info.Panes[info.ActivePane]; exists && paneInfo != nil && paneInfo.ClaudeSessionID == "" {
					// Active pane is uninitialized - use it
					return name, info.ActivePane, info.TopicID
				}
			}
			// Fallback: find any uninitialized pane (for older sessions without ActivePane set)
			for paneIndex, paneInfo := range info.Panes {
				if paneInfo != nil && paneInfo.ClaudeSessionID == "" {
					return name, paneIndex, info.TopicID
				}
			}
		}
		// Fallback to legacy single-pane check
		if info.ClaudeSessionID == claudeSessionID {
			return name, "", info.TopicID
		}
	}
	return "", "", 0
}

// findSessionWithPane matches by Claude session ID first, then falls back to cwd
// Returns (sessionName, paneIndex, topicID) — paneIndex is empty for session-level routing
func findSessionWithPane(config *Config, cwd string, claudeSessionID string) (string, string, int64) {
	if sessName, paneIndex, topicID := findPaneByClaudeID(config, claudeSessionID); sessName != "" {
		return sessName, paneIndex, topicID
	}
	sessName, topicID := findSessionByCwd(config, cwd)
	return sessName, "", topicID
}

// resolvePaneRef resolves a pane reference (index "0"/"1" or name "reviewer") to a pane index
// Returns the pane index or empty string if not found
func resolvePaneRef(config *Config, sessionName string, ref string) string {
	info := config.Sessions[sessionName]
	if info == nil || info.Panes == nil {
		return ""
	}

	// Try exact match on pane index
	if _, exists := info.Panes[ref]; exists {
		return ref
	}

	// Try match on friendly name (case-insensitive)
	refLower := strings.ToLower(ref)
	for paneIndex, paneInfo := range info.Panes {
		if paneInfo != nil && strings.ToLower(paneInfo.Name) == refLower {
			return paneIndex
		}
	}
	return ""
}
