package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// tmuxSafeName converts a session name to a tmux-safe window name
// (dots are interpreted as window/pane separators in tmux)
func tmuxSafeName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}

// getWindowID safely looks up the tmux WindowID from config for a session name.
// Returns empty string if the session or WindowID is not set.
func getWindowID(config *Config, sessionName string) string {
	if config == nil || config.Sessions == nil {
		return ""
	}
	info, exists := config.Sessions[sessionName]
	if !exists || info == nil {
		return ""
	}
	return info.WindowID
}

func createSession(config *Config, name string) error {
	// Check if session already exists
	if _, exists := config.Sessions[name]; exists {
		return fmt.Errorf("session '%s' already exists", name)
	}

	// Get the provider to use for this session
	provider := getActiveProvider(config)
	providerName := ""
	if provider != nil && config.ActiveProvider != "" {
		providerName = config.ActiveProvider
	}

	// Create Telegram topic
	topicID, err := createForumTopic(config, name, providerName)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	// Create tmux window
	workDir := resolveProjectPath(config, name)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		// Create project directory
		os.MkdirAll(workDir, 0755)
	}

	windowID, err := createTmuxWindow(tmuxSafeName(name), workDir, false, providerName, "")
	if err != nil {
		return fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Save mapping with full path
	config.Sessions[name] = &SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		WindowID:     windowID,
		ProviderName: providerName,
	}
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

func killSession(config *Config, name string) error {
	if _, exists := config.Sessions[name]; !exists {
		return fmt.Errorf("session '%s' not found", name)
	}

	// Kill tmux window
	killTmuxWindow(getWindowID(config, name), tmuxSafeName(name))

	// Remove from config
	delete(config.Sessions, name)
	saveConfig(config)

	return nil
}

func getSessionByTopic(config *Config, topicID int64) string {
	for name, info := range config.Sessions {
		if info != nil && info.TopicID == topicID {
			return name
		}
	}
	return ""
}

// startSession creates/attaches to a tmux window with Telegram topic
func startSession(continueSession bool) error {
	// Get current directory name as session name
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	name := filepath.Base(cwd)
	winName := tmuxSafeName(name)

	// Load config to check/create topic
	config, err := loadConfig()
	if err != nil {
		// No config, just run claude directly with default provider
		return runClaudeRaw(continueSession, "", "")
	}

	// Get the provider to use for this session
	providerName := ""
	if config.Sessions[name] != nil && config.Sessions[name].ProviderName != "" {
		// Use the session's saved provider
		providerName = config.Sessions[name].ProviderName
	} else if config.ActiveProvider != "" {
		// Use the active provider
		providerName = config.ActiveProvider
	}

	// Create topic if it doesn't exist and we have a group configured
	if config.GroupID != 0 {
		if _, exists := config.Sessions[name]; !exists {
			topicID, err := createForumTopic(config, name, providerName)
			if err == nil {
				config.Sessions[name] = &SessionInfo{
					TopicID:      topicID,
					Path:         cwd,
					ProviderName: providerName,
				}
				saveConfig(config)
				fmt.Printf("📱 Created Telegram topic: %s\n", name)
			}
		}
	}

	// Check if window already exists
	windowID := getWindowID(config, name)
	if tmuxWindowExistsByID(windowID, winName) {
		target := tmuxTargetByID(windowID, winName)
		// Extract session name from target "session:window" (only for name-based targets)
		sessName := strings.SplitN(target, ":", 2)[0]
		if os.Getenv("TMUX") != "" {
			cmd := exec.Command(tmuxPath, "select-window", "-t", target)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		exec.Command(tmuxPath, "select-window", "-t", target).Run()
		cmd := exec.Command(tmuxPath, "attach-session", "-t", sessName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Create new window
	windowID, err = createTmuxWindow(winName, cwd, continueSession, providerName, "")
	if err != nil {
		return err
	}

	// Store window ID back to config
	if config.Sessions[name] != nil {
		config.Sessions[name].WindowID = windowID
		saveConfig(config)
	}

	target := tmuxTargetByID(windowID, winName)
	sessName := strings.SplitN(target, ":", 2)[0]
	if os.Getenv("TMUX") != "" {
		cmd := exec.Command(tmuxPath, "select-window", "-t", target)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	cmd := exec.Command(tmuxPath, "attach-session", "-t", sessName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// startDetached creates a Telegram topic, tmux window with Claude, and sends a prompt (no attach)
func startDetached(name string, workDir string, prompt string) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if config.Sessions == nil {
		config.Sessions = make(map[string]*SessionInfo)
	}

	// Get the provider to use for this session
	provider := getActiveProvider(config)
	providerName := ""
	if provider != nil && config.ActiveProvider != "" {
		providerName = config.ActiveProvider
	}

	// Create Telegram topic
	topicID, err := createForumTopic(config, name, providerName)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	winName := tmuxSafeName(name)

	// Kill existing window if any
	oldWindowID := getWindowID(config, name)
	if tmuxWindowExistsByID(oldWindowID, winName) {
		killTmuxWindow(oldWindowID, winName)
		time.Sleep(300 * time.Millisecond)
	}

	// Create tmux window (detached)
	windowID, err := createTmuxWindow(winName, workDir, false, providerName, "")
	if err != nil {
		return fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Save session info
	config.Sessions[name] = &SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		WindowID:     windowID,
		ProviderName: providerName,
	}
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	target := tmuxTargetByID(windowID, winName)

	// Wait for Claude to be ready before sending prompt
	if err := waitForClaude(target, 30*time.Second); err != nil {
		return fmt.Errorf("claude did not start in time: %w", err)
	}

	// Send the prompt to the tmux window
	if err := sendToTmux(target, prompt); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	fmt.Printf("Session '%s' started in window '%s' with topic %d\n", name, winName, topicID)
	return nil
}
