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
