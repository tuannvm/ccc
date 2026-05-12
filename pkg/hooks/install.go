package hooks

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// IsCccHook checks if a hook entry contains a ccc command
func IsCccHook(entry any) bool {
	if m, ok := entry.(map[string]any); ok {
		if cmd, ok := m["command"].(string); ok {
			return isCccHookCommand(cmd)
		}
		if hooks, ok := m["hooks"].([]any); ok {
			for _, h := range hooks {
				if hm, ok := h.(map[string]any); ok {
					if cmd, ok := hm["command"].(string); ok {
						if isCccHookCommand(cmd) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func isCccHookCommand(cmd string) bool {
	return strings.Contains(cmd, " hook-") && strings.Contains(cmd, "ccc")
}

// RemoveCccHooks filters out ccc hooks from a hook array
func RemoveCccHooks(hookArray []any) []any {
	var result []any
	for _, entry := range hookArray {
		m, ok := entry.(map[string]any)
		if !ok {
			result = append(result, entry)
			continue
		}
		if cmd, ok := m["command"].(string); ok && isCccHookCommand(cmd) {
			continue
		}
		if nested, ok := m["hooks"].([]any); ok {
			keptNested := make([]any, 0, len(nested))
			for _, hook := range nested {
				hookMap, ok := hook.(map[string]any)
				if !ok {
					keptNested = append(keptNested, hook)
					continue
				}
				cmd, _ := hookMap["command"].(string)
				if isCccHookCommand(cmd) {
					continue
				}
				keptNested = append(keptNested, hook)
			}
			if len(keptNested) == 0 {
				continue
			}
			entryCopy := make(map[string]any, len(m))
			for key, value := range m {
				entryCopy[key] = value
			}
			entryCopy["hooks"] = keptNested
			result = append(result, entryCopy)
			continue
		}
		result = append(result, entry)
	}
	return result
}

func cccHookPath() string {
	if tmux.CCCPath != "" {
		return tmux.CCCPath
	}
	return "ccc"
}

func cccHookCommandPrefix() string {
	return shellQuoteIfNeeded(cccHookPath())
}

func shellQuoteIfNeeded(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\r'\"\\$&;()<>|*?[]{}!#") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
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
						if isCccHookCommand(cmd) {
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

func hasCccHookCommand(hookEntries []any) bool {
	for _, entry := range hookEntries {
		if IsCccHook(entry) {
			return true
		}
	}
	return false
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

	cccPath := cccHookCommandPrefix()
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

// InstallCodexHooksForProject installs ccc hooks to a project's .codex/hooks.json.
func InstallCodexHooksForProject(projectPath string) error {
	hooksPath := filepath.Join(projectPath, ".codex", "hooks.json")

	if err := InstallCodexHooksToPath(hooksPath); err != nil {
		return fmt.Errorf("failed to install Codex hooks to %s: %w", hooksPath, err)
	}

	HookLog("install-codex-hooks: installed to %s", hooksPath)
	return nil
}

// VerifyCodexHooksForProject checks if ccc hooks are present in a project's .codex/hooks.json.
func VerifyCodexHooksForProject(projectPath string) bool {
	hooksPath := filepath.Join(projectPath, ".codex", "hooks.json")

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		HookLog("verify-codex-hooks: no hooks.json at %s", hooksPath)
		return false
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		HookLog("verify-codex-hooks: failed to parse hooks.json: %v", err)
		return false
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		HookLog("verify-codex-hooks: no hooks in hooks.json")
		return false
	}

	requiredHooks := []string{"PreToolUse", "PostToolUse", "Stop", "UserPromptSubmit"}
	for _, hookType := range requiredHooks {
		hookEntries, exists := hooks[hookType].([]any)
		if !exists || !hasCccHookCommand(hookEntries) {
			HookLog("verify-codex-hooks: missing %s for %s", hookType, projectPath)
			return false
		}
	}

	HookLog("verify-codex-hooks: hooks present for %s", projectPath)
	return true
}

// InstallCodexHooksToPath installs ccc hooks to a Codex hooks.json file.
func InstallCodexHooksToPath(hooksPath string) error {
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0755); err != nil {
		return fmt.Errorf("failed to create .codex directory: %w", err)
	}

	var settings map[string]any
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		settings = make(map[string]any)
	} else if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse hooks: %w", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
	}

	allHookTypes := []string{"Stop", "PostToolUse", "PreToolUse", "UserPromptSubmit"}
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

	cccPath := cccHookCommandPrefix()
	cccHooks := map[string][]any{
		"PreToolUse": {
			map[string]any{
				"matcher": "*",
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": cccPath + " hook-permission",
						"timeout": 300000,
					},
				},
			},
		},
		"PostToolUse": {
			map[string]any{
				"matcher": "*",
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": cccPath + " hook-post-tool",
					},
				},
			},
		},
		"Stop": {
			map[string]any{
				"matcher": "*",
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": cccPath + " hook-stop",
					},
				},
			},
		},
		"UserPromptSubmit": {
			map[string]any{
				"matcher": "*",
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": cccPath + " hook-user-prompt",
					},
				},
			},
		},
	}

	for hookType, newHooks := range cccHooks {
		var existingHooks []any
		if existing, ok := hooks[hookType].([]any); ok {
			existingHooks = existing
		}
		hooks[hookType] = append(newHooks, existingHooks...)
	}

	settings["hooks"] = hooks

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal hooks: %w", err)
	}

	if err := os.WriteFile(hooksPath, newData, 0600); err != nil {
		return fmt.Errorf("failed to write hooks: %w", err)
	}

	return nil
}

type codexHookTrustSpec struct {
	EventName string
	KeyLabel  string
	Matcher   string
	Command   string
	Timeout   int
}

// TrustCodexHooksForProject pre-approves the exact Codex hooks CCC installs.
// Codex keeps hook trust in config.toml, keyed by project hook file path and a
// hash of the normalized hook identity.
func TrustCodexHooksForProject(cfg *config.Config, providerName string, hooksPath string) error {
	absHooksPath, err := filepath.Abs(hooksPath)
	if err != nil {
		return err
	}
	configPath := codexConfigTomlPath(cfg, providerName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	cccPath := cccHookCommandPrefix()
	specs := []codexHookTrustSpec{
		{EventName: "pre_tool_use", KeyLabel: "pre_tool_use", Matcher: "*", Command: cccPath + " hook-permission", Timeout: 300000},
		{EventName: "post_tool_use", KeyLabel: "post_tool_use", Matcher: "*", Command: cccPath + " hook-post-tool", Timeout: 600},
		{EventName: "stop", KeyLabel: "stop", Command: cccPath + " hook-stop", Timeout: 600},
		{EventName: "user_prompt_submit", KeyLabel: "user_prompt_submit", Command: cccPath + " hook-user-prompt", Timeout: 600},
	}
	states := make(map[string]string, len(specs))
	for _, spec := range specs {
		key := fmt.Sprintf("%s:%s:0:0", absHooksPath, spec.KeyLabel)
		states[key] = codexCommandHookHash(spec)
	}

	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated := upsertCodexHookTrustStates(string(data), states)
	if err := os.WriteFile(configPath, []byte(updated), 0600); err != nil {
		return err
	}
	HookLog("trust-codex-hooks: trusted %d hooks in %s", len(states), configPath)
	return nil
}

func codexConfigTomlPath(cfg *config.Config, providerName string) string {
	if cfg != nil && providerName != "" && cfg.Providers != nil {
		if p := cfg.Providers[providerName]; p != nil && strings.EqualFold(p.Backend, "codex") && p.ConfigDir != "" {
			return filepath.Join(config.ExpandPath(p.ConfigDir), "config.toml")
		}
	}
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		return filepath.Join(config.ExpandPath(codexHome), "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "config.toml")
}

func codexCommandHookHash(spec codexHookTrustSpec) string {
	hook := map[string]any{
		"async":   false,
		"command": spec.Command,
		"timeout": spec.Timeout,
		"type":    "command",
	}
	identity := map[string]any{
		"event_name": spec.EventName,
		"hooks":      []any{hook},
	}
	if spec.Matcher != "" {
		identity["matcher"] = spec.Matcher
	}
	serialized, _ := json.Marshal(canonicalJSON(identity))
	sum := sha256.Sum256(serialized)
	return fmt.Sprintf("sha256:%x", sum)
}

func canonicalJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		ordered := make([]any, 0, len(keys)*2)
		for _, key := range keys {
			ordered = append(ordered, key, canonicalJSON(v[key]))
		}
		return canonicalObject(ordered)
	case []any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, canonicalJSON(item))
		}
		return items
	default:
		return v
	}
}

type canonicalObject []any

func (o canonicalObject) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i < len(o); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(o[i])
		val, err := json.Marshal(o[i+1])
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(val)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func upsertCodexHookTrustStates(toml string, states map[string]string) string {
	lines := strings.Split(toml, "\n")
	var kept []string
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if key, ok := parseCodexHookStateHeader(line); ok {
			if _, replace := states[key]; replace {
				for i+1 < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "[") {
					i++
				}
				continue
			}
		}
		kept = append(kept, lines[i])
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if !containsTrimmedLine(kept, "[hooks.state]") {
		if len(kept) > 0 {
			kept = append(kept, "")
		}
		kept = append(kept, "[hooks.state]")
	}
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		kept = append(kept, "", fmt.Sprintf("[hooks.state.%q]", key), "enabled = true", fmt.Sprintf("trusted_hash = %q", states[key]))
	}
	return strings.Join(kept, "\n") + "\n"
}

func parseCodexHookStateHeader(line string) (string, bool) {
	const prefix = "[hooks.state.\""
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "\"]") {
		return "", false
	}
	key := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "\"]")
	key = strings.ReplaceAll(key, "\\\\", "\\")
	key = strings.ReplaceAll(key, "\\\"", "\"")
	return key, true
}

func containsTrimmedLine(lines []string, want string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
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

// EnsureCodexHooksForSession ensures ccc Codex hooks are installed in the session's project directory.
func EnsureCodexHooksForSession(cfg *EnsureHooksForSessionConfig) error {
	if cfg.SessionInfo == nil {
		if cfg.Config == nil || cfg.Config.Sessions == nil {
			return nil
		}
		cfg.SessionInfo = cfg.Config.Sessions[cfg.SessionName]
		if cfg.SessionInfo == nil {
			return nil
		}
	}

	projectPath := cfg.GetSessionWorkDir(cfg.Config, cfg.SessionName, cfg.SessionInfo)
	if projectPath == "" {
		return fmt.Errorf("unable to determine project path for session '%s'", cfg.SessionName)
	}
	providerName := cfg.SessionInfo.ProviderName
	if providerName == "" && cfg.Config != nil {
		providerName = cfg.Config.ActiveProvider
	}
	hooksPath := filepath.Join(projectPath, ".codex", "hooks.json")

	if VerifyCodexHooksForProject(projectPath) {
		if err := TrustCodexHooksForProject(cfg.Config, providerName, hooksPath); err != nil {
			return fmt.Errorf("failed to trust Codex hooks for project %s: %w", projectPath, err)
		}
		HookLog("ensure-codex-hooks: hooks already present for %s", projectPath)
		return nil
	}

	HookLog("ensure-codex-hooks: installing hooks to %s", projectPath)
	if err := InstallCodexHooksForProject(projectPath); err != nil {
		return fmt.Errorf("failed to install Codex hooks for project %s: %w", projectPath, err)
	}
	if err := TrustCodexHooksForProject(cfg.Config, providerName, hooksPath); err != nil {
		return fmt.Errorf("failed to trust Codex hooks for project %s: %w", projectPath, err)
	}

	HookLog("ensure-codex-hooks: hooks installed successfully for %s", projectPath)
	return nil
}
