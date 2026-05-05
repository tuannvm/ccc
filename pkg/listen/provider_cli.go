package listen

import (
	"fmt"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/lookup"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
)

// RunProviderFromArgs mirrors Telegram's /provider command for the terminal.
func RunProviderFromArgs(args []string) error {
	cfg, err := configpkg.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cwd := currentWorkingDir()
	sessionName, info := lookup.FindSessionForPath(cfg, cwd)
	if len(args) == 0 {
		fmt.Print(BuildCLIProviderStatus(cfg, sessionName, info))
		return nil
	}

	providerName := args[0]
	provider := providerpkg.GetProvider(cfg, providerName)
	if provider == nil {
		return fmt.Errorf("unknown provider %q; available: %s", providerName, strings.Join(providerpkg.GetProviderNames(cfg), ", "))
	}
	if sessionName == "" || info == nil {
		return fmt.Errorf("no session mapped to this directory; run 'ccc status all' to pick a session")
	}

	previousProvider := info.ProviderName
	info.ProviderName = provider.Name()
	if err := configpkg.Save(cfg); err != nil {
		info.ProviderName = previousProvider
		return fmt.Errorf("failed to save provider: %w", err)
	}
	pinSessionHeader(cfg, sessionName, info)

	fmt.Printf("provider changed\nsession: %s\nprovider: %s\nsource: session\n\nrestart: ccc status restart\n", sessionName, provider.Name())
	return nil
}

func BuildCLIProviderStatus(cfg *configpkg.Config, sessionName string, info *configpkg.SessionInfo) string {
	var lines []string
	if sessionName != "" && info != nil {
		lines = append(lines, fmt.Sprintf("session: %s", sessionName))
		lines = append(lines, providerSummary(cfg, info))
		lines = append(lines, "")
	}
	lines = append(lines, "providers")
	for _, name := range providerpkg.GetProviderNames(cfg) {
		active := ""
		if sessionName != "" && info != nil {
			if effectiveProviderName(cfg, info) == name {
				active = " (current)"
			}
		} else if cfg.ActiveProvider == name || (cfg.ActiveProvider == "" && name == builtinProviderName) {
			active = " (active)"
		}
		if name == builtinProviderName || strings.EqualFold(name, "codex") {
			lines = append(lines, fmt.Sprintf("  - %s%s (builtin)", name, active))
		} else {
			lines = append(lines, fmt.Sprintf("  - %s%s", name, active))
		}
	}
	if sessionName != "" && info != nil {
		lines = append(lines, "", "change: ccc provider <name>")
	} else {
		lines = append(lines, "", "run inside a mapped project directory to change a session provider")
	}
	return strings.Join(lines, "\n") + "\n"
}
