package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func saveConfig(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := getConfigPath()
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

// getActiveProvider returns the active provider config.
// First checks providers map + active_provider, then falls back to legacy provider field.
func getActiveProvider(config *Config) *ProviderConfig {
	// New style: providers map with active_provider
	if config.Providers != nil && config.ActiveProvider != "" {
		if provider := config.Providers[config.ActiveProvider]; provider != nil {
			return provider
		}
	}
	// Legacy: direct provider field
	return config.Provider
}

// getProviderNames returns a list of configured provider names
// Includes the builtin 'anthropic' provider and all configured providers
func getProviderNames(config *Config) []string {
	var names []string
	// Always include anthropic as a built-in option
	names = append(names, "anthropic")
	// Add configured providers
	if config.Providers != nil {
		for name := range config.Providers {
			if name != "anthropic" {
				names = append(names, name)
			}
		}
	}
	return names
}

// getProvider returns a Provider interface for the given provider name
// If name is empty, returns the active provider (or builtin if none active)
// Returns nil if provider name is specified but not found
func getProvider(config *Config, name string) Provider {
	if name == "" {
		// No specific name requested - use active provider or builtin
		if config.ActiveProvider != "" && config.Providers != nil {
			if p := config.Providers[config.ActiveProvider]; p != nil {
				return ConfiguredProvider{name: config.ActiveProvider, config: p}
			}
		}
		// Legacy fallback: check config.Provider (old single-provider config)
		if config.Provider != nil {
			return ConfiguredProvider{name: "legacy", config: config.Provider}
		}
		return BuiltinProvider{}
	}

	// Specific name requested
	if name == "anthropic" {
		return BuiltinProvider{}
	}

	if config.Providers != nil {
		if p := config.Providers[name]; p != nil {
			return ConfiguredProvider{name: name, config: p}
		}
	}

	// Unknown provider
	return nil
}

// ensureProviderSettings updates the provider's settings.json with trusted directories
// This prevents the "Do you trust the files in this folder?" prompt
func ensureProviderSettings(provider Provider) error {
	if provider == nil || provider.ConfigDir() == "" {
		return nil // No provider config dir, nothing to do
	}

	// Expand ~ in config dir path
	configDir := provider.ConfigDir()
	if configDir == "~" || configDir == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot resolve home directory for config path: %w", err)
		}
		configDir = home
	} else if strings.HasPrefix(configDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot resolve home directory for config path: %w", err)
		}
		configDir = filepath.Join(home, configDir[2:])
	}

	settingsPath := filepath.Join(configDir, "settings.json")

	// Read existing settings or create new
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			// Invalid JSON - return error to avoid data loss
			// The user should fix the malformed file manually
			return fmt.Errorf("settings file %q contains invalid JSON: %w", settingsPath, err)
		}
		// Initialize map if JSON was null (json.Unmarshal leaves it as nil)
		if settings == nil {
			settings = make(map[string]interface{})
		}
	} else if os.IsNotExist(err) {
		// File doesn't exist, create new settings
		settings = make(map[string]interface{})
	} else {
		// Other error (permission, I/O, etc.) - return error to avoid data loss
		return fmt.Errorf("failed to read settings file: %w", err)
	}

	// Get home directory for trusted paths
	home, err := os.UserHomeDir()
	if err != nil {
		// Can't resolve home directory, skip adding trusted directories
		// but continue with the rest of the settings
		listenLog("ensureProviderSettings: cannot resolve home directory for trusted paths: %v (skipping trusted directories)", err)
		return nil
	}

	// Check if trustedDirectories and trustDirectories auto-approve are already configured
	_, hasTrustedDirs := settings["trustedDirectories"]
	hasTrustDirAutoApprove := false
	if autoApprove, ok := settings["autoApprove"].(map[string]interface{}); ok {
		if trustDirs, ok := autoApprove["trustDirectories"].(bool); ok {
			hasTrustDirAutoApprove = trustDirs
		}
	}

	// Only update if not already configured
	if !hasTrustedDirs || !hasTrustDirAutoApprove {
		// Add trusted directories
		// Trust home directory and common project locations
		trustedDirs := []interface{}{
			home,
			filepath.Join(home, "Projects"),
			filepath.Join(home, "Projects", "cli"),
			filepath.Join(home, "Projects", "sandbox"),
		}

		// If existing trusted directories exist, preserve them
		if existingDirs, ok := settings["trustedDirectories"].([]interface{}); ok && len(existingDirs) > 0 {
			for _, dir := range existingDirs {
				if dirStr, ok := dir.(string); ok {
					trustedDirs = append(trustedDirs, dirStr)
				}
			}
		}

		settings["trustedDirectories"] = trustedDirs

		// Add autoApprove for trust directories
		// Preserve existing autoApprove settings if they exist
		var autoApprove map[string]interface{}
		if existingAA, ok := settings["autoApprove"].(map[string]interface{}); ok {
			autoApprove = existingAA
		} else {
			autoApprove = make(map[string]interface{})
		}
		autoApprove["trustDirectories"] = true
		settings["autoApprove"] = autoApprove

		// Write updated settings
		newData, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}

		// Create config directory if it doesn't exist
		os.MkdirAll(configDir, 0755)

		if err := os.WriteFile(settingsPath, newData, 0600); err != nil {
			return err
		}

		return nil
	}

	return nil
}
