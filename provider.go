package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Provider defines the interface for AI providers that can be used with ccc
// This abstraction allows for provider-agnostic design where all providers
// are treated equally without hardcoded special cases
type Provider interface {
	// Name returns the provider's identifier (e.g., "anthropic", "zai", "openai")
	Name() string

	// BaseURL returns the API base URL for this provider
	BaseURL() string

	// AuthToken returns the API key/token for this provider
	// Respects auth_token or auth_env_var from config
	AuthToken(config *Config) string

	// Models returns the model configuration for this provider
	Models() ModelConfig

	// ConfigDir returns the provider-specific config directory
	// Empty string means use default Claude Code config dir
	ConfigDir() string

	// TranscriptPath returns the path to transcripts for a given session
	// Empty string means use the default location
	TranscriptPath(sessionID string) string

	// EnvVars returns the environment variables to set for this provider
	EnvVars(config *Config) []string

	// IsBuiltin returns true if this is the default/unnamed provider
	// The builtin provider uses environment variables and default paths
	IsBuiltin() bool
}

// ModelConfig holds the model names for a provider
type ModelConfig struct {
	Opus     string
	Sonnet   string
	Haiku    string
	Subagent string
}

// BuiltinProvider represents the default/unnamed provider that uses
// environment variables and Claude Code's default configuration
type BuiltinProvider struct{}

func (BuiltinProvider) Name() string {
	return "anthropic"
}

func (BuiltinProvider) BaseURL() string {
	// Builtin provider uses ANTHROPIC_BASE_URL env var if set
	// or Claude Code's default
	return ""
}

func (BuiltinProvider) AuthToken(config *Config) string {
	// Builtin provider checks config.OAuthToken (for user-specified token)
	// or uses Claude's own auth (CLAUDE_API_KEY or OAuth)
	if config != nil && config.OAuthToken != "" {
		return config.OAuthToken
	}
	// CLAUDE_API_KEY or Claude's OAuth will be used
	return ""
}

func (BuiltinProvider) Models() ModelConfig {
	// Builtin provider uses ANTHROPIC_DEFAULT_*_MODEL env vars
	// or Claude Code's defaults
	// The active provider's models are used when set
	return ModelConfig{}
}

func (BuiltinProvider) ConfigDir() string {
	// Builtin provider uses Claude Code's default config dir
	return ""
}

func (BuiltinProvider) TranscriptPath(sessionID string) string {
	// Builtin provider uses Claude Code's default transcript location
	// Default: ~/.claude/transcripts/<session-id>/transcript.jsonl
	return ""
}

func (BuiltinProvider) EnvVars(config *Config) []string {
	var vars []string

	// BuiltinProvider uses Claude Code's defaults
	// Only set OAuth token if configured (for Claude Code auth)
	// Do NOT inherit from active configured provider - that would break explicit provider override
	if config != nil && config.OAuthToken != "" {
		vars = append(vars, "ANTHROPIC_AUTH_TOKEN="+config.OAuthToken)
	}

	// BuiltinProvider does NOT set base_url, models, or config_dir
	// These are left to Claude Code's own defaults and environment detection

	return vars
}

func (BuiltinProvider) IsBuiltin() bool {
	return true
}

// ConfiguredProvider represents a provider configured in config.json
type ConfiguredProvider struct {
	name   string
	config *ProviderConfig
}

func (p ConfiguredProvider) Name() string {
	return p.name
}

func (p ConfiguredProvider) BaseURL() string {
	if p.config == nil {
		return ""
	}
	return p.config.BaseURL
}

func (p ConfiguredProvider) AuthToken(config *Config) string {
	if p.config == nil {
		return ""
	}
	// Check auth_token first, then auth_env_var
	if p.config.AuthToken != "" {
		return p.config.AuthToken
	}
	if p.config.AuthEnvVar != "" {
		// Read from environment (not from config to avoid storing secrets)
		// The caller should handle os.Getenv lookup
		return "$" + p.config.AuthEnvVar
	}
	return ""
}

func (p ConfiguredProvider) Models() ModelConfig {
	if p.config == nil {
		return ModelConfig{}
	}
	return ModelConfig{
		Opus:     p.config.OpusModel,
		Sonnet:   p.config.SonnetModel,
		Haiku:    p.config.HaikuModel,
		Subagent: p.config.SubagentModel,
	}
}

func (p ConfiguredProvider) ConfigDir() string {
	if p.config == nil {
		return ""
	}
	configDir := p.config.ConfigDir
	// Expand ~ to home directory
	if configDir == "~" || configDir == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return configDir
		}
		return home
	}
	if strings.HasPrefix(configDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return configDir
		}
		return filepath.Join(home, configDir[2:])
	}
	return configDir
}

func (p ConfiguredProvider) TranscriptPath(sessionID string) string {
	if p.config == nil || p.config.ConfigDir == "" {
		return ""
	}
	// Provider-specific transcript location
	// Format: <config_dir>/claude-cli/transcripts/<session-id>/transcript.jsonl
	return filepath.Join(p.config.ConfigDir, "claude-cli", "transcripts", sessionID, "transcript.jsonl")
}

func (p ConfiguredProvider) EnvVars(config *Config) []string {
	if p.config == nil {
		return nil
	}
	var vars []string

	// Base URL
	if p.config.BaseURL != "" {
		vars = append(vars, "ANTHROPIC_BASE_URL="+p.config.BaseURL)
	}

	// Auth token support
	// Priority: auth_env_var (expanded from env) > auth_token (direct value)
	// For auth_env_var, use $ prefix to signal applyProviderEnv to expand from os.Getenv
	// For auth_token, set directly (no $ prefix needed)
	if p.config.AuthEnvVar != "" {
		// Mark with $ prefix for expansion in applyProviderEnv
		vars = append(vars, "ANTHROPIC_AUTH_TOKEN=$"+p.config.AuthEnvVar)
	} else if p.config.AuthToken != "" {
		// Direct token value (deprecated but supported for backward compatibility)
		vars = append(vars, "ANTHROPIC_AUTH_TOKEN="+p.config.AuthToken)
	}

	// Models
	if p.config.OpusModel != "" {
		vars = append(vars, "ANTHROPIC_DEFAULT_OPUS_MODEL="+p.config.OpusModel)
	}
	if p.config.SonnetModel != "" {
		vars = append(vars, "ANTHROPIC_DEFAULT_SONNET_MODEL="+p.config.SonnetModel)
		vars = append(vars, "ANTHROPIC_MODEL="+p.config.SonnetModel)
	} else if p.config.OpusModel != "" {
		// Fallback: if Sonnet is not configured but Opus is, use Opus as default model
		vars = append(vars, "ANTHROPIC_MODEL="+p.config.OpusModel)
	}
	if p.config.HaikuModel != "" {
		vars = append(vars, "ANTHROPIC_DEFAULT_HAIKU_MODEL="+p.config.HaikuModel)
	}
	if p.config.SubagentModel != "" {
		vars = append(vars, "CLAUDE_CODE_SUBAGENT_MODEL="+p.config.SubagentModel)
	}

	// Config dir - use ConfigDir() method to expand ~ to home directory
	if p.config.ConfigDir != "" {
		vars = append(vars, "CLAUDE_CONFIG_DIR="+p.ConfigDir())
	}

	// API timeout
	if p.config.ApiTimeout > 0 {
		vars = append(vars, fmt.Sprintf("API_TIMEOUT_MS=%d", p.config.ApiTimeout))
	}

	return vars
}

func (p ConfiguredProvider) IsBuiltin() bool {
	return false
}

// ========== Provider Helper Functions ==========

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
	var settings map[string]any
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			// Invalid JSON - return error to avoid data loss
			// The user should fix the malformed file manually
			return fmt.Errorf("settings file %q contains invalid JSON: %w", settingsPath, err)
		}
		// Initialize map if JSON was null (json.Unmarshal leaves it as nil)
		if settings == nil {
			settings = make(map[string]any)
		}
	} else if os.IsNotExist(err) {
		// File doesn't exist, create new settings
		settings = make(map[string]any)
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
	if autoApprove, ok := settings["autoApprove"].(map[string]any); ok {
		if trustDirs, ok := autoApprove["trustDirectories"].(bool); ok {
			hasTrustDirAutoApprove = trustDirs
		}
	}

	// Only update if not already configured
	if !hasTrustedDirs || !hasTrustDirAutoApprove {
		// Add trusted directories
		// Trust home directory and common project locations
		trustedDirs := []any{
			home,
			filepath.Join(home, "Projects"),
			filepath.Join(home, "Projects", "cli"),
			filepath.Join(home, "Projects", "sandbox"),
		}

		// If existing trusted directories exist, preserve them
		if existingDirs, ok := settings["trustedDirectories"].([]any); ok && len(existingDirs) > 0 {
			for _, dir := range existingDirs {
				if dirStr, ok := dir.(string); ok {
					trustedDirs = append(trustedDirs, dirStr)
				}
			}
		}

		settings["trustedDirectories"] = trustedDirs

		// Add autoApprove for trust directories
		// Preserve existing autoApprove settings if they exist
		var autoApprove map[string]any
		if existingAA, ok := settings["autoApprove"].(map[string]any); ok {
			autoApprove = existingAA
		} else {
			autoApprove = make(map[string]any)
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
