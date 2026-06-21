package listen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/lookup"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// HandleNewCommand handles the /new command - create/restart session
func HandleNewCommand(cfg *configpkg.Config, chatID, threadID int64, text string) {
	fresh, err := configpkg.Load()
	if err != nil || fresh == nil {
		loggingpkg.ListenLog("[/new] failed to load config: %v", err)
		if cfg != nil {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to load config: %v", err))
		}
		return
	}
	cfg = fresh
	ensureNewSessionsMap(cfg)
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/new"))

	if arg != "" {
		HandleNewWithArg(cfg, chatID, threadID, arg)
		return
	}

	// Without args - restart session in current topic
	if threadID > 0 {
		sessionName := lookup.GetSessionByTopic(cfg, threadID)
		if sessionName == "" {
			telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic. Use /new <name> to create one.")
			return
		}
		sessionInfo := cfg.Sessions[sessionName]
		if sessionInfo == nil {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Session '%s' is missing config. Use /new <name> to create it again.", sessionName))
			return
		}
		workDir := lookup.GetSessionWorkDir(cfg, sessionName, sessionInfo)
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			os.MkdirAll(workDir, 0755)
		}
		worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)

		if err := EnsureAgentHooks(cfg, sessionName, sessionInfo); err != nil {
			loggingpkg.ListenLog("[/new] Failed to verify/install hooks for %s: %v", sessionName, err)
		}

		providerName := effectiveProviderName(cfg, sessionInfo)
		if useCodexRelay(cfg, providerName) {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("%s ready via Codex relay\n%s", sessionName, providerSummary(cfg, sessionInfo)))
			return
		}
		if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, resumeSessionID, worktreeName, true, false); err != nil {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
		} else {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("%s restarted\n%s", sessionName, providerSummary(cfg, sessionInfo)))
		}
	} else {
		telegram.SendMessage(cfg, chatID, threadID, "Usage: /new <name> to create a new session")
	}
}

// HandleNewWithArg handles /new <name> with arguments
func HandleNewWithArg(cfg *configpkg.Config, chatID, threadID int64, arg string) {
	if cfg == nil {
		loggingpkg.ListenLog("[/new] ignored request with nil config")
		return
	}
	ensureNewSessionsMap(cfg)

	providerName := ""
	sessionInput := arg
	providerWasExplicit := false

	if strings.Contains(arg, " --provider ") {
		parts := strings.SplitN(arg, " --provider ", 2)
		sessionInput = strings.TrimSpace(parts[0])
		providerName = strings.TrimSpace(parts[1])
		providerWasExplicit = providerName != ""
	} else if !configpkg.IsGitURL(arg) {
		if idx := strings.Index(arg, "@"); idx > 0 {
			sessionInput = arg[:idx]
			providerName = strings.TrimSpace(arg[idx+1:])
			providerWasExplicit = providerName != ""
		}
	}

	gitURL := ""
	sessionName := sessionInput

	if configpkg.IsGitURL(sessionInput) {
		gitURL = sessionInput
		sessionName = configpkg.ExtractRepoName(sessionInput)

		if sessionName == "" {
			telegram.SendMessage(cfg, chatID, threadID, "❌ Invalid git URL: could not extract repository name")
			return
		}

		if providerName != "" {
			provider := providerpkg.GetProvider(cfg, providerName)
			if provider == nil {
				available := providerpkg.GetProviderNames(cfg)
				msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
					providerName, strings.Join(available, ", "))
				telegram.SendMessage(cfg, chatID, threadID, msg)
				return
			}
			if !providerpkg.IsCodexBackend(provider.Backend()) {
				telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Telegram session creation currently supports Codex only. Choose a Codex model/provider instead of '%s'.", providerName))
				return
			}
		}

		existing, exists := cfg.Sessions[sessionName]
		if exists && existing != nil && existing.TopicID != 0 {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
			return
		}

		workDir := filepath.Join(configpkg.GetProjectsDir(cfg), sessionName)
		if exists && existing != nil && existing.Path != "" {
			workDir = existing.Path
		}

		displayURL := configpkg.RedactGitURL(gitURL)
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("📥 Cloning %s into session '%s'...", displayURL, sessionName))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		result, err := configpkg.CloneRepo(ctx, gitURL, workDir)
		cancel()

		if err != nil {
			errMsg := fmt.Sprintf("❌ Failed to clone repository: %v", err)
			if strings.Contains(err.Error(), "directory exists but is not a git repository") {
				errMsg = fmt.Sprintf("⚠️ Directory exists but is not a git repository: %s\n\nPlease remove or rename it and try again.", workDir)
			} else if strings.Contains(err.Error(), "different git repository") {
				errMsg = "⚠️ Directory exists as a different git repository.\n\nPlease remove it or pick a different session name."
			} else if strings.Contains(err.Error(), "no origin remote") {
				errMsg = fmt.Sprintf("⚠️ Directory is a git repository but has no origin remote: %s\n\nPlease remove or use a different session name.", workDir)
			} else if strings.Contains(err.Error(), "context deadline exceeded") {
				errMsg = fmt.Sprintf("⏱️ Cloning timed out after 5 minutes. The repository may be very large or the network may be slow.")
			}
			telegram.SendMessage(cfg, chatID, threadID, errMsg)
			return
		}

		if result == configpkg.CloneResultCloned {
			telegram.SendMessage(cfg, chatID, threadID, "✅ Repository cloned")
		} else if result == configpkg.CloneResultAlreadyExists {
			telegram.SendMessage(cfg, chatID, threadID, "✅ Repository ready (using existing clone)")
		}

	}

	if providerName != "" {
		provider := providerpkg.GetProvider(cfg, providerName)
		if provider == nil {
			available := providerpkg.GetProviderNames(cfg)
			msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
				providerName, strings.Join(available, ", "))
			telegram.SendMessage(cfg, chatID, threadID, msg)
			return
		}
		if !providerpkg.IsCodexBackend(provider.Backend()) {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Telegram session creation currently supports Codex only. Choose a Codex model/provider instead of '%s'.", providerName))
			return
		}
	}

	if providerName == "" {
		existing, exists := cfg.Sessions[sessionName]
		if exists && existing != nil && existing.TopicID != 0 {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
			return
		}

		sendNewProviderSelection(cfg, chatID, threadID, 0, sessionName, "codex")
		return
	}

	existing, exists := cfg.Sessions[sessionName]
	if exists && existing != nil && existing.TopicID != 0 {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
		return
	}
	if providerName == "" {
		providerName = "codex"
	}
	topicID, err := telegram.CreateForumTopic(cfg, sessionName, providerName, "")
	if err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
		return
	}
	workDir := configpkg.ResolveProjectPath(cfg, sessionName)
	if exists && existing != nil && existing.Path != "" {
		workDir = existing.Path
	}
	previous, hadPrevious := cfg.Sessions[sessionName]
	cfg.Sessions[sessionName] = &configpkg.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		if hadPrevious {
			cfg.Sessions[sessionName] = previous
		} else {
			delete(cfg.Sessions, sessionName)
		}
		_ = telegram.DeleteForumTopic(cfg, topicID)
		loggingpkg.ListenLog("[/new] Failed to create workdir for %s: %v", sessionName, err)
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to create workdir: %v", err))
		return
	}
	if err := EnsureNewSessionHooks(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
		if hadPrevious {
			cfg.Sessions[sessionName] = previous
		} else {
			delete(cfg.Sessions, sessionName)
		}
		_ = telegram.DeleteForumTopic(cfg, topicID)
		loggingpkg.ListenLog("[/new] Failed to install/trust hooks for %s: %v", sessionName, err)
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to install/trust hooks: %v", err))
		return
	}
	if err := configpkg.Save(cfg); err != nil {
		if hadPrevious {
			cfg.Sessions[sessionName] = previous
		} else {
			delete(cfg.Sessions, sessionName)
		}
		if deleteErr := telegram.DeleteForumTopic(cfg, topicID); deleteErr != nil {
			loggingpkg.ListenLog("[/new] failed to delete topic after save failure for %s: %v", sessionName, deleteErr)
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Failed to save config: %v\nAlso failed to clean up the new Telegram topic: %v\nA topic may have been orphaned; delete it manually or retry /new.", err, deleteErr))
			return
		}
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Failed to save config: %v", err))
		return
	}
	pinSessionHeader(cfg, sessionName, cfg.Sessions[sessionName])

	providerMsg := activeDefaultProviderSummary(cfg)
	if providerWasExplicit {
		providerMsg = explicitProviderSummary(providerName)
	}
	if useCodexRelay(cfg, providerName) {
		telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("%s ready via Codex relay\n%s\n\nSend messages here to interact with %s.", sessionName, providerMsg, agentDisplayName(cfg, providerName)))
	} else if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("❌ Failed to start session: %v", err))
	} else {
		telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("%s started\n%s\n\nSend messages here to interact with %s.", sessionName, providerMsg, agentDisplayName(cfg, providerName)))
	}
}

func sendNewProviderSelection(cfg *configpkg.Config, chatID, threadID int64, messageID int64, sessionName, agentName string) {
	if agentName == "" || agentName == "claude" {
		agentName = "codex"
	}
	buttons := newProviderButtonsForAgent(cfg, sessionName, agentName)
	if len(buttons) == 0 {
		msg := fmt.Sprintf("❌ No %s provider/model is configured.", agentOptionLabel(cfg, agentName))
		if messageID != 0 {
			if err := telegram.EditMessageRemoveKeyboard(cfg, chatID, messageID, msg); err != nil {
				loggingpkg.ListenLog("[/new] failed to edit empty provider selection for %s: %v", sessionName, err)
			}
		} else {
			telegram.SendMessage(cfg, chatID, threadID, msg)
		}
		return
	}

	msg := fmt.Sprintf("Create Codex session\nsession: %s\n\nChoose model:", sessionName)
	if messageID != 0 {
		if err := telegram.EditMessageWithKeyboard(cfg, chatID, messageID, msg, buttons); err != nil {
			loggingpkg.ListenLog("[/new] failed to edit provider selection for %s: %v", sessionName, err)
		}
		return
	}
	if err := telegram.SendMessageWithKeyboard(cfg, chatID, threadID, msg, buttons); err != nil {
		loggingpkg.ListenLog("[/new] failed to send provider selection for %s: %v", sessionName, err)
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to show provider choices: %v", err))
	}
}

func ensureNewSessionsMap(cfg *configpkg.Config) {
	if cfg != nil && cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*configpkg.SessionInfo)
	}
}

func newProviderButtonsForAgent(cfg *configpkg.Config, sessionName, agentName string) [][]telegram.InlineKeyboardButton {
	var buttons [][]telegram.InlineKeyboardButton
	selectedName := selectedNewProviderNameForAgent(cfg, agentName)
	for _, name := range providerNamesForAgent(cfg, agentName) {
		label := providerModelOptionLabel(cfg, name)
		if name == selectedName {
			label += " ⭐"
		}
		callbackData := newSessionCallbackData(newSessionCallback{
			Action:       "provider",
			SessionName:  sessionName,
			AgentName:    agentName,
			ProviderName: name,
		})
		if callbackData == "" {
			continue
		}
		buttons = append(buttons, []telegram.InlineKeyboardButton{
			{Text: label, CallbackData: callbackData},
		})
	}
	return buttons
}

func selectedNewProviderNameForAgent(cfg *configpkg.Config, agentName string) string {
	if agentName != "codex" {
		if cfg != nil && cfg.ActiveProvider != "" {
			return cfg.ActiveProvider
		}
		return builtinProviderName
	}
	if cfg == nil || cfg.ActiveProvider == "" {
		return "codex"
	}
	provider := providerpkg.GetProvider(cfg, cfg.ActiveProvider)
	if provider != nil && providerpkg.IsCodexBackend(provider.Backend()) {
		return provider.Name()
	}
	return "codex"
}

func providerNamesForAgent(cfg *configpkg.Config, agentName string) []string {
	if agentName != "claude" && agentName != "codex" {
		return nil
	}
	var names []string
	for _, name := range providerpkg.GetProviderNames(cfg) {
		provider := providerpkg.GetProvider(cfg, name)
		if provider == nil {
			continue
		}
		if agentName == "codex" && provider.Backend() != providerpkg.BackendCodex {
			continue
		}
		if agentName == "claude" && provider.Backend() != providerpkg.BackendClaude {
			continue
		}
		names = append(names, name)
	}
	return names
}
