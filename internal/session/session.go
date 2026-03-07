package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kidandcat/ccc/internal/config"
	"github.com/kidandcat/ccc/internal/telegram"
	"github.com/kidandcat/ccc/internal/tmux"
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
	info     *config.SessionInfo
	priority int
}

// getWindowID safely looks up the tmux WindowID from config for a session name.
// Returns empty string if the session or WindowID is not set.
func getWindowID(cfg *config.Config, sessionName string) string {
	if cfg == nil || cfg.Sessions == nil {
		return ""
	}
	info, exists := cfg.Sessions[sessionName]
	if !exists || info == nil {
		return ""
	}
	return info.WindowID
}

// getSessionWorkDir returns the correct working directory for a session.
// For worktree sessions, this returns the base repository path (not the .claude/worktrees path).
// For regular sessions, this returns the session's stored Path.
func GetSessionWorkDir(cfg *config.Config, sessionName string, sessionInfo *config.SessionInfo) string {
	if sessionInfo == nil {
		if cfg != nil && cfg.Sessions != nil {
			sessionInfo = cfg.Sessions[sessionName]
		}
		if sessionInfo == nil {
			return config.ResolveProjectPath(cfg, sessionName)
		}
	}

	// For worktree sessions, use the base session's path
	if sessionInfo.IsWorktree && sessionInfo.BaseSession != "" {
		if cfg != nil && cfg.Sessions != nil {
			if baseInfo := cfg.Sessions[sessionInfo.BaseSession]; baseInfo != nil && baseInfo.Path != "" {
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
	return config.ResolveProjectPath(cfg, sessionName)
}

// attachToTmuxSession attaches the user's terminal to the tmux session for a given session name.
// If already inside tmux, it selects the window. Otherwise, it attaches to the session.
func attachToTmuxSession(sessionName string) error {
	target, err := tmux.GetCccWindowTarget(sessionName)
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

	if os.Getenv("TMUX") != "" {
		// Inside tmux: just select the window
		cmd := exec.Command(tmux.TmuxPath, "select-window", "-t", target)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Outside tmux: need to attach to the session and select the window
	sessName := strings.SplitN(target, ":", 2)[0]
	cmd := exec.Command(tmux.TmuxPath, "attach-session", "-t", sessName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start attach in background, then select the window
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait for attach to complete, then select the window
	time.Sleep(tmuxAttachDelay)
	exec.Command(tmux.TmuxPath, "select-window", "-t", target).Run()

	return cmd.Wait()
}

// findSessionForPath finds the best matching session for a given directory path.
// Uses deterministic selection: exact path match first, then longest prefix match.
// Returns the session name and info, or empty strings if no match found.
func findSessionForPath(cfg *config.Config, cwd string) (string, *config.SessionInfo) {
	if cfg == nil || cfg.Sessions == nil {
		return "", nil
	}

	var matches []sessionMatch

	for name, info := range cfg.Sessions {
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

// generateUniqueSessionName creates a unique session name based on the current directory.
// If the basename collides with an existing session, it uses the parent directory as prefix.
// If that also collides, it appends a counter.
func generateUniqueSessionName(cfg *config.Config, cwd string, basename string) string {
	sessionName := basename

	// Check for collision
	if _, exists := cfg.Sessions[sessionName]; !exists {
		return sessionName
	}

	// Check if the collision is with the same path (not a real collision)
	if info, ok := cfg.Sessions[sessionName]; ok && info != nil && info.Path == cwd {
		return sessionName
	}

	// Collision detected - use parent directory as prefix
	parentDir := filepath.Base(filepath.Dir(cwd))
	sessionName = parentDir + "-" + sessionName

	// Check again for collision
	if _, exists := cfg.Sessions[sessionName]; !exists {
		return sessionName
	}

	// Still colliding - use counter suffix
	counter := 1
	for {
		candidateName := fmt.Sprintf("%s-%d", sessionName, counter)
		if _, exists := cfg.Sessions[candidateName]; !exists {
			return candidateName
		}
		counter++
	}
}

func createSession(cfg *config.Config, name string) error {
	// Check if session already exists
	if _, exists := cfg.Sessions[name]; exists {
		return fmt.Errorf("session '%s' already exists", name)
	}

	// Get the provider to use for this session
	provider := config.GetActiveProvider(cfg)
	providerName := ""
	if provider != nil && cfg.ActiveProvider != "" {
		providerName = cfg.ActiveProvider
	}

	// Create Telegram topic
	topicID, err := telegram.CreateForumTopic(cfg, name, providerName)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	// Create work directory
	workDir := config.ResolveProjectPath(cfg, name)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		// Create project directory
		os.MkdirAll(workDir, 0755)
	}

	// Save session info first
	cfg.Sessions[name] = &config.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// TODO: Ensure hooks are installed in the project directory
	// if err := ensureHooksForSession(cfg, name, cfg.Sessions[name]); err != nil {
	// 	fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
	// }

	// Switch to the new session in the single ccc window
	if err := tmux.SwitchSessionInWindow(name, workDir, providerName, "", "", false, true); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	return nil
}

func killSession(cfg *config.Config, name string) error {
	if _, exists := cfg.Sessions[name]; !exists {
		return fmt.Errorf("session '%s' not found", name)
	}

	// Remove from config (no need to kill tmux window with single window approach)
	delete(cfg.Sessions, name)
	config.SaveConfig(cfg)

	return nil
}

func GetSessionByTopic(cfg *config.Config, topicID int64) string {
	for name, info := range cfg.Sessions {
		if info != nil && info.TopicID == topicID {
			return name
		}
	}
	return ""
}

// Start creates/attaches to a tmux window with Telegram topic
func Start(continueSession bool) error {
	// Get current directory name as session name
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	name := filepath.Base(cwd)

	// Load config to check/create topic
	cfg, err := config.LoadConfig()
	if err != nil {
		// No config, just run claude directly with default provider
		return tmux.RunClaudeRaw(continueSession, "", "", "")
	}

	// Get the provider to use for this session
	providerName := ""
	if cfg.Sessions[name] != nil && cfg.Sessions[name].ProviderName != "" {
		// Use the session's saved provider
		providerName = cfg.Sessions[name].ProviderName
	} else if cfg.ActiveProvider != "" {
		// Use the active provider
		providerName = cfg.ActiveProvider
	}

	// Get stored session ID if continuing
	resumeSessionID := ""
	if continueSession && cfg.Sessions[name] != nil {
		resumeSessionID = cfg.Sessions[name].ClaudeSessionID
	}

	// Create topic if it doesn't exist and we have a group configured
	if cfg.GroupID != 0 {
		if _, exists := cfg.Sessions[name]; !exists {
			topicID, err := telegram.CreateForumTopic(cfg, name, providerName)
			if err == nil {
				cfg.Sessions[name] = &config.SessionInfo{
					TopicID:      topicID,
					Path:         cwd,
					ProviderName: providerName,
				}
				config.SaveConfig(cfg)

				// TODO: Ensure hooks are installed in the project directory
				// if err := ensureHooksForSession(cfg, name, cfg.Sessions[name]); err != nil {
				// 	fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
				// }

				fmt.Printf("📱 Created Telegram topic: %s\n", name)
			}
		}
	}

	// For existing sessions, ensure hooks are present (both for new and existing sessions)
	// TODO: Re-enable when ensureHooksForSession is implemented
	// if cfg.Sessions[name] != nil {
	// 	if err := ensureHooksForSession(cfg, name, cfg.Sessions[name]); err != nil {
	// 		fmt.Printf("⚠️ Failed to verify/install hooks: %v\n", err)
	// 	}
	// }

	// Switch to the session in the single ccc window
	workDir := cwd
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	// Check if this is a worktree session
	worktreeName := ""
	if cfg.Sessions[name] != nil && cfg.Sessions[name].IsWorktree {
		worktreeName = cfg.Sessions[name].WorktreeName
	}

	if err := tmux.SwitchSessionInWindow(name, workDir, providerName, resumeSessionID, worktreeName, continueSession, true); err != nil {
		return fmt.Errorf("failed to switch session: %w", err)
	}

	// Get the ccc window target for attaching
	target, err := tmux.GetCccWindowTarget(name)
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

	if os.Getenv("TMUX") != "" {
		// Inside tmux: just select the window
		cmd := exec.Command(tmux.TmuxPath, "select-window", "-t", target)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Outside tmux: need to attach to the session and select the window
	// First attach to the session, then select the specific window
	sessName := strings.SplitN(target, ":", 2)[0]
	cmd := exec.Command(tmux.TmuxPath, "attach-session", "-t", sessName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start attach in background, then select the window
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait a moment for attach to complete, then select the window
	time.Sleep(100 * time.Millisecond)
	exec.Command(tmux.TmuxPath, "select-window", "-t", target).Run()

	// Wait for attach command to complete
	return cmd.Wait()
}

// StartDetached creates a Telegram topic, tmux window with Claude, and sends a prompt (no attach)
func StartDetached(name string, workDir string, prompt string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*config.SessionInfo)
	}

	// Get the provider to use for this session
	provider := config.GetActiveProvider(cfg)
	providerName := ""
	if provider != nil && cfg.ActiveProvider != "" {
		providerName = cfg.ActiveProvider
	}

	// Create Telegram topic
	topicID, err := telegram.CreateForumTopic(cfg, name, providerName)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	// Switch to the new session in the single ccc window
	if err := tmux.SwitchSessionInWindow(name, workDir, providerName, "", "", false, true); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	// Save session info (no WindowID needed for single window)
	cfg.Sessions[name] = &config.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// TODO: Ensure hooks are installed in the project directory
	// if err := ensureHooksForSession(cfg, name, cfg.Sessions[name]); err != nil {
	// 	return fmt.Errorf("failed to install hooks: %w", err)
	// }

	// Get the ccc window target
	target, err := tmux.GetCccWindowTarget(name)
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

	// Wait for Claude to be ready before sending prompt
	if err := tmux.WaitForClaude(target, 30*time.Second); err != nil {
		return fmt.Errorf("claude did not start in time: %w", err)
	}

	// Send the prompt to the tmux window
	if err := tmux.SendToTmux(target, prompt); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	fmt.Printf("Session '%s' started in ccc window with topic %d\n", name, topicID)
	return nil
}

// StartInCurrentDir starts a session for the current working directory.
// If a session already exists for this directory, it attaches to it.
// If not, it creates a new session with topic, hooks, and starts Claude.
// This is the default behavior when running "ccc" from terminal.
func StartInCurrentDir(message string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config. Run: ccc setup <bot_token>")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check for existing session
	existingSession, existingInfo := findSessionForPath(cfg, cwd)
	if existingSession != "" && existingInfo != nil {
		return attachToExistingSession(cfg, existingSession, existingInfo, message)
	}

	// Create new session
	basename := filepath.Base(cwd)
	sessionName := generateUniqueSessionName(cfg, cwd, basename)

	if cfg.GroupID == 0 {
		return startLocalSession(cfg, sessionName, cwd, message)
	}

	return startTelegramSession(cfg, sessionName, cwd, message)
}

// attachToExistingSession attaches to an existing session and sends a message if provided.
func attachToExistingSession(cfg *config.Config, sessionName string, sessionInfo *config.SessionInfo, message string) error {
	workDir := GetSessionWorkDir(cfg, sessionName, sessionInfo)
	resumeSessionID := sessionInfo.ClaudeSessionID

	// Preserve worktree context
	worktreeName := ""
	if sessionInfo.IsWorktree {
		worktreeName = sessionInfo.WorktreeName
	}

	// TODO: Ensure hooks are installed
	// if err := ensureHooksForSession(cfg, sessionName, sessionInfo); err != nil {
	// 	fmt.Printf("Warning: failed to verify hooks: %v\n", err)
	// }

	// Send Telegram message before attaching (tmux attach blocks)
	// Local message is sent after session restart to avoid pane respawn losing it
	if message != "" && cfg.GroupID != 0 {
		// Telegram mode: send to topic
		if err := telegram.SendMessage(cfg, cfg.GroupID, sessionInfo.TopicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	// Restart the session
	// Use continueSession=true only if we have a resumeSessionID
	// Use skipRestart=false to force Claude to start if not running
	// (skipRestart=true is too conservative and prevents new sessions from starting)
	continueSession := resumeSessionID != ""
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, continueSession, false); err != nil {
		return fmt.Errorf("failed to attach to session '%s': %w", sessionName, err)
	}

	// Send local message after session restart (before attaching)
	// This ensures the message reaches the restarted Claude process
	if message != "" && cfg.GroupID == 0 {
		target, err := tmux.GetCccWindowTarget(sessionName)
		if err != nil {
			fmt.Printf("Warning: failed to get window for message: %v\n", err)
		} else if err := tmux.SendToTmux(target, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	fmt.Printf("Attached to existing session '%s'\n", sessionName)
	return attachToTmuxSession(sessionName)
}

// startLocalSession starts a session without Telegram integration (local-only mode).
// If a message is provided, it will be sent to the session after starting.
func startLocalSession(cfg *config.Config, sessionName, workDir, message string) error {
	providerName := cfg.ActiveProvider

	// Initialize sessions map if needed
	if cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*config.SessionInfo)
	}

	// Create session info
	cfg.Sessions[sessionName] = &config.SessionInfo{
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// TODO: Ensure hooks are installed
	// if err := ensureHooksForSession(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
	// 	fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
	// }

	// Start the session
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	// Send message to session if provided
	if message != "" {
		target, err := tmux.GetCccWindowTarget(sessionName)
		if err != nil {
			fmt.Printf("Warning: failed to get window for message: %v\n", err)
		} else if err := tmux.SendToTmux(target, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	fmt.Printf("Started local session '%s' (no Telegram integration)\n", sessionName)
	return attachToTmuxSession(sessionName)
}

// startTelegramSession starts a session with Telegram integration.
func startTelegramSession(cfg *config.Config, sessionName, workDir, message string) error {
	// Get provider
	provider := config.GetActiveProvider(cfg)
	providerName := ""
	if provider != nil && cfg.ActiveProvider != "" {
		providerName = cfg.ActiveProvider
	}

	// Create Telegram topic
	topicID, err := telegram.CreateForumTopic(cfg, sessionName, providerName)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	// Save session info
	if cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*config.SessionInfo)
	}
	cfg.Sessions[sessionName] = &config.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// TODO: Ensure hooks are installed
	// if err := ensureHooksForSession(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
	// 	fmt.Printf("Warning: failed to install hooks: %v\n", err)
	// }

	// Start the session
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	providerMsg := ""
	if providerName != "" {
		providerMsg = fmt.Sprintf(" (provider: %s)", providerName)
	}
	fmt.Printf("Created new session '%s'%s\n", sessionName, providerMsg)

	// Send message before attaching
	if message != "" {
		if err := telegram.SendMessage(cfg, cfg.GroupID, topicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	return attachToTmuxSession(sessionName)
}
