package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func sessionName(name string) string {
	// Replace dots with underscores - tmux interprets dots as window/pane separators
	safeName := strings.ReplaceAll(name, ".", "_")
	return "claude-" + safeName
}

func createSession(config *Config, name string) error {
	// Check if session already exists
	if _, exists := config.Sessions[name]; exists {
		return fmt.Errorf("session '%s' already exists", name)
	}

	// Create Telegram topic
	topicID, err := createForumTopic(config, name)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	// Create tmux session
	workDir := resolveProjectPath(config, name)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		// Create project directory
		os.MkdirAll(workDir, 0755)
	}

	if err := createTmuxSession(sessionName(name), workDir, false); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Save mapping with full path
	config.Sessions[name] = &SessionInfo{
		TopicID: topicID,
		Path:    workDir,
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

	// Kill tmux session
	killTmuxSession(sessionName(name))

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

// startSession creates/attaches to a tmux session with Telegram topic
func startSession(continueSession bool) error {
	// Get current directory name as session name
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	name := filepath.Base(cwd)
	tmuxName := sessionName(name)

	// Load config to check/create topic
	config, err := loadConfig()
	if err != nil {
		// No config, just run claude directly
		return runClaudeRaw(continueSession)
	}

	// Create topic if it doesn't exist and we have a group configured
	if config.GroupID != 0 {
		if _, exists := config.Sessions[name]; !exists {
			topicID, err := createForumTopic(config, name)
			if err == nil {
				config.Sessions[name] = &SessionInfo{
					TopicID: topicID,
					Path:    cwd,
				}
				saveConfig(config)
				fmt.Printf("📱 Created Telegram topic: %s\n", name)
			}
		}
	}

	// Check if tmux session exists
	if tmuxSessionExists(tmuxName) {
		// Check if we're already inside tmux
		if os.Getenv("TMUX") != "" {
			// Inside tmux: switch to the session
			cmd := exec.Command(tmuxPath, "switch-client", "-t", tmuxName)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		// Outside tmux: attach to existing session
		cmd := exec.Command(tmuxPath, "attach-session", "-t", tmuxName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Create new tmux session and attach
	if err := createTmuxSession(tmuxName, cwd, continueSession); err != nil {
		return err
	}

	// Check if we're already inside tmux
	if os.Getenv("TMUX") != "" {
		cmd := exec.Command(tmuxPath, "switch-client", "-t", tmuxName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	cmd := exec.Command(tmuxPath, "attach-session", "-t", tmuxName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// startDetached creates a Telegram topic, tmux session with Claude, and sends a prompt (no attach)
func startDetached(name string, workDir string, prompt string) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if config.Sessions == nil {
		config.Sessions = make(map[string]*SessionInfo)
	}

	// Create Telegram topic
	topicID, err := createForumTopic(config, name)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	tmuxName := sessionName(name)

	// Kill existing tmux session if any
	if tmuxSessionExists(tmuxName) {
		killTmuxSession(tmuxName)
	}

	// Create tmux session (detached)
	if err := createTmuxSession(tmuxName, workDir, false); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Save session info
	config.Sessions[name] = &SessionInfo{
		TopicID: topicID,
		Path:    workDir,
	}
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Wait for Claude to be ready before sending prompt
	if err := waitForClaude(tmuxName, 30*time.Second); err != nil {
		return fmt.Errorf("claude did not start in time: %w", err)
	}

	// Send the prompt to the tmux session
	if err := sendToTmux(tmuxName, prompt); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	fmt.Printf("Session '%s' started in tmux '%s' with topic %d\n", name, tmuxName, topicID)
	return nil
}

