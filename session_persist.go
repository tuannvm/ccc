package main

// persistClaudeSessionID saves the claude session ID to config if changed
// It also clears the same claude_session_id from any other session to prevent ambiguity
func persistClaudeSessionID(config *Config, sessName string, claudeSessionID string) {
	if claudeSessionID == "" || sessName == "" {
		return
	}
	info, exists := config.Sessions[sessName]
	if !exists || info == nil {
		return
	}

	// Always check for and clear duplicate claude_session_id values, even if the current session
	// already has this ID. This handles the case where duplicates are created via /session command.
	duplicateCleared := false
	for otherName, otherInfo := range config.Sessions {
		if otherName == sessName {
			continue
		}
		if otherInfo != nil && otherInfo.ClaudeSessionID == claudeSessionID {
			otherInfo.ClaudeSessionID = ""
			hookLog("cleared duplicate claude_session_id=%s from session=%s", claudeSessionID, otherName)
			duplicateCleared = true
		}
	}

	// Only save if the ID actually changed OR we cleared a duplicate
	if info.ClaudeSessionID != claudeSessionID {
		info.ClaudeSessionID = claudeSessionID
		saveConfig(config)
		hookLog("persisted claude_session_id=%s for session=%s", claudeSessionID, sessName)
	} else if duplicateCleared {
		saveConfig(config)
		hookLog("persisted claude_session_id=%s for session=%s (duplicate cleared)", claudeSessionID, sessName)
	}
}
