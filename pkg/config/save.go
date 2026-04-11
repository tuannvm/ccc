package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Save atomically writes the config to disk using write-then-rename pattern
// Multiple processes may write config simultaneously; atomic rename prevents corruption
func Save(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := GetConfigPath()
	configDir := filepath.Dir(configPath)

	// Atomic write: use unique temp file + rename to prevent concurrent write corruption
	// Multiple ccc processes (listener, hooks, CLI) may write config simultaneously
	// Using unique temp filename prevents race conditions where multiple processes
	// use the same .tmp file and one process's rename causes another to fail
	tmpFile, err := os.CreateTemp(configDir, "config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Set permissions to 0600 (owner read/write only) before writing
	if err := tmpFile.Chmod(0600); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set temp file permissions: %w", err)
	}

	// Track success for cleanup
	success := false
	defer func() {
		if !success {
			// Clean up temp file on any error
			os.Remove(tmpPath)
		}
	}()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Sync to disk - critical for durability
	// Ensures data is written to stable storage before rename
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename - this is the critical operation that ensures atomicity
	// On Linux/macOS, rename is atomic within the same filesystem
	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("failed to rename config: %w", err)
	}

	// Sync parent directory to persist rename across crashes/power loss
	// The directory entry itself is cached and must be flushed to disk
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
