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
// We replace dots with "__" (double underscore) to avoid name collisions
// while keeping the name readable and avoiding conflicts with natural underscores
func tmuxSafeName(name string) string {
	return strings.ReplaceAll(name, ".", "__")
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

	// Save session info first (needed for ensureHooksForSession)
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
		fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
	}

	// Switch to the new session in the single ccc window
	if err := switchSessionInWindow(name, workDir, providerName, "", "", false, true); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
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

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Generate session name from directory basename
	sessionName := filepath.Base(cwd)

	// Check if a session already exists for this directory
	// Use deterministic selection: exact path match first, then longest prefix match
	// IMPORTANT: We do NOT use suffix/basename matching to avoid attaching to wrong sessions
	// (e.g., /tmp/foo attaching to session for /work/foo just because they share the basename)
	type sessionMatch struct {
		name     string
		info     *SessionInfo
		priority int // 0 = exact path, 1 = prefix match
	}
	var matches []sessionMatch

	for name, info := range config.Sessions {
		if info == nil {
			continue
		}

		// Determine match type and priority
		if cwd == info.Path {
			// Exact path match (highest priority)
			matches = append(matches, sessionMatch{name: name, info: info, priority: 0})
		} else if strings.HasPrefix(cwd, info.Path+"/") {
			// Parent directory prefix match (medium priority)
			matches = append(matches, sessionMatch{name: name, info: info, priority: 1})
		}
		// NOTE: No suffix/basename match - too risky, can match wrong sessions
	}

	// For prefix matches, find the longest path (most specific)
	var existingSession string
	var existingSessionInfo *SessionInfo
	if len(matches) > 0 {
		// Sort by priority (ascending), then by path length (descending) for prefix matches
		bestMatch := matches[0]
		for _, m := range matches {
			if m.priority < bestMatch.priority {
				bestMatch = m
			} else if m.priority == 1 && bestMatch.priority == 1 {
				// Both are prefix matches - pick the longest path (most specific)
				if len(m.info.Path) > len(bestMatch.info.Path) {
					bestMatch = m
				}
			}
		}
		existingSession = bestMatch.name
		existingSessionInfo = bestMatch.info
	}

	// If session exists, attach/restart it
	if existingSession != "" && existingSessionInfo != nil {
		workDir := getSessionWorkDir(config, existingSession, existingSessionInfo)
		resumeSessionID := existingSessionInfo.ClaudeSessionID

		// Preserve worktree context if this is a worktree session
		worktreeName := ""
		if existingSessionInfo.IsWorktree {
			worktreeName = existingSessionInfo.WorktreeName
		}

			// Ensure hooks are installed
		if err := ensureHooksForSession(config, existingSession, existingSessionInfo); err != nil {
			fmt.Printf("Warning: failed to verify hooks: %v\n", err)
		}

		// If a message was provided, send it to the topic BEFORE attaching
		// (tmux attach blocks, so message must be sent first)
		if message != "" && config.GroupID != 0 {
			if err := sendMessage(config, config.GroupID, existingSessionInfo.TopicID, message); err != nil {
				fmt.Printf("Warning: failed to send message: %v\n", err)
			}
		}

		// Restart the session (preserve worktree name)
		if err := switchSessionInWindow(existingSession, workDir, existingSessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
			return fmt.Errorf("failed to attach to session '%s': %w", existingSession, err)
		}

		fmt.Printf("Attached to existing session '%s'\n", existingSession)

		// Actually attach to the tmux session (like startSession does)
		target, err := getCccWindowTarget(existingSession)
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

		// Wait a moment for attach to complete, then select the window
		time.Sleep(100 * time.Millisecond)
		exec.Command(tmuxPath, "select-window", "-t", target).Run()

		// Wait for attach command to complete
		return cmd.Wait()
	}

	// No existing session - create a new one
	// Check for session name collision (same basename, different path)
	// This prevents overwriting sessions from other directories with the same name
	// Apply this to both local-only and Telegram modes
	collisionFound := false
	for name, info := range config.Sessions {
		if info == nil {
			continue
		}
		if name == sessionName && info.Path != cwd {
			collisionFound = true
			break
		}
	}

	// If collision detected, create a unique session name using parent directory
	if collisionFound {
		parentDir := filepath.Base(filepath.Dir(cwd))
		sessionName = parentDir + "-" + sessionName
		// Check again for collision (unlikely but possible)
		for name, info := range config.Sessions {
			if info == nil {
				continue
			}
			if name == sessionName && info.Path != cwd {
				// Still colliding, use a counter suffix
				counter := 1
				for {
					candidateName := fmt.Sprintf("%s-%d", sessionName, counter)
					if _, exists := config.Sessions[candidateName]; !exists {
						sessionName = candidateName
						break
					}
					counter++
				}
				break
			}
		}
		fmt.Printf("Session name collision detected, using unique name: '%s'\n", sessionName)
	}

	// Support local-only mode (no GroupID) like startSession does
	if config.GroupID == 0 {
		// Local-only mode: just run claude directly
		// Get provider to use
		providerName := ""
		if config.ActiveProvider != "" {
			providerName = config.ActiveProvider
		}

		// Initialize sessions map if needed
		if config.Sessions == nil {
			config.Sessions = make(map[string]*SessionInfo)
		}

		// Create a basic session info for local use (no Telegram topic)
		config.Sessions[sessionName] = &SessionInfo{
			Path:         cwd,
			ProviderName: providerName,
		}
		saveConfig(config)

		// Ensure hooks are installed
		if err := ensureHooksForSession(config, sessionName, config.Sessions[sessionName]); err != nil {
			fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
		}

		// Run claude directly
		if err := switchSessionInWindow(sessionName, cwd, providerName, "", "", false, false); err != nil {
			return fmt.Errorf("failed to start session: %w", err)
		}

		// Attach to the tmux session
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
			if err := cmd.Run(); err != nil {
				return err
			}
		} else {
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

			// Wait a moment for attach to complete, then select the window
			time.Sleep(100 * time.Millisecond)
			exec.Command(tmuxPath, "select-window", "-t", target).Run()

			// Wait for attach command to complete
			if err := cmd.Wait(); err != nil {
				return err
			}
		}

		fmt.Printf("Started local session '%s' (no Telegram integration)\n", sessionName)
		return nil
	}


	// Get the provider to use for this session
	provider := getActiveProvider(config)
	providerName := ""
	if provider != nil && config.ActiveProvider != "" {
		providerName = config.ActiveProvider
	}

	// Create Telegram topic
	topicID, err := createForumTopic(config, sessionName, providerName)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	// Save session info (use current directory as path)
	if config.Sessions == nil {
		config.Sessions = make(map[string]*SessionInfo)
	}
	config.Sessions[sessionName] = &SessionInfo{
		TopicID:      topicID,
		Path:         cwd,
		ProviderName: providerName,
	}
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Ensure hooks are installed in the current directory
	if err := ensureHooksForSession(config, sessionName, config.Sessions[sessionName]); err != nil {
		fmt.Printf("Warning: failed to install hooks: %v\n", err)
	}

	// Start the session
	if err := switchSessionInWindow(sessionName, cwd, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	providerMsg := ""
	if providerName != "" {
		providerMsg = fmt.Sprintf(" (provider: %s)", providerName)
	}
	fmt.Printf("Created new session '%s'%s\n", sessionName, providerMsg)

	// If a message was provided, send it to the topic BEFORE attaching
	// (tmux attach blocks, so message must be sent first)
	if message != "" {
		if err := sendMessage(config, config.GroupID, topicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	// Attach to the tmux session (like startSession does)
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

	// Wait a moment for attach to complete, then select the window
	time.Sleep(100 * time.Millisecond)
	exec.Command(tmuxPath, "select-window", "-t", target).Run()

	// Wait for attach command to complete
	return cmd.Wait()
}
