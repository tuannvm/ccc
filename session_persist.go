package main

// persistClaudeSessionID saves the claude session ID to config if changed
func persistClaudeSessionID(config *Config, sessName string, claudeSessionID string) {
	persistClaudeSessionIDForPane(config, sessName, "", claudeSessionID)
}

// persistClaudeSessionIDForPane saves the claude session ID for a specific pane or the legacy session field
// If paneIndex is non-empty and panes map exists, saves to Panes[paneIndex]
// Otherwise falls back to legacy SessionInfo.ClaudeSessionID
func persistClaudeSessionIDForPane(config *Config, sessName string, paneIndex string, claudeSessionID string) {
	if claudeSessionID == "" || sessName == "" {
		return
	}
	info, exists := config.Sessions[sessName]
	if !exists || info == nil {
		return
	}

	if paneIndex != "" && info.Panes != nil {
		if paneInfo := info.Panes[paneIndex]; paneInfo != nil {
			if paneInfo.ClaudeSessionID != claudeSessionID {
				paneInfo.ClaudeSessionID = claudeSessionID
				saveConfig(config)
				hookLog("persisted claude_session_id=%s for session=%s pane=%s", claudeSessionID, sessName, paneIndex)
			}
		}
		return
	}

	if info.ClaudeSessionID != claudeSessionID {
		info.ClaudeSessionID = claudeSessionID
		saveConfig(config)
		hookLog("persisted claude_session_id=%s for session=%s", claudeSessionID, sessName)
	}
}
