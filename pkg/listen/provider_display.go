package listen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

const builtinProviderName = "anthropic"

func effectiveProviderName(cfg *configpkg.Config, info *configpkg.SessionInfo) string {
	if info != nil && info.ProviderName != "" {
		return info.ProviderName
	}
	if cfg != nil && cfg.ActiveProvider != "" {
		return cfg.ActiveProvider
	}
	return builtinProviderName
}

func providerSource(cfg *configpkg.Config, info *configpkg.SessionInfo) string {
	if info != nil && info.ProviderName != "" {
		return "session"
	}
	if cfg != nil && cfg.ActiveProvider != "" {
		return "active default"
	}
	return "builtin default"
}

func defaultProviderName(cfg *configpkg.Config) string {
	return effectiveProviderName(cfg, nil)
}

func providerSummary(cfg *configpkg.Config, info *configpkg.SessionInfo) string {
	return fmt.Sprintf("provider: %s\nsource: %s", effectiveProviderName(cfg, info), providerSource(cfg, info))
}

func explicitProviderSummary(providerName string) string {
	if providerName == "" {
		providerName = builtinProviderName
	}
	return fmt.Sprintf("provider: %s\nsource: explicit", providerName)
}

func selectedProviderSummary(providerName string) string {
	if providerName == "" {
		providerName = builtinProviderName
	}
	return fmt.Sprintf("provider: %s\nsource: selected", providerName)
}

func activeDefaultProviderSummary(cfg *configpkg.Config) string {
	return fmt.Sprintf("provider: %s\nsource: %s", defaultProviderName(cfg), providerSource(cfg, nil))
}

func isCodexProviderName(providerName string) bool {
	return strings.EqualFold(providerName, "codex")
}

func agentDisplayName(providerName string) string {
	if isCodexProviderName(providerName) {
		return "Codex"
	}
	return "Claude"
}

func agentOptionLabel(providerName string) string {
	if providerName == builtinProviderName {
		return "Claude"
	}
	if isCodexProviderName(providerName) {
		return "Codex"
	}
	return providerName
}

func shortSessionID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "..."
}

func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		if path == home {
			return "~"
		}
		prefix := home + string(filepath.Separator)
		if strings.HasPrefix(path, prefix) {
			return "~/" + strings.TrimPrefix(path, prefix)
		}
	}
	return path
}
