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

// getSessionWorkDir returns the correct working directory for a session.
// For worktree sessions, this returns the base repository path (not the .claude/worktrees path).
// For regular sessions, this returns the session's stored Path.
func getSessionWorkDir(config *Config, sessionName string, sessionInfo *SessionInfo) string {
	if sessionInfo == nil {
		if config != nil && config.Sessions != nil {
			sessionInfo = config.Sessions[sessionName]
		}
		if sessionInfo == nil {
			return resolveProjectPath(config, sessionName)
		}
	}

	// For worktree sessions, use the base session's path
	if sessionInfo.IsWorktree && sessionInfo.BaseSession != "" {
		if config != nil && config.Sessions != nil {
			if baseInfo := config.Sessions[sessionInfo.BaseSession]; baseInfo != nil && baseInfo.Path != "" {
				return baseInfo.Path
			}
		}
		// Fallback: derive from worktree path (remove .claude/worktrees/<name>/ suffix)
		worktreePath := sessionInfo.Path
		if strings.HasSuffix(worktreePath, "/.claude/worktrees/"+sessionInfo.WorktreeName) {
			return strings.TrimSuffix(worktreePath, "/.claude/worktrees/"+sessionInfo.WorktreeName)
		}
	}

	// For regular sessions, use the stored Path
	if sessionInfo.Path != "" {
		return sessionInfo.Path
	}
	return resolveProjectPath(config, sessionName)
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

	// Create work directory
	workDir := resolveProjectPath(config, name)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		// Create project directory
		os.MkdirAll(workDir, 0755)
	}

	// Switch to the new session in the single ccc window
	if err := switchSessionInWindow(name, workDir, providerName, "", "", false, true); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	// Save mapping with full path (no WindowID needed for single window)
	config.Sessions[name] = &SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
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

	// Remove from config (no need to kill tmux window with single window approach)
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

	// Load config to check/create topic
	config, err := loadConfig()
	if err != nil {
		// No config, just run claude directly with default provider
		return runClaudeRaw(continueSession, "", "", "")
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

	// Get stored session ID if continuing
	resumeSessionID := ""
	if continueSession && config.Sessions[name] != nil {
		resumeSessionID = config.Sessions[name].ClaudeSessionID
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

	// Switch to the session in the single ccc window
	workDir := cwd
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	// Check if this is a worktree session
	worktreeName := ""
	if config.Sessions[name] != nil && config.Sessions[name].IsWorktree {
		worktreeName = config.Sessions[name].WorktreeName
	}

	if err := switchSessionInWindow(name, workDir, providerName, resumeSessionID, worktreeName, continueSession, true); err != nil {
		return fmt.Errorf("failed to switch session: %w", err)
	}

	// Get the ccc session name for attaching
	target, err := getCccWindowTarget()
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

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

	// Switch to the new session in the single ccc window
	if err := switchSessionInWindow(name, workDir, providerName, "", "", false, true); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	// Save session info (no WindowID needed for single window)
	config.Sessions[name] = &SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Get the ccc window target
	target, err := getCccWindowTarget()
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

	// Wait for Claude to be ready before sending prompt
	if err := waitForClaude(target, 30*time.Second); err != nil {
		return fmt.Errorf("claude did not start in time: %w", err)
	}

	// Send the prompt to the tmux window
	if err := sendToTmux(target, prompt); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	fmt.Printf("Session '%s' started in ccc window with topic %d\n", name, topicID)
	return nil
}
