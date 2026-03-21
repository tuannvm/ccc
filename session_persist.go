package main

// persistClaudeSessionID saves the claude session ID to config if changed
// For single sessions, stores in SessionInfo.ClaudeSessionID
// For team sessions, stores in the matching pane's PaneInfo.ClaudeSessionID
// It also clears the same claude_session_id from any other session/pane to prevent ambiguity
func persistClaudeSessionID(config *Config, sessName string, claudeSessionID string) {
	if claudeSessionID == "" || sessName == "" {
		return
	}

	// First check if this is a team session
	var sessInfo *SessionInfo
	var isTeam bool

	// Look up the session in single sessions first
	for name, info := range config.Sessions {
		if info != nil && name == sessName {
			sessInfo = info
			break
		}
	}

	// Check team sessions
	if sessInfo == nil {
		for _, info := range config.TeamSessions {
			if info != nil && info.SessionName == sessName {
				sessInfo = info
				isTeam = true
				break
			}
		}
	}

	if sessInfo == nil {
		return
	}

	// For team sessions, store the Claude session ID in the appropriate pane
	// We need to find which pane this session ID belongs to by matching cwd or using the pane's pane_id
	if isTeam && sessInfo.Panes != nil {
		hookLog("persistClaudeSessionID: team session=%s, claudeSessionID=%s", sessName, claudeSessionID)
		// Check if any pane already has this Claude session ID
		hasMatch := false
		for role, pane := range sessInfo.Panes {
			if pane != nil {
				hookLog("persistClaudeSessionID: pane role=%s has claudeSessionID=%s", role, pane.ClaudeSessionID)
				if pane.ClaudeSessionID == claudeSessionID {
					hasMatch = true
					break
				}
			}
		}

		// If no existing match, we need to determine which pane this is
		// For now, we'll store it in all panes that don't have a session ID yet
		// (This is a fallback - ideally we'd match by cwd or pane_id)
		if !hasMatch {
			hookLog("persistClaudeSessionID: no existing match, storing in empty panes")
			for role, pane := range sessInfo.Panes {
				if pane != nil && pane.ClaudeSessionID == "" {
					pane.ClaudeSessionID = claudeSessionID
					hookLog("persisted claude_session_id=%s for team session=%s pane=%s", claudeSessionID, sessName, role)
				}
			}
			saveConfig(config)
			return
		}

		// Already had this ID, nothing to do
		hookLog("persistClaudeSessionID: already has this ID")
		return
	}

	// For single sessions, use the original logic
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
