package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// generateUniqueSessionName creates a unique session name based on the current directory.
// If the basename collides with an existing session, it uses the parent directory as prefix.
// If that also collides, it appends a counter.
func generateUniqueSessionName(config *Config, cwd string, basename string) string {
	sessionName := basename

	// Check for collision
	if _, exists := config.Sessions[sessionName]; !exists {
		return sessionName
	}

	// Check if the collision is with the same path (not a real collision)
	if info, ok := config.Sessions[sessionName]; ok && info != nil && info.Path == cwd {
		return sessionName
	}

	// Collision detected - use parent directory as prefix
	parentDir := filepath.Base(filepath.Dir(cwd))
	sessionName = parentDir + "-" + sessionName

	// Check again for collision
	if _, exists := config.Sessions[sessionName]; !exists {
		return sessionName
	}

	// Still colliding - use counter suffix
	counter := 1
	for {
		candidateName := fmt.Sprintf("%s-%d", sessionName, counter)
		if _, exists := config.Sessions[candidateName]; !exists {
			return candidateName
		}
		counter++
	}
}

// attachToExistingSession attaches to an existing session and sends a message if provided.
func attachToExistingSession(config *Config, sessionName string, sessionInfo *SessionInfo, message string) error {
	workDir := getSessionWorkDir(config, sessionName, sessionInfo)
	resumeSessionID := sessionInfo.ClaudeSessionID

	// Preserve worktree context
	worktreeName := ""
	if sessionInfo.IsWorktree {
		worktreeName = sessionInfo.WorktreeName
	}

	// Ensure hooks are installed
	if err := ensureHooksForSession(config, sessionName, sessionInfo); err != nil {
		fmt.Printf("Warning: failed to verify hooks: %v\n", err)
	}

	// Send Telegram message before attaching (tmux attach blocks)
	// Local message is sent after session restart to avoid pane respawn losing it
	if message != "" && config.GroupID != 0 {
		// Telegram mode: send to topic
		if err := sendMessage(config, config.GroupID, sessionInfo.TopicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	// Restart the session
	// Use continueSession=true only if we have a resumeSessionID
	// Use skipRestart=false to force Claude to start if not running
	// (skipRestart=true is too conservative and prevents new sessions from starting)
	continueSession := resumeSessionID != ""
	if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, continueSession, false); err != nil {
		return fmt.Errorf("failed to attach to session '%s': %w", sessionName, err)
	}

	// Send local message after session restart (before attaching)
	// This ensures the message reaches the restarted Claude process
	if message != "" && config.GroupID == 0 {
		target, err := getCccWindowTarget(sessionName)
		if err != nil {
			fmt.Printf("Warning: failed to get window for message: %v\n", err)
		} else if err := sendToTmux(target, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	fmt.Printf("Attached to existing session '%s'\n", sessionName)
	return attachToTmuxSession(sessionName)
}

// startLocalSession starts a session without Telegram integration (local-only mode).
// If a message is provided, it will be sent to the session after starting.
func startLocalSession(config *Config, sessionName, workDir, message string) error {
	providerName := config.ActiveProvider

	// Initialize sessions map if needed
	if config.Sessions == nil {
		config.Sessions = make(map[string]*SessionInfo)
	}

	// Create session info
	config.Sessions[sessionName] = &SessionInfo{
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Ensure hooks are installed
	if err := ensureHooksForSession(config, sessionName, config.Sessions[sessionName]); err != nil {
		fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
	}

	// Start the session
	if err := switchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	// Send message to session if provided
	if message != "" {
		target, err := getCccWindowTarget(sessionName)
		if err != nil {
			fmt.Printf("Warning: failed to get window for message: %v\n", err)
		} else if err := sendToTmux(target, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	fmt.Printf("Started local session '%s' (no Telegram integration)\n", sessionName)
	return attachToTmuxSession(sessionName)
}

// startTelegramSession starts a session with Telegram integration.
func startTelegramSession(config *Config, sessionName, workDir, message string) error {
	// Get provider
	provider := getActiveProvider(config)
	providerName := ""
	if provider != nil && config.ActiveProvider != "" {
		providerName = config.ActiveProvider
	}

	// Create Telegram topic
	topicID, err := createForumTopic(config, sessionName, providerName, "")
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	// Save session info
	if config.Sessions == nil {
		config.Sessions = make(map[string]*SessionInfo)
	}
	config.Sessions[sessionName] = &SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Ensure hooks are installed
	if err := ensureHooksForSession(config, sessionName, config.Sessions[sessionName]); err != nil {
		fmt.Printf("Warning: failed to install hooks: %v\n", err)
	}

	// Start the session
	if err := switchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	providerMsg := ""
	if providerName != "" {
		providerMsg = fmt.Sprintf(" (provider: %s)", providerName)
	}
	fmt.Printf("Created new session '%s'%s\n", sessionName, providerMsg)

	// Send message before attaching
	if message != "" {
		if err := sendMessage(config, config.GroupID, topicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	return attachToTmuxSession(sessionName)
}

// attachToTmuxSession attaches the user's terminal to the tmux session for a given session name.
// If already inside tmux, it selects the window. Otherwise, it attaches to the session.
func attachToTmuxSession(sessionName string) error {
	target, err := getCccWindowTarget(sessionName)
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

	if os.Getenv("TMUX") != "" {
		// Inside tmux: just select the window
		cmd := exec.Command(tmuxPath, "select-window", "-t", target)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Outside tmux: need to attach to the session and select the window
	sessName := strings.SplitN(target, ":", 2)[0]
	cmd := exec.Command(tmuxPath, "attach-session", "-t", sessName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start attach in background, then select the window
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait for attach to complete, then select the window
	time.Sleep(tmuxAttachDelay)
	exec.Command(tmuxPath, "select-window", "-t", target).Run()

	return cmd.Wait()
}
