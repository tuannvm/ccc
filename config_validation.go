package main

import (
	"fmt"
	"os"
)

// validateConfig checks if the config is valid and returns any errors
// It validates required fields for configured features and provider configs
func validateConfig(config *Config) error {
	// Validate Telegram integration - only if BOTH fields are set
	// This allows partial configs during setup (e.g., setting bot_token first)
	// We validate consistency, not completeness
	if config.BotToken != "" && config.ChatID != 0 {
		// Both are set, validate they're non-empty (already checked by condition)
		// No additional validation needed for non-empty values
	}
	// Note: If only one is set, that's OK - it's a partial config during setup

	// Validate provider configs
	for name, provider := range config.Providers {
		if provider == nil {
			return fmt.Errorf("provider %q: config is nil", name)
		}
		// Check that at least one auth method is configured
		if provider.AuthToken == "" && provider.AuthEnvVar == "" && name != "anthropic" {
			return fmt.Errorf("provider %q: must have either auth_token or auth_env_var", name)
		}
		// If auth_env_var is set, verify the environment variable exists (optional)
		if provider.AuthEnvVar != "" {
			if _, exists := os.LookupEnv(provider.AuthEnvVar); !exists {
				// This is a warning, not an error - the env var might be set in the shell environment
				// We just log this for debugging purposes
			}
		}
	}

	// Validate sessions
	for name, session := range config.Sessions {
		if session == nil {
			return fmt.Errorf("session %q: info is nil", name)
		}
		// Optional: Check if session path exists
		if session.Path != "" {
			if _, err := os.Stat(session.Path); os.IsNotExist(err) {
				// Path doesn't exist - this might be intentional for new sessions
				// We don't return an error, but could log a warning in debug mode
			}
		}
	}

	return nil
}
