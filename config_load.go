package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadConfig reads and parses the config file, handling migration from old format
func loadConfig() (*Config, error) {
	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		return nil, err
	}

	// First check if this is old format (sessions as map[string]int64)
	var rawConfig map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return nil, err
	}

	// Try to detect old sessions format
	var needsMigration bool
	var oldSessions map[string]int64
	if sessionsRaw, ok := rawConfig["sessions"]; ok {
		// Try to parse as old format (map of topic IDs)
		if json.Unmarshal(sessionsRaw, &oldSessions) == nil && len(oldSessions) > 0 {
			// Check if values are positive numbers (old format)
			for _, v := range oldSessions {
				if v > 0 {
					needsMigration = true
					break
				}
			}
		}
	}

	var config Config
	if needsMigration {
		// Parse everything except sessions first
		type ConfigWithoutSessions struct {
			BotToken     string                       `json:"bot_token"`
			ChatID       int64                        `json:"chat_id"`
			GroupID      int64                        `json:"group_id"`
			ProjectsDir  string                       `json:"projects_dir"`
			Away         bool                         `json:"away"`
			TeamSessions map[int64]*SessionInfo       `json:"team_sessions,omitempty"`
		}
		var partial ConfigWithoutSessions
		json.Unmarshal(data, &partial)

		config.BotToken = partial.BotToken
		config.ChatID = partial.ChatID
		config.GroupID = partial.GroupID
		config.ProjectsDir = partial.ProjectsDir
		config.Away = partial.Away
		// Preserve existing TeamSessions if present in the config
		config.TeamSessions = partial.TeamSessions

		// Migrate sessions
		home, _ := os.UserHomeDir()
		config.Sessions = make(map[string]*SessionInfo)
		for name, topicID := range oldSessions {
			// For old sessions, try to figure out the path
			var sessionPath string
			if strings.HasPrefix(name, "/") {
				// Absolute path
				sessionPath = name
			} else if strings.HasPrefix(name, "~/") {
				// Home-relative path
				sessionPath = filepath.Join(home, name[2:])
			} else if config.ProjectsDir != "" {
				// Use projects_dir if set
				projectsDir := config.ProjectsDir
				if strings.HasPrefix(projectsDir, "~/") {
					projectsDir = filepath.Join(home, projectsDir[2:])
				}
				sessionPath = filepath.Join(projectsDir, name)
			} else {
				sessionPath = filepath.Join(home, name)
			}
			config.Sessions[name] = &SessionInfo{
				TopicID: topicID,
				Path:    sessionPath,
			}
		}
		// IMPORTANT: Initialize TeamSessions only if not already present
		// The migration should preserve existing TeamSessions, not wipe them
		if config.TeamSessions == nil {
			config.TeamSessions = make(map[int64]*SessionInfo)
		}
		// Save migrated config (now with both Sessions and TeamSessions)
		saveConfig(&config)
	} else {
		// Parse with new format
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, err
		}
	}

	if config.Sessions == nil {
		config.Sessions = make(map[string]*SessionInfo)
	}
	// IMPORTANT: Initialize TeamSessions if nil (may not be in old config files)
	if config.TeamSessions == nil {
		config.TeamSessions = make(map[int64]*SessionInfo)
	}

	// Validate the loaded config
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}
