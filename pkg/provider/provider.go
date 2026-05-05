package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tuannvm/ccc/pkg/config"
)

// Provider defines the interface for AI providers that can be used with ccc
type Provider interface {
	Name() string
	Backend() string
	BaseURL() string
	AuthToken(cfg *config.Config) string
	Models() ModelConfig
	ConfigDir() string
	TranscriptPath(sessionID string) string
	EnvVars(cfg *config.Config) []string
	IsBuiltin() bool
}

const (
	BackendClaude = "claude"
	BackendCodex  = "codex"
)

// ModelConfig holds the model names for a provider
type ModelConfig struct {
	Opus     string
	Sonnet   string
	Haiku    string
	Subagent string
}

// BuiltinProvider represents the default/unnamed provider
type BuiltinProvider struct{}

func (BuiltinProvider) Name() string { return "anthropic" }

func (BuiltinProvider) Backend() string { return BackendClaude }

func (BuiltinProvider) BaseURL() string { return "" }

func (BuiltinProvider) AuthToken(cfg *config.Config) string {
	if cfg != nil && cfg.OAuthToken != "" {
		return cfg.OAuthToken
	}
	return ""
}

func (BuiltinProvider) Models() ModelConfig { return ModelConfig{} }

func (BuiltinProvider) ConfigDir() string { return "" }

func (BuiltinProvider) TranscriptPath(string) string { return "" }

func (BuiltinProvider) EnvVars(cfg *config.Config) []string {
	var vars []string
	if cfg != nil && cfg.OAuthToken != "" {
		vars = append(vars, "ANTHROPIC_AUTH_TOKEN="+cfg.OAuthToken)
	}
	return vars
}

func (BuiltinProvider) IsBuiltin() bool { return true }

// CodexProvider represents Codex CLI as a native backend.
type CodexProvider struct {
	ProviderName string
	Config       *config.ProviderConfig
}

func (p CodexProvider) Name() string {
	if p.ProviderName != "" {
		return p.ProviderName
	}
	return BackendCodex
}

func (CodexProvider) Backend() string { return BackendCodex }

func (p CodexProvider) BaseURL() string {
	if p.Config == nil {
		return ""
	}
	return p.Config.BaseURL
}

func (CodexProvider) AuthToken(*config.Config) string { return "" }

func (p CodexProvider) Models() ModelConfig {
	if p.Config == nil {
		return ModelConfig{}
	}
	return ModelConfig{
		Opus:   p.Config.OpusModel,
		Sonnet: p.Config.SonnetModel,
		Haiku:  p.Config.HaikuModel,
	}
}

func (p CodexProvider) ConfigDir() string {
	if p.Config == nil {
		return ""
	}
	return expandProviderConfigDir(p.Config.ConfigDir)
}

func (CodexProvider) TranscriptPath(string) string { return "" }

func (p CodexProvider) EnvVars(*config.Config) []string {
	vars := []string{"CODEX_SANDBOX_NETWORK_DISABLED=1"}
	if configDir := p.ConfigDir(); configDir != "" {
		vars = append(vars, "CODEX_HOME="+configDir)
	}
	return vars
}

func (CodexProvider) IsBuiltin() bool { return true }

// IsCodexProviderName reports whether a provider name selects the Codex CLI backend.
func IsCodexProviderName(name string) bool {
	return strings.EqualFold(name, BackendCodex) || strings.EqualFold(name, "codex-anthropic")
}

// ConfiguredProvider represents a provider configured in config.json
type ConfiguredProvider struct {
	ProviderName string
	Config       *config.ProviderConfig
}

func (p ConfiguredProvider) Name() string { return p.ProviderName }

func (p ConfiguredProvider) Backend() string {
	if p.Config != nil && p.Config.Backend != "" {
		return strings.ToLower(p.Config.Backend)
	}
	return BackendClaude
}

func (p ConfiguredProvider) BaseURL() string {
	if p.Config == nil {
		return ""
	}
	return p.Config.BaseURL
}

func (p ConfiguredProvider) AuthToken(cfg *config.Config) string {
	if p.Config == nil {
		return ""
	}
	if p.Config.AuthToken != "" {
		return p.Config.AuthToken
	}
	if p.Config.AuthEnvVar != "" {
		return "$" + p.Config.AuthEnvVar
	}
	return ""
}

func (p ConfiguredProvider) Models() ModelConfig {
	if p.Config == nil {
		return ModelConfig{}
	}
	return ModelConfig{
		Opus:     p.Config.OpusModel,
		Sonnet:   p.Config.SonnetModel,
		Haiku:    p.Config.HaikuModel,
		Subagent: p.Config.SubagentModel,
	}
}

func (p ConfiguredProvider) ConfigDir() string {
	if p.Config == nil {
		return ""
	}
	return expandProviderConfigDir(p.Config.ConfigDir)
}

func expandProviderConfigDir(configDir string) string {
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
	configDir := p.ConfigDir()
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "claude-cli", "transcripts", sessionID, "transcript.jsonl")
}

func (p ConfiguredProvider) EnvVars(cfg *config.Config) []string {
	if p.Config == nil {
		return nil
	}
	var vars []string

	if p.Config.BaseURL != "" {
		vars = append(vars, "ANTHROPIC_BASE_URL="+p.Config.BaseURL)
	}

	if p.Config.AuthEnvVar != "" {
		vars = append(vars, "ANTHROPIC_AUTH_TOKEN=$"+p.Config.AuthEnvVar)
	} else if p.Config.AuthToken != "" {
		vars = append(vars, "ANTHROPIC_AUTH_TOKEN="+p.Config.AuthToken)
	}

	if p.Config.OpusModel != "" {
		vars = append(vars, "ANTHROPIC_DEFAULT_OPUS_MODEL="+p.Config.OpusModel)
	}
	if p.Config.SonnetModel != "" {
		vars = append(vars, "ANTHROPIC_DEFAULT_SONNET_MODEL="+p.Config.SonnetModel)
		vars = append(vars, "ANTHROPIC_MODEL="+p.Config.SonnetModel)
	} else if p.Config.OpusModel != "" {
		vars = append(vars, "ANTHROPIC_MODEL="+p.Config.OpusModel)
	}
	if p.Config.HaikuModel != "" {
		vars = append(vars, "ANTHROPIC_DEFAULT_HAIKU_MODEL="+p.Config.HaikuModel)
	}
	if p.Config.SubagentModel != "" {
		vars = append(vars, "CLAUDE_CODE_SUBAGENT_MODEL="+p.Config.SubagentModel)
	}

	if p.Config.ConfigDir != "" {
		vars = append(vars, "CLAUDE_CONFIG_DIR="+p.ConfigDir())
	}

	if p.Config.ApiTimeout > 0 {
		vars = append(vars, fmt.Sprintf("API_TIMEOUT_MS=%d", p.Config.ApiTimeout))
	}

	return vars
}

func (p ConfiguredProvider) IsBuiltin() bool { return false }

// GetActiveProvider returns the active provider config
func GetActiveProvider(cfg *config.Config) *config.ProviderConfig {
	if cfg == nil {
		return nil
	}
	if cfg.Providers != nil && cfg.ActiveProvider != "" {
		if p := cfg.Providers[cfg.ActiveProvider]; p != nil {
			return p
		}
	}
	return cfg.Provider
}

// GetProviderNames returns configured provider names
func GetProviderNames(cfg *config.Config) []string {
	var names []string
	names = append(names, "anthropic")
	names = append(names, BackendCodex)
	if cfg != nil && cfg.Providers != nil {
		for name := range cfg.Providers {
			if name != "anthropic" && !strings.EqualFold(name, BackendCodex) {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names[2:])
	return names
}

// GetProvider returns a Provider for the given name
func GetProvider(cfg *config.Config, name string) Provider {
	if name == "" {
		if cfg != nil && cfg.ActiveProvider != "" && cfg.Providers != nil {
			if p := cfg.Providers[cfg.ActiveProvider]; p != nil {
				if strings.EqualFold(p.Backend, BackendCodex) {
					return CodexProvider{ProviderName: cfg.ActiveProvider, Config: p}
				}
				return ConfiguredProvider{ProviderName: cfg.ActiveProvider, Config: p}
			}
		}
		if cfg != nil && IsCodexProviderName(cfg.ActiveProvider) {
			return CodexProvider{ProviderName: cfg.ActiveProvider}
		}
		if cfg != nil && cfg.Provider != nil {
			return ConfiguredProvider{ProviderName: "legacy", Config: cfg.Provider}
		}
		return BuiltinProvider{}
	}

	if name == "anthropic" {
		return BuiltinProvider{}
	}

	if cfg != nil && cfg.Providers != nil {
		if p := cfg.Providers[name]; p != nil {
			if strings.EqualFold(p.Backend, BackendCodex) {
				return CodexProvider{ProviderName: name, Config: p}
			}
			return ConfiguredProvider{ProviderName: name, Config: p}
		}
	}
	if IsCodexProviderName(name) {
		return CodexProvider{ProviderName: name}
	}

	return nil
}

// EnsureProviderSettings updates the provider's settings.json with trusted directories
func EnsureProviderSettings(p Provider) error {
	if p == nil || p.ConfigDir() == "" {
		return nil
	}

	configDir := p.ConfigDir()
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

	var settings map[string]any
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("settings file %q contains invalid JSON: %w", settingsPath, err)
		}
		if settings == nil {
			settings = make(map[string]any)
		}
	} else if os.IsNotExist(err) {
		settings = make(map[string]any)
	} else {
		return fmt.Errorf("failed to read settings file: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	_, hasTrustedDirs := settings["trustedDirectories"]
	hasTrustDirAutoApprove := false
	if autoApprove, ok := settings["autoApprove"].(map[string]any); ok {
		if trustDirs, ok := autoApprove["trustDirectories"].(bool); ok {
			hasTrustDirAutoApprove = trustDirs
		}
	}

	if !hasTrustedDirs || !hasTrustDirAutoApprove {
		trustedDirs := []any{
			home,
			filepath.Join(home, "Projects"),
			filepath.Join(home, "Projects", "cli"),
			filepath.Join(home, "Projects", "sandbox"),
		}

		if existingDirs, ok := settings["trustedDirectories"].([]any); ok && len(existingDirs) > 0 {
			for _, dir := range existingDirs {
				if dirStr, ok := dir.(string); ok {
					trustedDirs = append(trustedDirs, dirStr)
				}
			}
		}

		settings["trustedDirectories"] = trustedDirs

		var autoApprove map[string]any
		if existingAA, ok := settings["autoApprove"].(map[string]any); ok {
			autoApprove = existingAA
		} else {
			autoApprove = make(map[string]any)
		}
		autoApprove["trustDirectories"] = true
		settings["autoApprove"] = autoApprove

		newData, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}

		os.MkdirAll(configDir, 0755)

		if err := os.WriteFile(settingsPath, newData, 0600); err != nil {
			return err
		}
	}

	return nil
}
