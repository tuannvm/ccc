package main

import (
	"fmt"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
)

// installHook prints deprecation notice (hooks now managed per-project)
func installHook() error {
	fmt.Println("⚠️ Global hook installation is deprecated. Hooks are now installed per-project automatically.")
	fmt.Println("💡 Hooks will be automatically installed when you create or resume a session.")
	return nil
}

// uninstallHook prints deprecation notice
func uninstallHook() error {
	fmt.Println("⚠️ Global hook uninstallation is deprecated.")
	fmt.Println("💡 To remove hooks from a project, delete the .claude/settings.local.json file in that project.")
	fmt.Println("💡 To cleanup old global hooks, use: ccc cleanup-hooks")
	return nil
}

func installSkill() error {
	return hooks.InstallSkill()
}

func uninstallSkill() {
	hooks.UninstallSkill()
}

func cleanupGlobalHooks() error {
	return hooks.CleanupGlobalHooks(configpkg.Load)
}

func installHooksToCurrentDir() error {
	return hooks.InstallHooksToCurrentDir()
}

// ensureHooksForSession ensures ccc hooks are installed in the session's project directory
func ensureHooksForSession(config *Config, sessionName string, sessionInfo *SessionInfo) error {
	return hooks.EnsureHooksForSession(&hooks.EnsureHooksForSessionConfig{
		Config:            config,
		SessionName:       sessionName,
		SessionInfo:       sessionInfo,
		GetSessionWorkDir: getSessionWorkDir,
	})
}
