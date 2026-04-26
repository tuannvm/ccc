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
	cfg, _ = configpkg.Load()
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
		workDir := lookup.GetSessionWorkDir(cfg, sessionName, sessionInfo)
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			os.MkdirAll(workDir, 0755)
		}
		worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)

		if err := EnsureHooks(cfg, sessionName, sessionInfo); err != nil {
			loggingpkg.ListenLog("[/new] Failed to verify/install hooks for %s: %v", sessionName, err)
		}

		providerName := effectiveProviderName(cfg, sessionInfo)
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
	}

	if providerName == "" {
		existing, exists := cfg.Sessions[sessionName]
		if exists && existing != nil && existing.TopicID != 0 {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
			return
		}

		var buttons [][]telegram.InlineKeyboardButton
		providerNames := providerpkg.GetProviderNames(cfg)
		for _, name := range providerNames {
			label := name
			if cfg.ActiveProvider == name {
				label += " ⭐"
			}
			callbackData := fmt.Sprintf("new:%s:%s", sessionName, name)
			buttons = append(buttons, []telegram.InlineKeyboardButton{
				{Text: label, CallbackData: callbackData},
			})
		}

		msg := fmt.Sprintf("🤖 Select provider for '%s':", sessionName)
		telegram.SendMessageWithKeyboard(cfg, chatID, threadID, msg, buttons)
		return
	}

	existing, exists := cfg.Sessions[sessionName]
	if exists && existing != nil && existing.TopicID != 0 {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
		return
	}
	if providerName == "" {
		providerName = defaultProviderName(cfg)
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
	cfg.Sessions[sessionName] = &configpkg.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	configpkg.Save(cfg)
	pinSessionHeader(cfg, sessionName, cfg.Sessions[sessionName])

	if err := EnsureHooks(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
		loggingpkg.ListenLog("[/new] Failed to install hooks for %s: %v", sessionName, err)
	}

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}
	providerMsg := activeDefaultProviderSummary(cfg)
	if providerWasExplicit {
		providerMsg = explicitProviderSummary(providerName)
	}
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("❌ Failed to start session: %v", err))
	} else {
		telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("%s started\n%s\n\nSend messages here to interact with Claude.", sessionName, providerMsg))
	}
}
