package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const defaultTmuxSession = "ccc"

var (
	tmuxPath   string
	cccPath    string
	claudePath string
)

func initPaths() {
	// Find tmux binary
	if path, err := exec.LookPath("tmux"); err == nil {
		tmuxPath = path
	} else {
		// Fallback paths for common installations
		for _, p := range []string{"/opt/homebrew/bin/tmux", "/usr/local/bin/tmux", "/usr/bin/tmux"} {
			if _, err := os.Stat(p); err == nil {
				tmuxPath = p
				break
			}
		}
	}

	// Find ccc binary - prefer ~/bin/ccc (canonical install path),
	// then PATH, then current executable as last resort
	home, _ := os.UserHomeDir()
	binCcc := home + "/bin/ccc"
	if _, err := os.Stat(binCcc); err == nil {
		cccPath = binCcc
	} else if path, err := exec.LookPath("ccc"); err == nil {
		cccPath = path
	} else if exe, err := os.Executable(); err == nil {
		cccPath = exe
	}

	// Find claude binary - first try PATH, then fallback paths
	if path, err := exec.LookPath("claude"); err == nil {
		claudePath = path
	} else {
		home, _ := os.UserHomeDir()
		claudePaths := []string{
			home + "/.local/bin/claude",
			"/usr/local/bin/claude",
		}
		for _, p := range claudePaths {
			if _, err := os.Stat(p); err == nil {
				claudePath = p
				break
			}
		}
	}
}

// getTargetSession returns an existing tmux session name, or creates one if none exist
func getTargetSession() (string, error) {
	// Try to find any existing session
	cmd := exec.Command(tmuxPath, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			name := scanner.Text()
			if name != "" {
				return name, nil
			}
		}
	}
	// No sessions exist, create one
	c := exec.Command(tmuxPath, "new-session", "-d", "-s", defaultTmuxSession)
	if err := c.Run(); err != nil {
		return "", err
	}
	exec.Command(tmuxPath, "set-option", "-t", defaultTmuxSession, "mouse", "on").Run()
	return defaultTmuxSession, nil
}

// tmuxTargetByID returns the window ID if available, otherwise falls back to name lookup
func tmuxTargetByID(windowID string, windowName string) string {
	if windowID != "" {
		return windowID
	}
	return tmuxTargetByName(windowName)
}

// tmuxTargetByName finds a window target by name (fallback)
func tmuxTargetByName(windowName string) string {
	cmd := exec.Command(tmuxPath, "list-windows", "-a", "-F", "#{window_id}\t#{window_name}")
	out, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), "\t", 2)
			if len(parts) == 2 && parts[1] == windowName {
				return parts[0] // return window ID
			}
		}
	}
	return defaultTmuxSession + ":" + windowName
}

func tmuxWindowExistsByID(windowID string, windowName string) bool {
	if windowID != "" {
		// Check by ID directly
		cmd := exec.Command(tmuxPath, "list-windows", "-a", "-F", "#{window_id}")
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			if scanner.Text() == windowID {
				return true
			}
		}
		return false
	}
	// Fallback: search by name
	cmd := exec.Command(tmuxPath, "list-windows", "-a", "-F", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if scanner.Text() == windowName {
			return true
		}
	}
	return false
}

func createTmuxWindow(windowName string, workDir string, continueSession bool, providerName string) (string, error) {
	// Build the command to run inside the window
	cccCmd := cccPath + " run"
	if continueSession {
		cccCmd += " -c"
	}
	if providerName != "" {
		cccCmd += " --provider " + providerName
	}

	// Get an existing session or create one
	sess, err := getTargetSession()
	if err != nil {
		return "", err
	}

	// Create new window, -P -F prints the window ID
	args := []string{"new-window", "-P", "-F", "#{window_id}", "-t", sess + ":", "-n", windowName, "-c", workDir}
	cmd := exec.Command(tmuxPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	windowID := strings.TrimSpace(string(out))

	// Send the command to the window via send-keys using window ID
	time.Sleep(200 * time.Millisecond)
	exec.Command(tmuxPath, "send-keys", "-t", windowID, cccCmd, "C-m").Run()

	return windowID, nil
}

// applyProviderEnv applies provider-specific environment variables to cmd.Env
// Returns the modified environment slice
func applyProviderEnv(baseEnv []string, provider *ProviderConfig) []string {
	if provider == nil {
		return baseEnv
	}

	home, _ := os.UserHomeDir()

	// Build environment with provider settings
	// First, unset any Anthropic-related vars to avoid conflicts
	env := baseEnv
	env = unsetEnvVars(env, []string{
		"ANTHROPIC_API_KEY",
		"CLAUDE_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"CLAUDE_CODE_SUBAGENT_MODEL",
	})

	// Determine auth token (auto-load from env if empty)
	authToken := provider.AuthToken
	if authToken == "" && provider.AuthEnvVar != "" {
		authToken = os.Getenv(provider.AuthEnvVar)
	}
	// If still no token, preserve existing Anthropic credentials from environment
	if authToken == "" {
		if existing := os.Getenv("ANTHROPIC_API_KEY"); existing != "" {
			authToken = existing
		} else if existing := os.Getenv("ANTHROPIC_AUTH_TOKEN"); existing != "" {
			authToken = existing
		} else if existing := os.Getenv("CLAUDE_API_KEY"); existing != "" {
			authToken = existing
		}
	}
	if authToken != "" {
		env = append(env, "ANTHROPIC_AUTH_TOKEN="+authToken)
	}

	// Base URL
	if provider.BaseURL != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+provider.BaseURL)
	}

	// Models
	if provider.OpusModel != "" {
		env = append(env, "ANTHROPIC_DEFAULT_OPUS_MODEL="+provider.OpusModel)
		env = append(env, "ANTHROPIC_MODEL="+provider.OpusModel)
	}
	if provider.SonnetModel != "" {
		env = append(env, "ANTHROPIC_DEFAULT_SONNET_MODEL="+provider.SonnetModel)
	}
	if provider.HaikuModel != "" {
		env = append(env, "ANTHROPIC_DEFAULT_HAIKU_MODEL="+provider.HaikuModel)
	}
	if provider.SubagentModel != "" {
		env = append(env, "CLAUDE_CODE_SUBAGENT_MODEL="+provider.SubagentModel)
	}

	// Config dir with ~ expansion
	if provider.ConfigDir != "" {
		configDir := provider.ConfigDir
		if strings.HasPrefix(configDir, "~/") {
			configDir = home + configDir[1:]
		} else if configDir == "~" {
			configDir = home
		}
		env = append(env, "CLAUDE_CONFIG_DIR="+configDir)
	}

	// Common settings for all providers (use values from config)
	if provider.ApiTimeout > 0 {
		env = append(env, fmt.Sprintf("API_TIMEOUT_MS=%d", provider.ApiTimeout))
	}
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
func runClaudeRaw(continueSession bool, providerOverride string) error {
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
	if continueSession {
		args = append(args, "-c")
	}

	cmd := exec.Command(claudePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start with current environment
	cmd.Env = os.Environ()

	// Load config and apply provider settings
	config, err := loadConfig()
	if err == nil {
		// Determine which provider to use
		var provider *ProviderConfig
		if providerOverride != "" && config.Providers != nil {
			// Use explicitly specified provider
			provider = config.Providers[providerOverride]
		}
		if provider == nil {
			// Fall back to active provider
			provider = getActiveProvider(config)
		}
		cmd.Env = applyProviderEnv(cmd.Env, provider)

		// Ensure OAuth token is available from config if not already in environment
		if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" && config.OAuthToken != "" {
			cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+config.OAuthToken)
		}
	}

	return cmd.Run()
}

// waitForClaude polls the tmux pane until Claude Code's input prompt appears
func waitForClaude(target string, timeout time.Duration) error {
	// Poll faster for short timeouts (message sending), slower for startup
	interval := 100 * time.Millisecond
	if timeout > 10*time.Second {
		interval = 500 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command(tmuxPath, "capture-pane", "-t", target, "-p")
		out, err := cmd.Output()
		if err == nil {
			content := string(out)
			// Claude Code shows "❯" when ready for input
			if strings.Contains(content, "❯") {
				return nil
			}
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("timeout waiting for Claude to start")
}

// sendToTmuxFromTelegram sets the Telegram active flag before sending,
// so the permission hook knows this input came from Telegram and requires OTP.
// windowNameFromTarget extracts the window name from a "session:window" target
func windowNameFromTarget(target string) string {
	if idx := strings.LastIndex(target, ":"); idx >= 0 {
		return target[idx+1:]
	}
	return target
}

func sendToTmuxFromTelegram(target string, windowName string, text string) error {
	os.WriteFile(telegramActiveFlag(windowName), []byte("1"), 0600)
	return sendToTmux(target, text)
}

func sendToTmuxFromTelegramWithDelay(target string, windowName string, text string, delay time.Duration) error {
	os.WriteFile(telegramActiveFlag(windowName), []byte("1"), 0600)
	return sendToTmuxWithDelay(target, text, delay)
}

func sendToTmux(target string, text string) error {
	// Calculate delay based on text length
	// Base: 50ms + 0.5ms per character, capped at 5 seconds
	baseDelay := 50 * time.Millisecond
	charDelay := time.Duration(len(text)) * 500 * time.Microsecond // 0.5ms per char
	delay := baseDelay + charDelay
	if delay > 5*time.Second {
		delay = 5 * time.Second
	}
	return sendToTmuxWithDelay(target, text, delay)
}

func sendToTmuxWithDelay(target string, text string, delay time.Duration) error {
	// Send text literally
	cmd := exec.Command(tmuxPath, "send-keys", "-t", target, "-l", text)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Brief pause for TUI to process pasted text
	time.Sleep(100 * time.Millisecond)

	// Send Enter twice (Claude Code needs double Enter)
	exec.Command(tmuxPath, "send-keys", "-t", target, "C-m").Run()
	time.Sleep(50 * time.Millisecond)
	exec.Command(tmuxPath, "send-keys", "-t", target, "C-m").Run()

	return nil
}

func killTmuxWindow(windowID string, windowName string) error {
	target := tmuxTargetByID(windowID, windowName)
	cmd := exec.Command(tmuxPath, "kill-window", "-t", target)
	return cmd.Run()
}

func listTmuxWindows() ([]string, error) {
	cmd := exec.Command(tmuxPath, "list-windows", "-a", "-F", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var windows []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		name := scanner.Text()
		windows = append(windows, name)
	}
	return windows, nil
}

// killTmuxSession kills an entire tmux session (used for temporary sessions like auth)
func killTmuxSession(name string) error {
	cmd := exec.Command(tmuxPath, "kill-session", "-t", name)
	return cmd.Run()
}
