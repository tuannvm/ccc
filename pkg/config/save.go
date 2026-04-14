package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Save atomically writes the legacy aggregate config and split config files.
// Legacy config is written first so callers always have a consistent fallback.
func Save(config *Config) error {
	if err := writeConfigFile(GetConfigPath(), config); err != nil {
		return err
	}

	core := config.CoreConfig()
	sessions := config.SessionsConfig()
	providers := config.ProvidersConfig()

	if err := writeConfigFile(GetCoreConfigPath(), core); err != nil {
		cleanupSplitConfigFiles()
		return err
	}
	if err := writeConfigFile(GetSessionsConfigPath(), sessions); err != nil {
		cleanupSplitConfigFiles()
		return err
	}
	if err := writeConfigFile(GetProvidersConfigPath(), providers); err != nil {
		cleanupSplitConfigFiles()
		return err
	}
	return nil
}

func cleanupSplitConfigFiles() {
	_ = os.Remove(GetCoreConfigPath())
	_ = os.Remove(GetSessionsConfigPath())
	_ = os.Remove(GetProvidersConfigPath())
}

func writeConfigFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config %s: %w", path, err)
	}

	configDir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(configDir, "config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if err := tmpFile.Chmod(0600); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set temp file permissions: %w", err)
	}

	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename config: %w", err)
	}

	dirFile, err := os.Open(configDir)
	if err != nil {
		return fmt.Errorf("failed to open config directory: %w", err)
	}
	if err := dirFile.Sync(); err != nil {
		dirFile.Close()
		return fmt.Errorf("failed to sync config directory: %w", err)
	}
	dirFile.Close()

	success = true
	return nil
}
