package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadConfig reads and parses config files, handling migration from old format
func Load() (*Config, error) {
	var config Config

	legacyLoaded, err := loadLegacyConfig(&config)
	if err != nil {
		return nil, err
	}

	loadedSplit, err := loadSplitConfigs(&config)
	if err != nil {
		return nil, err
	}

	if !legacyLoaded && !loadedSplit {
		return nil, os.ErrNotExist
	}

	if config.Sessions == nil {
		config.Sessions = make(map[string]*SessionInfo)
	}
	if config.TeamSessions == nil {
		config.TeamSessions = make(map[int64]*SessionInfo)
	}

	if err := Validate(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

func loadLegacyConfig(config *Config) (bool, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	migrated, err := unmarshalConfigWithMigration(data, config)
	if err != nil {
		return false, err
	}
	if migrated {
		if err := Save(config); err != nil {
			return false, err
		}
	}

	return true, nil
}

func loadSplitConfigs(config *Config) (bool, error) {
	loadedAny := false

	coreData, err := os.ReadFile(GetCoreConfigPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	} else {
		var core coreConfig
		if err := json.Unmarshal(coreData, &core); err != nil {
			return false, err
		}
		applyCoreConfig(config, core)
		loadedAny = true
	}

	sessionsData, err := os.ReadFile(GetSessionsConfigPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	} else {
		var sessions sessionsConfig
		if err := json.Unmarshal(sessionsData, &sessions); err != nil {
			return false, err
		}
		applySessionsConfig(config, sessions)
		loadedAny = true
	}

	providersData, err := os.ReadFile(GetProvidersConfigPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	} else {
		var providers providersConfig
		if err := json.Unmarshal(providersData, &providers); err != nil {
			return false, err
		}
		applyProvidersConfig(config, providers)
		loadedAny = true
	}

	return loadedAny, nil
}

func unmarshalConfigWithMigration(data []byte, config *Config) (bool, error) {
	var rawConfig map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return false, err
	}

	var needsMigration bool
	var oldSessions map[string]int64
	if sessionsRaw, ok := rawConfig["sessions"]; ok {
		if json.Unmarshal(sessionsRaw, &oldSessions) == nil && len(oldSessions) > 0 {
			for _, v := range oldSessions {
				if v > 0 {
					needsMigration = true
					break
				}
			}
		}
	}

	if !needsMigration {
		if err := json.Unmarshal(data, config); err != nil {
			return false, err
		}
		return false, nil
	}

	type ConfigWithoutSessions struct {
		BotToken     string                 `json:"bot_token"`
		ChatID       int64                  `json:"chat_id"`
		GroupID      int64                  `json:"group_id"`
		ProjectsDir  string                 `json:"projects_dir"`
		Away         bool                   `json:"away"`
		TeamSessions map[int64]*SessionInfo `json:"team_sessions,omitempty"`
	}
	var partial ConfigWithoutSessions
	json.Unmarshal(data, &partial)

	config.BotToken = partial.BotToken
	config.ChatID = partial.ChatID
	config.GroupID = partial.GroupID
	config.ProjectsDir = partial.ProjectsDir
	config.Away = partial.Away
	config.TeamSessions = partial.TeamSessions

	home, _ := os.UserHomeDir()
	config.Sessions = make(map[string]*SessionInfo)
	for name, topicID := range oldSessions {
		var sessionPath string
		if strings.HasPrefix(name, "/") {
			sessionPath = name
		} else if strings.HasPrefix(name, "~/") {
			sessionPath = filepath.Join(home, name[2:])
		} else if config.ProjectsDir != "" {
			projectsDir := config.ProjectsDir
			if strings.HasPrefix(projectsDir, "~/") {
				projectsDir = filepath.Join(home, projectsDir[2:])
			}
			sessionPath = filepath.Join(projectsDir, name)
		} else {
			sessionPath = filepath.Join(home, name)
		}
		config.Sessions[name] = &SessionInfo{TopicID: topicID, Path: sessionPath}
	}
	if config.TeamSessions == nil {
		config.TeamSessions = make(map[int64]*SessionInfo)
	}

	return true, nil
}

func applyCoreConfig(config *Config, core coreConfig) {
	config.BotToken = core.BotToken
	config.ChatID = core.ChatID
	config.GroupID = core.GroupID
	config.MultiUserMode = core.MultiUserMode
	config.CustomEmojiIDs = core.CustomEmojiIDs
	config.EnableStreaming = core.EnableStreaming
	config.ProjectsDir = core.ProjectsDir
	config.TranscriptionLang = core.TranscriptionLang
	config.RelayURL = core.RelayURL
	config.Away = core.Away
	config.OAuthToken = core.OAuthToken
	config.OTPSecret = core.OTPSecret
}

func applySessionsConfig(config *Config, sessions sessionsConfig) {
	config.Sessions = sessions.Sessions
	config.TeamSessions = sessions.TeamSessions
}

func applyProvidersConfig(config *Config, providers providersConfig) {
	config.ActiveProvider = providers.ActiveProvider
	config.Providers = providers.Providers
	config.Provider = providers.Provider
}
