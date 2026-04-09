package listen

import (
	"fmt"
	"os"
	"path/filepath"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/lookup"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// AttachToExistingSession attaches to an existing session and sends a message if provided.
func AttachToExistingSession(cfg *configpkg.Config, sessionName string, sessionInfo *configpkg.SessionInfo, message string, attachFunc func(string) error) error {
	workDir := lookup.GetSessionWorkDir(cfg, sessionName, sessionInfo)
	worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)

	// Ensure hooks are installed
	if err := EnsureHooks(cfg, sessionName, sessionInfo); err != nil {
		fmt.Printf("Warning: failed to verify hooks: %v\n", err)
	}

	// Send Telegram message before attaching (tmux attach blocks)
	if message != "" && cfg.GroupID != 0 {
		if err := telegram.SendMessage(cfg, cfg.GroupID, sessionInfo.TopicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	// Restart the session
	continueSession := resumeSessionID != ""
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, continueSession, false); err != nil {
		return fmt.Errorf("failed to attach to session '%s': %w", sessionName, err)
	}

	// Send local message after session restart
	if message != "" && cfg.GroupID == 0 {
		target, err := tmux.GetWindowTarget(sessionName)
		if err != nil {
			fmt.Printf("Warning: failed to get window for message: %v\n", err)
		} else if err := tmux.SendKeys(target, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	fmt.Printf("Attached to existing session '%s'\n", sessionName)
	return attachFunc(sessionName)
}

// StartLocalSession starts a session without Telegram integration (local-only mode).
func StartLocalSession(cfg *configpkg.Config, sessionName, workDir, message string, attachFunc func(string) error) error {
	providerName := cfg.ActiveProvider

	if cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*configpkg.SessionInfo)
	}

	cfg.Sessions[sessionName] = &configpkg.SessionInfo{
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := configpkg.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := EnsureHooks(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
		fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
	}

	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	if message != "" {
		target, err := tmux.GetWindowTarget(sessionName)
		if err != nil {
			fmt.Printf("Warning: failed to get window for message: %v\n", err)
		} else if err := tmux.SendKeys(target, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	fmt.Printf("Started local session '%s' (no Telegram integration)\n", sessionName)
	return attachFunc(sessionName)
}

// StartTelegramSession starts a session with Telegram integration.
func StartTelegramSession(cfg *configpkg.Config, sessionName, workDir, message string, attachFunc func(string) error) error {
	provider := providerpkg.GetActiveProvider(cfg)
	providerName := ""
	if provider != nil && cfg.ActiveProvider != "" {
		providerName = cfg.ActiveProvider
	}

	topicID, err := telegram.CreateForumTopic(cfg, sessionName, providerName, "")
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	if cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*configpkg.SessionInfo)
	}
	cfg.Sessions[sessionName] = &configpkg.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := configpkg.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := EnsureHooks(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
		fmt.Printf("Warning: failed to install hooks: %v\n", err)
	}

	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	providerMsg := ""
	if providerName != "" {
		providerMsg = fmt.Sprintf(" (provider: %s)", providerName)
	}
	fmt.Printf("Created new session '%s'%s\n", sessionName, providerMsg)

	if message != "" {
		if err := telegram.SendMessage(cfg, cfg.GroupID, topicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	return attachFunc(sessionName)
}

// StartSession creates/attaches to a tmux window with Telegram topic.
func StartSession(continueSession bool, attachFunc func(string) error) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	name := filepath.Base(cwd)

	config, err := configpkg.Load()
	if err != nil {
		// No config, just run claude directly with default provider
		return tmux.RunClaudeRaw(continueSession, "", "", "", "", nil)
	}

	providerName := ""
	if config.Sessions[name] != nil && config.Sessions[name].ProviderName != "" {
		providerName = config.Sessions[name].ProviderName
	} else if config.ActiveProvider != "" {
		providerName = config.ActiveProvider
	}

	resumeSessionID := ""
	if continueSession && config.Sessions[name] != nil {
		resumeSessionID = config.Sessions[name].ClaudeSessionID
	}

	if config.GroupID != 0 {
		if _, exists := config.Sessions[name]; !exists {
			topicID, err := telegram.CreateForumTopic(config, name, providerName, "")
			if err == nil {
				config.Sessions[name] = &configpkg.SessionInfo{
					TopicID:      topicID,
					Path:         cwd,
					ProviderName: providerName,
				}
				configpkg.Save(config)

				if err := EnsureHooks(config, name, config.Sessions[name]); err != nil {
					fmt.Printf("⚠️ Failed to install hooks: %v\n", err)
				}

				fmt.Printf("📱 Created Telegram topic: %s\n", name)
			}
		}
	}

	if config.Sessions[name] != nil {
		if err := EnsureHooks(config, name, config.Sessions[name]); err != nil {
			fmt.Printf("⚠️ Failed to verify/install hooks: %v\n", err)
		}
	}

	workDir := cwd
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	worktreeName := ""
	if config.Sessions[name] != nil && config.Sessions[name].IsWorktree {
		worktreeName = config.Sessions[name].WorktreeName
	}

	if err := tmux.SwitchSessionInWindow(name, workDir, providerName, resumeSessionID, worktreeName, continueSession, true); err != nil {
		return fmt.Errorf("failed to switch session: %w", err)
	}

	return attachFunc(name)
}

// StartDetached creates a Telegram topic, tmux window with Claude, and sends a prompt (no attach).
func StartDetached(name string, workDir string, prompt string) error {
	config, err := configpkg.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if config.Sessions == nil {
		config.Sessions = make(map[string]*configpkg.SessionInfo)
	}

	provider := providerpkg.GetActiveProvider(config)
	providerName := ""
	if provider != nil && config.ActiveProvider != "" {
		providerName = config.ActiveProvider
	}

	topicID, err := telegram.CreateForumTopic(config, name, providerName, "")
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	if err := tmux.SwitchSessionInWindow(name, workDir, providerName, "", "", false, true); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	config.Sessions[name] = &configpkg.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := configpkg.Save(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := EnsureHooks(config, name, config.Sessions[name]); err != nil {
		return fmt.Errorf("failed to install hooks: %w", err)
	}

	target, err := tmux.GetWindowTarget(name)
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

	if err := tmux.WaitForClaude(target, 30*1e9); err != nil { // 30s
		return fmt.Errorf("claude did not start in time: %w", err)
	}

	if err := tmux.SendKeys(target, prompt); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	fmt.Printf("Session '%s' started in ccc window with topic %d\n", name, topicID)
	return nil
}

// StartSessionInCurrentDir starts a session for the current working directory.
func StartSessionInCurrentDir(config *configpkg.Config, message string,
	attachFunc func(string) error,
	findSessionForPath func(*configpkg.Config, string) (string, *configpkg.SessionInfo),
	generateUniqueSessionName func(*configpkg.Config, string, string) string) error {

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	existingSession, existingInfo := findSessionForPath(config, cwd)
	if existingSession != "" && existingInfo != nil {
		return AttachToExistingSession(config, existingSession, existingInfo, message, attachFunc)
	}

	basename := filepath.Base(cwd)
	sessionName := generateUniqueSessionName(config, cwd, basename)

	if config.GroupID == 0 {
		return StartLocalSession(config, sessionName, cwd, message, attachFunc)
	}

	return StartTelegramSession(config, sessionName, cwd, message, attachFunc)
}

// StartSessionInCurrentDirAuto loads config and starts a session for the current working directory.
func StartSessionInCurrentDirAuto(message string,
	attachFunc func(string) error,
	findSessionForPath func(*configpkg.Config, string) (string, *configpkg.SessionInfo),
	generateUniqueSessionName func(*configpkg.Config, string, string) string) error {

	config, err := configpkg.Load()
	if err != nil {
		return fmt.Errorf("failed to load config. Run: ccc setup <bot_token>")
	}
	return StartSessionInCurrentDir(config, message, attachFunc, findSessionForPath, generateUniqueSessionName)
}
