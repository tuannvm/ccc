package listen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/lookup"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// AttachToExistingSession attaches to an existing session and sends a message if provided.
func AttachToExistingSession(cfg *configpkg.Config, sessionName string, sessionInfo *configpkg.SessionInfo, message string) error {
	workDir := lookup.GetSessionWorkDir(cfg, sessionName, sessionInfo)
	worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)

	providerName := effectiveProviderName(cfg, sessionInfo)
	if err := EnsureAgentHooks(cfg, sessionName, sessionInfo); err != nil {
		fmt.Printf("Warning: failed to verify hooks: %v\n", err)
	}

	// Send Telegram message before attaching (tmux attach blocks)
	if message != "" && cfg.GroupID != 0 {
		if err := telegram.SendMessage(cfg, cfg.GroupID, sessionInfo.TopicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	// Restart the session. Codex does not persist a Claude transcript session id,
	// so continuing maps to `codex resume --last` in the backend runner.
	continueSession := resumeSessionID != "" || isCodexProvider(cfg, providerName)
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, resumeSessionID, worktreeName, continueSession, false); err != nil {
		return fmt.Errorf("failed to attach to session '%s': %w", sessionName, err)
	}

	// Send local message after session restart
	if message != "" && cfg.GroupID == 0 {
		target, err := tmux.GetWindowTarget(sessionName)
		if err != nil {
			fmt.Printf("Warning: failed to get window for message: %v\n", err)
		} else if err := tmux.SendKeysForBackend(target, message, providerBackend(cfg, providerName)); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	fmt.Printf("Attached to existing session '%s'\n%s\n", sessionName, providerSummary(cfg, sessionInfo))
	return tmux.AttachToSession(sessionName)
}

// StartLocalSession starts a session without Telegram integration (local-only mode).
func StartLocalSession(cfg *configpkg.Config, sessionName, workDir, message string) error {
	providerName := defaultProviderName(cfg)

	if cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*configpkg.SessionInfo)
	}

	cfg.Sessions[sessionName] = &configpkg.SessionInfo{
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := EnsureNewSessionHooks(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
		delete(cfg.Sessions, sessionName)
		return err
	}
	if err := configpkg.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	if message != "" {
		target, err := tmux.GetWindowTarget(sessionName)
		if err != nil {
			fmt.Printf("Warning: failed to get window for message: %v\n", err)
		} else if err := tmux.SendKeysForBackend(target, message, providerBackend(cfg, providerName)); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	fmt.Printf("Started local session '%s' (no Telegram integration)\n%s\n", sessionName, providerSummary(cfg, cfg.Sessions[sessionName]))
	return tmux.AttachToSession(sessionName)
}

// StartTelegramSession starts a session with Telegram integration.
func StartTelegramSession(cfg *configpkg.Config, sessionName, workDir, message string) error {
	providerName := defaultProviderName(cfg)

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
	if err := EnsureNewSessionHooks(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
		delete(cfg.Sessions, sessionName)
		_ = telegram.DeleteForumTopic(cfg, topicID)
		return err
	}
	if err := configpkg.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	pinSessionHeader(cfg, sessionName, cfg.Sessions[sessionName])

	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	fmt.Printf("Created new session '%s'\n%s\n", sessionName, providerSummary(cfg, cfg.Sessions[sessionName]))

	if message != "" {
		if err := telegram.SendMessage(cfg, cfg.GroupID, topicID, message); err != nil {
			fmt.Printf("Warning: failed to send message: %v\n", err)
		}
	}

	return tmux.AttachToSession(sessionName)
}

// SyncSessionInCurrentDir links the current working directory to CCC without
// attaching tmux. This is intended for agent skills invoked inside an existing
// Claude session: it creates or reuses the Telegram topic, installs hooks, and
// leaves the current process in control.
func SyncSessionInCurrentDir(cfg *configpkg.Config, message string) error {
	if cfg == nil {
		return fmt.Errorf("config unavailable. Run: ccc setup <bot_token>")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	if cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*configpkg.SessionInfo)
	}
	runtimeContext := detectCurrentAgentContext(cfg)

	sessionName, sessionInfo := lookup.FindSessionForPath(cfg, cwd)
	created := false
	createdTopic := false
	providerChanged := false
	if sessionName == "" || sessionInfo == nil {
		sessionName = lookup.GenerateUniqueSessionName(cfg, cwd, filepath.Base(cwd))
		sessionInfo = &configpkg.SessionInfo{
			Path:         cwd,
			ProviderName: runtimeContext.ProviderName,
		}
		if sessionInfo.ProviderName == "" {
			sessionInfo.ProviderName = defaultProviderName(cfg)
		}
		if runtimeContext.SessionID != "" {
			sessionInfo.ClaudeSessionID = runtimeContext.SessionID
		}
		cfg.Sessions[sessionName] = sessionInfo
		created = true
	}
	if sessionInfo.Path == "" {
		sessionInfo.Path = cwd
	}
	if runtimeContext.ProviderName != "" && sessionInfo.ProviderName != runtimeContext.ProviderName {
		sessionInfo.ProviderName = runtimeContext.ProviderName
		providerChanged = true
	} else if sessionInfo.ProviderName == "" {
		sessionInfo.ProviderName = defaultProviderName(cfg)
	}
	if runtimeContext.SessionID != "" {
		sessionInfo.ClaudeSessionID = runtimeContext.SessionID
	}

	if cfg.GroupID != 0 && sessionInfo.TopicID == 0 {
		topicID, err := telegram.CreateForumTopic(cfg, sessionName, effectiveProviderName(cfg, sessionInfo), "")
		if err != nil {
			if created {
				delete(cfg.Sessions, sessionName)
			}
			return fmt.Errorf("failed to create topic: %w", err)
		}
		sessionInfo.TopicID = topicID
		createdTopic = true
	}

	if err := EnsureNewSessionHooks(cfg, sessionName, sessionInfo); err != nil {
		if created {
			delete(cfg.Sessions, sessionName)
		}
		if createdTopic {
			_ = telegram.DeleteForumTopic(cfg, sessionInfo.TopicID)
		}
		return err
	}
	if err := configpkg.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	if createdTopic || (providerChanged && sessionInfo.TopicID != 0) {
		pinSessionHeader(cfg, sessionName, sessionInfo)
	}

	if cfg.GroupID != 0 && sessionInfo.TopicID != 0 {
		status := "Synced CCC session"
		if created {
			status = "Created CCC session"
		}
		msg := fmt.Sprintf("%s\nsession: %s\n%s\npath: %s", status, sessionName, providerSummary(cfg, sessionInfo), cwd)
		if strings.TrimSpace(message) != "" {
			msg += "\n\n" + message
		}
		if err := telegram.SendMessage(cfg, cfg.GroupID, sessionInfo.TopicID, msg); err != nil {
			fmt.Printf("Warning: failed to send Telegram sync message: %v\n", err)
		}
	}

	verb := "Synced"
	if created {
		verb = "Created"
	}
	if sessionInfo.TopicID != 0 {
		fmt.Printf("%s CCC session '%s' with Telegram topic %d\n%s\n", verb, sessionName, sessionInfo.TopicID, providerSummary(cfg, sessionInfo))
	} else {
		fmt.Printf("%s local CCC session '%s' (no Telegram group configured)\n%s\n", verb, sessionName, providerSummary(cfg, sessionInfo))
	}
	return nil
}

// StartSession creates/attaches to a tmux window with Telegram topic.
func StartSession(continueSession bool) error {
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

	providerName := defaultProviderName(config)
	if config.Sessions[name] != nil && config.Sessions[name].ProviderName != "" {
		providerName = config.Sessions[name].ProviderName
	}

	resumeSessionID := ""
	if continueSession && config.Sessions[name] != nil {
		resumeSessionID = config.Sessions[name].ClaudeSessionID
	}

	if config.GroupID != 0 && !continueSession {
		if _, exists := config.Sessions[name]; !exists {
			topicID, err := telegram.CreateForumTopic(config, name, providerName, "")
			if err == nil {
				if config.Sessions == nil {
					config.Sessions = make(map[string]*configpkg.SessionInfo)
				}
				config.Sessions[name] = &configpkg.SessionInfo{
					TopicID:      topicID,
					Path:         cwd,
					ProviderName: providerName,
				}
				if err := EnsureNewSessionHooks(config, name, config.Sessions[name]); err != nil {
					delete(config.Sessions, name)
					_ = telegram.DeleteForumTopic(config, topicID)
					return err
				} else {
					configpkg.Save(config)
					pinSessionHeader(config, name, config.Sessions[name])

					fmt.Printf("Created Telegram topic: %s\n%s\n", name, providerSummary(config, config.Sessions[name]))
				}
			}
		}
	}

	if config.Sessions[name] != nil {
		if err := EnsureAgentHooks(config, name, config.Sessions[name]); err != nil {
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

	return tmux.AttachToSession(name)
}

// StartDetachedFromArgs validates CLI args and starts a detached session.
func StartDetachedFromArgs(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("Usage: ccc start <session-name> <work-dir> <prompt>")
	}
	return StartDetached(args[0], args[1], args[2])
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

	providerName := defaultProviderName(config)

	topicID, err := telegram.CreateForumTopic(config, name, providerName, "")
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	config.Sessions[name] = &configpkg.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := EnsureNewSessionHooks(config, name, config.Sessions[name]); err != nil {
		delete(config.Sessions, name)
		_ = telegram.DeleteForumTopic(config, topicID)
		return err
	}
	if err := configpkg.Save(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	pinSessionHeader(config, name, config.Sessions[name])

	if err := tmux.SwitchSessionInWindow(name, workDir, providerName, "", "", false, true); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	target, err := tmux.GetWindowTarget(name)
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

	if err := tmux.WaitForAgentBackend(target, providerBackend(config, providerName), 30*1e9); err != nil { // 30s
		return fmt.Errorf("agent did not start in time: %w", err)
	}

	if err := tmux.SendKeysForBackend(target, prompt, providerBackend(config, providerName)); err != nil {
		return fmt.Errorf("failed to send prompt: %w", err)
	}

	fmt.Printf("Session '%s' started in ccc window with topic %d\n%s\n", name, topicID, providerSummary(config, config.Sessions[name]))
	return nil
}

// StartSessionInCurrentDir starts a session for the current working directory.
func StartSessionInCurrentDir(config *configpkg.Config, message string,
) error {

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	existingSession, existingInfo := lookup.FindSessionForPath(config, cwd)
	if existingSession != "" && existingInfo != nil {
		return AttachToExistingSession(config, existingSession, existingInfo, message)
	}

	basename := filepath.Base(cwd)
	sessionName := lookup.GenerateUniqueSessionName(config, cwd, basename)

	if config.GroupID == 0 {
		return StartLocalSession(config, sessionName, cwd, message)
	}

	return StartTelegramSession(config, sessionName, cwd, message)
}

// StartSessionInCurrentDirAuto loads config and starts a session for the current working directory.
func StartSessionInCurrentDirAuto(message string,
) error {

	config, err := configpkg.Load()
	if err != nil {
		return fmt.Errorf("failed to load config. Run: ccc setup <bot_token>")
	}
	return StartSessionInCurrentDir(config, message)
}

// SyncSessionInCurrentDirAuto loads config and syncs the current directory
// without attaching to tmux.
func SyncSessionInCurrentDirAuto(args []string) error {
	config, err := configpkg.Load()
	if err != nil {
		return fmt.Errorf("failed to load config. Run: ccc setup <bot_token>")
	}
	return SyncSessionInCurrentDir(config, strings.Join(args, " "))
}
