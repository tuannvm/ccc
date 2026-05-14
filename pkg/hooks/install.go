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
	EventName        string
	InstalledMatcher string
	EffectiveMatcher string
	Command          string
	Timeout          int
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

	states, err := codexHookTrustStatesFromFile(absHooksPath)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return fmt.Errorf("no ccc Codex hooks found in %s", absHooksPath)
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

func codexHookTrustStatesFromFile(hooksPath string) (map[string]string, error) {
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		return nil, err
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse hooks: %w", err)
	}
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil, nil
	}

	states := make(map[string]string)
	for hookType, entriesRaw := range hooksMap {
		eventName := codexEventNameForHookType(hookType)
		if eventName == "" {
			continue
		}
		entries, ok := entriesRaw.([]any)
		if !ok {
			continue
		}
		for entryIndex, entryRaw := range entries {
			entry, ok := entryRaw.(map[string]any)
			if !ok {
				continue
			}
			matcher, _ := entry["matcher"].(string)
			nested, ok := entry["hooks"].([]any)
			if !ok {
				continue
			}
			for hookIndex, hookRaw := range nested {
				hook, ok := hookRaw.(map[string]any)
				if !ok {
					continue
				}
				spec, ok := installedCodexHookTrustSpec(eventName, matcher, hook)
				if !ok {
					continue
				}
				key := fmt.Sprintf("%s:%s:%d:%d", hooksPath, eventName, entryIndex, hookIndex)
				states[key] = codexCommandHookHash(spec)
			}
		}
	}
	return states, nil
}

func installedCodexHookTrustSpec(eventName, matcher string, hook map[string]any) (codexHookTrustSpec, bool) {
	if hookType, _ := hook["type"].(string); hookType != "command" {
		return codexHookTrustSpec{}, false
	}
	command, _ := hook["command"].(string)
	timeout := codexHookTimeout(hook)
	cccPath := cccHookCommandPrefix()

	specs := map[string]codexHookTrustSpec{
		"pre_tool_use": {
			EventName:        "pre_tool_use",
			InstalledMatcher: "*",
			EffectiveMatcher: "*",
			Command:          cccPath + " hook-permission",
			Timeout:          300000,
		},
		"post_tool_use": {
			EventName:        "post_tool_use",
			InstalledMatcher: "*",
			EffectiveMatcher: "*",
			Command:          cccPath + " hook-post-tool",
			Timeout:          600,
		},
		"stop": {
			EventName:        "stop",
			InstalledMatcher: "*",
			Command:          cccPath + " hook-stop",
			Timeout:          600,
		},
		"user_prompt_submit": {
			EventName:        "user_prompt_submit",
			InstalledMatcher: "*",
			Command:          cccPath + " hook-user-prompt",
			Timeout:          600,
		},
	}

	spec, ok := specs[eventName]
	if !ok {
		return codexHookTrustSpec{}, false
	}
	if matcher != spec.InstalledMatcher || command != spec.Command || timeout != spec.Timeout {
		return codexHookTrustSpec{}, false
	}
	return spec, true
}

func codexEventNameForHookType(hookType string) string {
	switch hookType {
	case "PreToolUse":
		return "pre_tool_use"
	case "PostToolUse":
		return "post_tool_use"
	case "Stop":
		return "stop"
	case "UserPromptSubmit":
		return "user_prompt_submit"
	default:
		return ""
	}
}

func codexHookTimeout(hook map[string]any) int {
	switch timeout := hook["timeout"].(type) {
	case float64:
		return int(timeout)
	case int:
		return timeout
	case json.Number:
		if v, err := timeout.Int64(); err == nil {
			return int(v)
		}
	}
	return 600
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
	if spec.EffectiveMatcher != "" {
		identity["matcher"] = spec.EffectiveMatcher
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
		kept = append(kept, "", fmt.Sprintf("[hooks.state.%q]", key), fmt.Sprintf("trusted_hash = %q", states[key]))
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

// InstallSkill installs the ccc skills into the current project only.
//
// Deprecated: project installs should use the skills marketplace, for example
// `npx skills add`, so the agent runtime owns project-scoped placement.
func InstallSkill() error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine current project directory: %w", err)
	}
	if err := installProjectSkillToDir(projectDir); err != nil {
		return err
	}
	fmt.Printf("✅ CCC skills installed to %s\n", projectDir)
	return nil
}

// InstallGlobalSkill installs the ccc skills into the user's global agent skill directories.
func InstallGlobalSkill() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	claudeSkillDir := filepath.Join(homeDir, ".claude", "skills")
	if err := installClaudeSkillToDir(claudeSkillDir); err != nil {
		return err
	}

	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(homeDir, ".codex")
	}
	codexSkillRoot := filepath.Join(codexHome, "skills")
	if err := installAgentSkillToRoot(codexSkillRoot, "ccc", codexCccSkillContent()); err != nil {
		return err
	}
	if err := installAgentSkillToRoot(codexSkillRoot, "ccc-send", codexSendSkillContent()); err != nil {
		return err
	}

	fmt.Printf("✅ CCC global skills installed to %s and %s\n", claudeSkillDir, codexSkillRoot)
	return nil
}

func installProjectSkillToDir(projectDir string) error {
	if err := installClaudeSkillToDir(filepath.Join(projectDir, ".claude", "skills")); err != nil {
		return err
	}
	if err := installAgentSkillToRoot(filepath.Join(projectDir, ".agents", "skills"), "ccc", codexCccSkillContent()); err != nil {
		return err
	}
	if err := installAgentSkillToRoot(filepath.Join(projectDir, ".agents", "skills"), "ccc-send", codexSendSkillContent()); err != nil {
		return err
	}
	return nil
}

func installClaudeSkillToDir(claudeSkillDir string) error {
	sendSkillPath := filepath.Join(claudeSkillDir, "ccc-send.md")
	cccSkillPath := filepath.Join(claudeSkillDir, "ccc.md")

	if err := os.MkdirAll(claudeSkillDir, 0755); err != nil {
		return fmt.Errorf("failed to create Claude skills directory: %w", err)
	}

	skillContent := cccSendSkillBody()
	if err := os.WriteFile(sendSkillPath, []byte(skillContent), 0644); err != nil {
		return fmt.Errorf("failed to write send skill file: %w", err)
	}

	cccSkillContent := cccSkillBody()
	if err := os.WriteFile(cccSkillPath, []byte(cccSkillContent), 0644); err != nil {
		return fmt.Errorf("failed to write ccc skill file: %w", err)
	}
	return nil
}

// UninstallSkill removes the ccc skills
func UninstallSkill() error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine current project directory: %w", err)
	}
	os.Remove(filepath.Join(projectDir, ".claude", "skills", "ccc-send.md"))
	os.Remove(filepath.Join(projectDir, ".claude", "skills", "ccc.md"))
	for _, name := range []string{"ccc", "ccc-send"} {
		os.RemoveAll(filepath.Join(projectDir, ".agents", "skills", name))
	}
	return nil
}

func installProjectAgentSkill(projectDir, name, content string) error {
	return installAgentSkillToRoot(filepath.Join(projectDir, ".agents", "skills"), name, content)
}

func installAgentSkillToRoot(rootDir, name, content string) error {
	dir := filepath.Join(rootDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create agent skill directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write agent skill %s: %w", name, err)
	}
	return nil
}

func codexCccSkillContent() string {
	return "---\nname: ccc\ndescription: Sync, link, continue, hand off, or take over the current Claude Code or Codex-backed session from Telegram by running ccc sync.\nallowed-tools: Bash\n---\n\n" + cccSkillBody()
}

func codexSendSkillContent() string {
	return "---\nname: ccc-send\ndescription: Send generated or built files to the user via Telegram using ccc send.\nallowed-tools: Bash\n---\n\n" + cccSendSkillBody()
}

func cccSendSkillBody() string {
	return "# CCC Send - File Transfer Skill\n\n" +
		"## Description\n" +
		"Send files to the user via Telegram using the ccc send command.\n\n" +
		"## Usage\n" +
		"When the user asks you to send them a file, or when you have generated/built a file that the user needs, run:\n\n" +
		"~~~bash\nccc send <file_path>\n~~~\n\n" +
		"## How it works\n" +
		"- Small files under 50MB are sent directly via Telegram.\n" +
		"- Large files are streamed via relay server with a one-time download link.\n\n" +
		"## Examples\n\n" +
		"~~~bash\nccc send ./build/app.apk\nccc send ./output/report.pdf\nccc send ~/Downloads/large-file.zip\n~~~\n\n" +
		"## Important Notes\n" +
		"- The command detects the current session from your working directory.\n" +
		"- For large files, the command will wait up to 10 minutes for the user to download.\n" +
		"- Each download link is one-time use only.\n" +
		"- Use this proactively when you have created files the user needs.\n"
}

func cccSkillBody() string {
	return "# CCC - Telegram Session Sync\n\n" +
		"## Description\n" +
		"Use CCC when the user asks to sync, link, continue, or take over the current Claude Code or Codex-backed session from Telegram. CCC creates or reuses the Telegram topic for the current project, installs the project hooks, and keeps the current agent session running.\n\n" +
		"## Usage\n" +
		"When this skill is triggered from Codex, run:\n\n" +
		"~~~bash\nCCC_AGENT_PROVIDER=codex ccc sync\n~~~\n\n" +
		"When this skill is triggered from Claude Code, run:\n\n" +
		"~~~bash\nCCC_AGENT_PROVIDER=anthropic ccc sync\n~~~\n\n" +
		"If you know the exact current provider name, use it instead of the defaults above. If the runtime exposes a session or thread id, pass it as CCC_AGENT_SESSION_ID. For example:\n\n" +
		"~~~bash\nCCC_AGENT_PROVIDER=codex CCC_AGENT_SESSION_ID=$CODEX_THREAD_ID ccc sync\n~~~\n\n" +
		"If the runtime is unknown, run:\n\n" +
		"~~~bash\nccc sync\n~~~\n\n" +
		"If the user gave a short handoff note that should appear in Telegram, include it:\n\n" +
		"~~~bash\nCCC_AGENT_PROVIDER=codex ccc sync \"Continuing this session from Telegram.\"\n~~~\n\n" +
		"## How it works\n" +
		"- If the current directory already maps to a CCC session, CCC reuses that Telegram topic.\n" +
		"- If no session maps to the current directory, CCC creates a new Telegram topic using the normal provider/session flow.\n" +
		"- CCC_AGENT_PROVIDER tells CCC whether this running session is Codex or Claude so Telegram resumes the same agent backend instead of using the configured default.\n" +
		"- CCC_AGENT_SESSION_ID lets CCC store an exact resume target when the runtime exposes one. Codex may not expose this to shell commands; in that case CCC uses Codex resume-last behavior for that backend.\n" +
		"- CCC installs or refreshes project-local hooks so later prompts, tool updates, permission requests, and completion messages are routed to the topic.\n" +
		"- CCC does not attach tmux or restart the agent; the current character keeps control after the sync command returns.\n\n" +
		"## Important Notes\n" +
		"- Run the command from the project directory the current agent session is working in.\n" +
		"- Do not run plain ccc for this skill; that command attaches tmux and is only for starting CCC at the beginning of a terminal session.\n" +
		"- After ccc sync succeeds, continue the user request normally.\n"
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
