package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// shellQuote safely quotes a string for shell command arguments
func shellQuote(s string) string {
	// Replace single quotes with '\'' and wrap in single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// applyProviderEnv applies provider-specific environment variables to cmd.Env
// Returns the modified environment slice
// This version uses the Provider interface for provider-agnostic design
func applyProviderEnv(baseEnv []string, provider Provider, config *Config) []string {
	if provider == nil {
		return baseEnv
	}

	env := baseEnv

	// Get provider variables and track which ones we'll actually set
	providerVars := provider.EnvVars(config)

	// For ConfiguredProvider with auth, we need to check if auth_env_var expands to non-empty
	// If it expands to empty, we should preserve ambient credentials instead
	shouldUnsetAuth := false
	if !provider.IsBuiltin() {
		for _, v := range providerVars {
			if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=$") {
				// This is auth_env_var - check if it expands to non-empty
				envVarName := strings.TrimPrefix(v, "ANTHROPIC_AUTH_TOKEN=$")
				if envVal := os.Getenv(envVarName); envVal != "" {
					shouldUnsetAuth = true
				}
				// If env var is empty, we DON'T unset existing auth - preserve ambient
				break
			} else if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=") && !strings.Contains(v, "$") {
				// Direct token value (no $ prefix)
				shouldUnsetAuth = true
				break
			}
		}
	}

	// Unset auth vars only if we're actually replacing them
	if shouldUnsetAuth {
		env = unsetEnvVars(env, []string{
			"ANTHROPIC_API_KEY",
			"CLAUDE_API_KEY",
			"ANTHROPIC_AUTH_TOKEN",
		})
		// Also unset model vars when using a configured provider with auth
		env = unsetEnvVars(env, []string{
			"ANTHROPIC_BASE_URL",
			"ANTHROPIC_MODEL",
			"ANTHROPIC_DEFAULT_OPUS_MODEL",
			"ANTHROPIC_DEFAULT_SONNET_MODEL",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL",
			"CLAUDE_CODE_SUBAGENT_MODEL",
		})
	}

	// Add provider-specific environment variables
	// For ConfiguredProvider, this includes expanded auth_env_var values and api_timeout
	for _, v := range providerVars {
		// Expand $VAR references for ConfiguredProvider auth_env_var
		if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=$") {
			envVarName := strings.TrimPrefix(v, "ANTHROPIC_AUTH_TOKEN=$")
			if envVal := os.Getenv(envVarName); envVal != "" {
				env = append(env, "ANTHROPIC_AUTH_TOKEN="+envVal)
			}
			// If env var is empty, we skip it (preserving ambient credentials)
		} else {
			env = append(env, v)
		}
	}

	// Common settings for all providers
	// Note: TMPDIR is set for all providers including builtin, as before
	env = append(env, []string{
		"TMPDIR=/tmp/claude",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DISABLE_COST_WARNINGS=1",
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	}...)

	return env
}

// unsetEnvVars removes specified environment variables from env slice
func unsetEnvVars(env []string, keys []string) []string {
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	var result []string
	for _, e := range env {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			result = append(result, e)
			continue
		}
		key := e[:idx]
		if !keyMap[key] {
			result = append(result, e)
		}
	}
	return result
}

// runClaudeRaw runs claude directly (used inside tmux sessions)
// providerOverride, if non-empty, specifies which provider to use instead of active_provider
// resumeSessionID, if non-empty, resumes a specific session by ID
// worktreeName, if non-empty, creates/uses a git worktree session
func runClaudeRaw(continueSession bool, resumeSessionID string, providerOverride string, worktreeName string) error {
	if claudePath == "" {
		return fmt.Errorf("claude binary not found")
	}

	// Clean stale Telegram flag from previous sessions.
	// Use window_name to identify the session
	if winName, err := exec.Command(tmuxPath, "display-message", "-p", "#{window_name}").Output(); err == nil {
		name := strings.TrimSpace(string(winName))
		if name != "" {
			os.Remove(telegramActiveFlag(name))
		}
	}

	var args []string
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	} else if continueSession {
		args = append(args, "-c")
	}
	// worktreeName is the special value WorktreeAutoGenerate for auto-generation, or a specific name
	// Claude accepts --worktree [name] where name is optional
	if worktreeName != "" {
		if worktreeName == WorktreeAutoGenerate {
			// Auto-generate: pass --worktree without a value
			args = append(args, "--worktree")
		} else {
			args = append(args, "--worktree", worktreeName)
		}
	}

	// Build the claude command with all args
	// Execute claude directly to ensure provider env vars are not overridden by shell rc files
	cmd := exec.Command(claudePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start with current environment
	cmd.Env = os.Environ()

	// Log the command for debugging
	cwd, _ := os.Getwd()
	configDirCode := os.Getenv("CLAUDE_CODE_CONFIG_DIR")
	configDirZai := os.Getenv("CLAUDE_CONFIG_DIR")
	homeDir := os.Getenv("HOME")
	listenLog("runClaudeRaw: claude=%s args=%v cwd=%s config_code_dir=%q config_dir=%q home=%q", claudePath, args, cwd, configDirCode, configDirZai, homeDir)

	// Load config and apply provider settings
	config, err := loadConfig()
	if err == nil {
		// CRITICAL: Ensure hooks are installed in the current project directory
		// Hooks are essential for ccc functionality (Telegram, OTP, tool tracking)
		// We use cwd (current working directory) as the project path
		sessionName := filepath.Base(cwd)
		sessionInfo := &SessionInfo{Path: cwd}
		if config.Sessions != nil {
			if existing := config.Sessions[sessionName]; existing != nil {
				sessionInfo = existing
			}
		}
		if err := ensureHooksForSession(config, sessionName, sessionInfo); err != nil {
			listenLog("Warning: Failed to ensure hooks in %s: %v", cwd, err)
		}

		// Determine which provider to use using the Provider interface
		// getProvider returns nil only for unknown providers
		provider := getProvider(config, providerOverride)

		// Validate provider - getProvider returns nil for unknown providers
		if providerOverride != "" && provider == nil {
			return fmt.Errorf("unknown provider: %s (available providers: %v)", providerOverride, getProviderNames(config))
		}

		// Apply provider env in the following cases:
		// 1. When NOT resuming (resumeSessionID == "") - start new session with provider env
		// 2. When resuming WITH explicit provider override - user specified which provider to use
		// Skip provider env only when resuming WITHOUT explicit override (preserve original session env)
		shouldApplyProviderEnv := (resumeSessionID == "") || (providerOverride != "")

		// Ensure provider settings have trusted directories configured
		// This prevents "Do you trust the files in this folder?" prompts
		// Works with both BuiltinProvider and ConfiguredProvider
		if err := ensureProviderSettings(provider); err != nil {
			listenLog("Failed to update provider settings: %v", err)
		}

		if shouldApplyProviderEnv {
			cmd.Env = applyProviderEnv(cmd.Env, provider, config)
			listenLog("Applying provider env: providerOverride=%q resumeSessionID=%q provider=%q", providerOverride, resumeSessionID, provider.Name())
		} else {
			listenLog("Preserving original session environment for resumeSessionID=%q", resumeSessionID)
		}

		// Ensure OAuth token is available from config if not already in environment
		if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" && config.OAuthToken != "" {
			cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+config.OAuthToken)
		}
	}

	return cmd.Run()
}
