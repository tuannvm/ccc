package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Session matching priority constants
const (
	priorityExactPath   = 0 // Exact path match (highest priority)
	priorityPrefixMatch = 1 // Parent directory prefix match
)

// Tmux attach delay - time to wait for attach command to complete
const tmuxAttachDelay = 100 * time.Millisecond

// sessionMatch represents a potential session match with its priority
type sessionMatch struct {
	name     string
	info     *SessionInfo
	priority int
}

// tmuxSafeName is in tmux_session.go (wrapper for tmux.SafeName)

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

	// Backward compatibility: check old default path ($HOME/<sessionName>) for existing sessions
	// This ensures sessions created before the ~/Projects default can still be found
	home, _ := os.UserHomeDir()
	oldPath := filepath.Join(home, sessionName)
	if _, err := os.Stat(oldPath); err == nil {
		// Old path exists, use it for backward compatibility
		return oldPath
	}

	// Use new default path (~/Projects/<sessionName>)
	return resolveProjectPath(config, sessionName)
}

// findSessionForPath finds the best matching session for a given directory path.
// Uses deterministic selection: exact path match first, then longest prefix match.
// Returns the session name and info, or empty strings if no match found.
func findSessionForPath(config *Config, cwd string) (string, *SessionInfo) {
	if config == nil || config.Sessions == nil {
		return "", nil
	}

	var matches []sessionMatch

	for name, info := range config.Sessions {
		if info == nil {
			continue
		}

		if cwd == info.Path {
			// Exact path match (highest priority)
			matches = append(matches, sessionMatch{name: name, info: info, priority: priorityExactPath})
		} else if info.Path != "" && strings.HasPrefix(cwd, info.Path+"/") {
			// Parent directory prefix match (medium priority)
			// Only match if path is not empty to avoid matching every directory when path is "/"
			matches = append(matches, sessionMatch{name: name, info: info, priority: priorityPrefixMatch})
		}
	}

	if len(matches) == 0 {
		return "", nil
	}

	// Find the best match: lowest priority, then longest path for prefix matches
	bestMatch := matches[0]
	for _, m := range matches {
		if m.priority < bestMatch.priority {
			bestMatch = m
		} else if m.priority == priorityPrefixMatch && bestMatch.priority == priorityPrefixMatch {
			// Both are prefix matches - pick the longest path (most specific)
			if len(m.info.Path) > len(bestMatch.info.Path) {
				bestMatch = m
			}
		}
	}

	return bestMatch.name, bestMatch.info
}

// getWorktreeNames returns a set of existing worktree names at basePath.
// Returns nil if the worktrees directory doesn't exist.
func getWorktreeNames(basePath string) map[string]bool {
	worktreesDir := filepath.Join(basePath, ".claude", "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil
	}

	names := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			names[entry.Name()] = true
		}
	}
	return names
}

// waitForNewWorktree polls for a new worktree directory to be created.
// Takes a snapshot of existing worktrees and waits for a new one to appear.
// Returns the new worktree name or empty string if timeout occurs.
func waitForNewWorktree(basePath string, existingNames map[string]bool, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		currentNames := getWorktreeNames(basePath)
		if currentNames == nil {
			// Directory doesn't exist yet, wait for it
			time.Sleep(pollInterval)
			continue
		}

		// Find a new name that wasn't in the existing snapshot
		var newName string
		for name := range currentNames {
			if !existingNames[name] {
				newName = name
				break
			}
		}

		if newName != "" {
			// Found a potential new worktree - confirm it's stable (not a transient file)
			// Wait one more poll interval and verify it still exists
			time.Sleep(pollInterval)
			confirmNames := getWorktreeNames(basePath)
			if confirmNames != nil && confirmNames[newName] {
				// Confirmed: worktree still exists after one poll cycle
				listenLog("[waitForNewWorktree] Confirmed new worktree: %s", newName)
				return newName
			}
			// Not confirmed - was transient or removed, continue waiting
			listenLog("[waitForNewWorktree] Worktree %s not confirmed, continuing to wait", newName)
		}

		time.Sleep(pollInterval)
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
			topicID, err := createForumTopic(config, name, providerName, "")
			if err == nil {
				config.Sessions[name] = &SessionInfo{
					TopicID:      topicID,
					Path:         cwd,
					ProviderName: providerName,
				}
				saveConfig(config)

				// Ensure hooks are installed in the project directory
				if err := ensureHooksForSession(config, name, config.Sessions[name]); err != nil {
					fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
				}

				fmt.Printf("📱 Created Telegram topic: %s\n", name)
			}
		}
	}

	// For existing sessions, ensure hooks are present (both for new and existing sessions)
	if config.Sessions[name] != nil {
		if err := ensureHooksForSession(config, name, config.Sessions[name]); err != nil {
			fmt.Printf("⚠️ Failed to verify/install hooks: %v\n", err)
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

	// Get the ccc window target for attaching
	target, err := getCccWindowTarget(name)
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
	// First attach to the session, then select the specific window
	sessName := strings.SplitN(target, ":", 2)[0]
	cmd := exec.Command(tmuxPath, "attach-session", "-t", sessName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start attach in background, then select the window
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait a moment for attach to complete, then select the window
	time.Sleep(100 * time.Millisecond)
	exec.Command(tmuxPath, "select-window", "-t", target).Run()

	// Wait for attach command to complete
	return cmd.Wait()
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
	topicID, err := createForumTopic(config, name, providerName, "")
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

	// Ensure hooks are installed in the project directory
	if err := ensureHooksForSession(config, name, config.Sessions[name]); err != nil {
		return fmt.Errorf("failed to install hooks: %w", err)
	}

	// Get the ccc window target
	target, err := getCccWindowTarget(name)
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

// startSessionInCurrentDir starts a session for the current working directory.
// If a session already exists for this directory, it attaches to it.
// If not, it creates a new session with topic, hooks, and starts Claude.
// This is the default behavior when running "ccc" from terminal.
func startSessionInCurrentDir(message string) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config. Run: ccc setup <bot_token>")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check for existing session
	existingSession, existingInfo := findSessionForPath(config, cwd)
	if existingSession != "" && existingInfo != nil {
		return attachToExistingSession(config, existingSession, existingInfo, message)
	}

	// Create new session
	basename := filepath.Base(cwd)
	sessionName := generateUniqueSessionName(config, cwd, basename)

	if config.GroupID == 0 {
		return startLocalSession(config, sessionName, cwd, message)
	}

	return startTelegramSession(config, sessionName, cwd, message)
}
