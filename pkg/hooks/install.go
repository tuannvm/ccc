package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// IsCccHook checks if a hook entry contains a ccc command
func IsCccHook(entry any) bool {
	if m, ok := entry.(map[string]any); ok {
		if cmd, ok := m["command"].(string); ok {
			return strings.Contains(cmd, "ccc hook")
		}
		if hooks, ok := m["hooks"].([]any); ok {
			for _, h := range hooks {
				if hm, ok := h.(map[string]any); ok {
					if cmd, ok := hm["command"].(string); ok {
						if strings.Contains(cmd, "ccc hook") {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// RemoveCccHooks filters out ccc hooks from a hook array
func RemoveCccHooks(hookArray []any) []any {
	var result []any
	for _, entry := range hookArray {
		if !IsCccHook(entry) {
			result = append(result, entry)
		}
	}
	return result
}

// InstallHooksForProject installs ccc hooks to a project's .claude/settings.local.json
func InstallHooksForProject(projectPath string) error {
	settingsLocalPath := filepath.Join(projectPath, ".claude", "settings.local.json")

	// Ensure .claude directory exists
	claudeDir := filepath.Dir(settingsLocalPath)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	// Install hooks
	if err := InstallHooksToPath(settingsLocalPath, true); err != nil {
		return fmt.Errorf("failed to install hooks to %s: %w", settingsLocalPath, err)
	}

	HookLog("install-hooks: installed to %s", settingsLocalPath)
	return nil
}

// VerifyHooksForProject checks if ccc hooks are present in a project's .claude/settings.local.json
func VerifyHooksForProject(projectPath string) bool {
	settingsLocalPath := filepath.Join(projectPath, ".claude", "settings.local.json")

	data, err := os.ReadFile(settingsLocalPath)
	if err != nil {
		HookLog("verify-hooks: no settings.local.json at %s", settingsLocalPath)
		return false
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		HookLog("verify-hooks: failed to parse settings.local.json: %v", err)
		return false
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		HookLog("verify-hooks: no hooks in settings.local.json")
		return false
	}

	// Check if ccc hooks are present
	hasCccHooks := false
	requiredHooks := []string{"PreToolUse", "Stop", "UserPromptSubmit", "Notification"}

	for _, hookType := range requiredHooks {
		if hookEntries, exists := hooks[hookType].([]any); exists {
			for _, entry := range hookEntries {
				if entryMap, ok := entry.(map[string]any); ok {
					if cmd, ok := entryMap["command"].(string); ok {
						if strings.Contains(cmd, "ccc hook-") {
							hasCccHooks = true
							break
						}
					}
				}
			}
		}
	}

	HookLog("verify-hooks: hasCccHooks=%v for %s", hasCccHooks, projectPath)
	return hasCccHooks
}

// InstallHooksToPath installs ccc hooks to a settings.json file
func InstallHooksToPath(settingsPath string, isLocal bool) error {
	// Ensure directory exists
	settingsDir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Read existing settings or create new
	var settings map[string]any
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		// File doesn't exist, create empty settings
		settings = make(map[string]any)
	} else if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse settings: %w", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
	}

	cccPath := tmux.CCCPath
	cccHooks := map[string][]any{
		"PreToolUse": {
			map[string]any{
				"hooks": []any{
					map[string]any{
						"command": cccPath + " hook-permission",
						"type":    "command",
						"timeout": 300000,
					},
				},
				"matcher": "",
			},
		},
		"Stop": {
			map[string]any{
				"hooks": []any{
					map[string]any{
						"command": cccPath + " hook-stop",
						"type":    "command",
					},
				},
			},
		},
		"PostToolUse": {
			map[string]any{
				"hooks": []any{
					map[string]any{
						"command": cccPath + " hook-post-tool",
						"type":    "command",
					},
				},
			},
		},
		"UserPromptSubmit": {
			map[string]any{
				"hooks": []any{
					map[string]any{
						"command": cccPath + " hook-user-prompt",
						"type":    "command",
					},
				},
			},
		},
		"Notification": {
			map[string]any{
				"hooks": []any{
					map[string]any{
						"command": cccPath + " hook-notification",
						"type":    "command",
					},
				},
			},
		},
	}

	// For settings.local.json, we completely replace hooks (not merge)
	// This ensures only ccc hooks are in the project-local settings
	if isLocal {
		// Remove ALL existing ccc hooks from all hook types (clean slate)
		allHookTypes := []string{"Stop", "Notification", "PermissionRequest", "PostToolUse", "PreToolUse", "UserPromptSubmit"}
		for _, hookType := range allHookTypes {
			delete(hooks, hookType)
		}

		// Add only our hooks (no merging)
		for hookType, newHooks := range cccHooks {
			hooks[hookType] = newHooks
		}
	} else {
		// Legacy behavior for global settings: merge with existing hooks
		// Remove ALL existing ccc hooks from all hook types
		allHookTypes := []string{"Stop", "Notification", "PermissionRequest", "PostToolUse", "PreToolUse", "UserPromptSubmit"}
		for _, hookType := range allHookTypes {
			if existing, ok := hooks[hookType].([]any); ok {
				filtered := RemoveCccHooks(existing)
				if len(filtered) == 0 {
					delete(hooks, hookType)
				} else {
					hooks[hookType] = filtered
				}
			}
		}

		// Add only the hooks we need
		for hookType, newHooks := range cccHooks {
			var existingHooks []any
			if existing, ok := hooks[hookType].([]any); ok {
				existingHooks = existing
			}
			hooks[hookType] = append(newHooks, existingHooks...)
		}
	}

	settings["hooks"] = hooks

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, newData, 0600); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	return nil
}

// CleanupGlobalHooks removes ccc hooks from global config files
// This is used to clean up old installations that installed hooks to global settings
func CleanupGlobalHooks(loadConfig func() (*config.Config, error)) error {
	home, _ := os.UserHomeDir()
	defaultSettingsPath := filepath.Join(home, ".claude", "settings.json")

	// Load config to get all provider config dirs
	cfg, err := loadConfig()
	cleanedCount := 0
	configDirs := make(map[string]bool)

	if err == nil && cfg.Providers != nil {
		// Collect all unique config dirs
		for _, provider := range cfg.Providers {
			if provider.ConfigDir != "" {
				// Expand ~
				configDir := provider.ConfigDir
				if strings.HasPrefix(configDir, "~/") {
					configDir = filepath.Join(home, configDir[2:])
				} else if configDir == "~" {
					configDir = home
				}
				configDirs[configDir] = true
			}
		}
	}

	// Cleanup hooks from each provider config dir
	for configDir := range configDirs {
		providerSettingsPath := filepath.Join(configDir, "settings.json")
		if _, err := os.Stat(providerSettingsPath); err == nil {
			if err := UninstallHooksFromPath(providerSettingsPath); err != nil {
				fmt.Printf("⚠️ Failed to cleanup hooks from %s: %v\n", configDir, err)
			} else {
				fmt.Printf("✅ Cleaned up hooks from %s\n", configDir)
				cleanedCount++
			}
		}
	}

	// Always cleanup from default ~/.claude
	if _, err := os.Stat(defaultSettingsPath); err == nil {
		if err := UninstallHooksFromPath(defaultSettingsPath); err != nil {
			fmt.Printf("⚠️ Failed to cleanup hooks from %s: %v\n", defaultSettingsPath, err)
		} else {
			fmt.Printf("✅ Cleaned up hooks from %s\n", defaultSettingsPath)
			cleanedCount++
		}
	}

	if cleanedCount == 0 {
		fmt.Println("✨ No global hooks found to cleanup")
		return nil
	}

	fmt.Printf("✅ Cleaned up ccc hooks from %d location(s)\n", cleanedCount)
	fmt.Println("💡 Hooks are now managed per-project in .claude/settings.local.json")
	return nil
}

// UninstallHooksFromPath removes ccc hooks from a specific settings.json file
func UninstallHooksFromPath(settingsPath string) error {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to read settings.json: %w", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse settings.json: %w", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil // No hooks to remove
	}

	hookTypes := []string{"Stop", "Notification", "PermissionRequest", "PostToolUse", "PreToolUse", "UserPromptSubmit"}
	for _, hookType := range hookTypes {
		if existing, ok := hooks[hookType].([]any); ok {
			filtered := RemoveCccHooks(existing)
			if len(filtered) == 0 {
				delete(hooks, hookType)
			} else {
				hooks[hookType] = filtered
			}
		}
	}

	settings["hooks"] = hooks

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, newData, 0600); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	return nil
}

// InstallSkill installs the ccc-send skill to ~/.claude/skills/
func InstallSkill() error {
	home, _ := os.UserHomeDir()
	skillDir := filepath.Join(home, ".claude", "skills")
	skillPath := filepath.Join(skillDir, "ccc-send.md")

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}

	skillContent := `# CCC Send - File Transfer Skill

## Description
Send files to the user via Telegram using the ccc send command.

## Usage
When the user asks you to send them a file, or when you have generated/built a file that the user needs (like an APK, binary, or any other file), use this command:

` + "```bash" + `
ccc send <file_path>
` + "```" + `

## How it works
- **Small files (< 50MB)**: Sent directly via Telegram
- **Large files (≥ 50MB)**: Streamed via relay server with a one-time download link

## Examples

### Send a built APK
` + "```bash" + `
ccc send ./build/app.apk
` + "```" + `

### Send a generated file
` + "```bash" + `
ccc send ./output/report.pdf
` + "```" + `

### Send from subdirectory
` + "```bash" + `
ccc send ~/Downloads/large-file.zip
` + "```" + `

## Important Notes
- The command detects the current session from your working directory
- For large files, the command will wait up to 10 minutes for the user to download
- Each download link is one-time use only
- Use this proactively when you've created files the user need!
`

	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		return fmt.Errorf("failed to write skill file: %w", err)
	}

	fmt.Println("✅ CCC send skill installed!")
	return nil
}

// UninstallSkill removes the ccc-send skill
func UninstallSkill() error {
	home, _ := os.UserHomeDir()
	skillPath := filepath.Join(home, ".claude", "skills", "ccc-send.md")
	os.Remove(skillPath)
	return nil
}

// InstallHooksToCurrentDir installs ccc hooks to the current directory's .claude/settings.local.json
// This is used by the 'ccc install-hooks' command
func InstallHooksToCurrentDir() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check if hooks are already installed
	if VerifyHooksForProject(cwd) {
		fmt.Printf("✅ Hooks already installed in %s\n", cwd)
		return nil
	}

	// Install hooks
	if err := InstallHooksForProject(cwd); err != nil {
		return fmt.Errorf("failed to install hooks: %w", err)
	}

	fmt.Printf("✅ Hooks installed to %s/.claude/settings.local.json\n", cwd)
	return nil
}

// EnsureHooksForSessionConfig holds configuration for ensuring hooks for a session
type EnsureHooksForSessionConfig struct {
	Config      *config.Config
	SessionName string
	SessionInfo *config.SessionInfo
	// GetSessionWorkDir returns the working directory for a session
	GetSessionWorkDir func(cfg *config.Config, sessionName string, sessionInfo *config.SessionInfo) string
}

// EnsureHooksForSession ensures ccc hooks are installed in the session's project directory
// This should be called when a session is created or resumed
func EnsureHooksForSession(cfg *EnsureHooksForSessionConfig) error {
	if cfg.SessionInfo == nil {
		if cfg.Config == nil || cfg.Config.Sessions == nil {
			return nil
		}
		cfg.SessionInfo = cfg.Config.Sessions[cfg.SessionName]
		if cfg.SessionInfo == nil {
			return nil
		}
	}

	// Get the project path for this session
	projectPath := cfg.GetSessionWorkDir(cfg.Config, cfg.SessionName, cfg.SessionInfo)
	if projectPath == "" {
		return fmt.Errorf("unable to determine project path for session '%s'", cfg.SessionName)
	}

	// Check if hooks are already installed
	if VerifyHooksForProject(projectPath) {
		HookLog("ensure-hooks: hooks already present for %s", projectPath)
		return nil
	}

	// Install hooks to the project
	HookLog("ensure-hooks: installing hooks to %s", projectPath)
	if err := InstallHooksForProject(projectPath); err != nil {
		return fmt.Errorf("failed to install hooks for project %s: %w", projectPath, err)
	}

	HookLog("ensure-hooks: hooks installed successfully for %s", projectPath)
	return nil
}
