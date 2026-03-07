package config

import (
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
