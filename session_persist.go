package main

// persistClaudeSessionID saves the claude session ID to config if changed
func persistClaudeSessionID(config *Config, sessName string, claudeSessionID string) {
	if claudeSessionID == "" || sessName == "" {
		return
	}
	info, exists := config.Sessions[sessName]
	if !exists || info == nil {
		return
	}
	if info.ClaudeSessionID != claudeSessionID {
		info.ClaudeSessionID = claudeSessionID
		saveConfig(config)
		hookLog("persisted claude_session_id=%s for session=%s", claudeSessionID, sessName)
	}
}
