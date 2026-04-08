package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// handleNewCommand handles the /new command - create/restart session
func handleNewCommand(config *Config, chatID, threadID int64, text string) {
	config, _ = configpkg.Load()
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/new"))

	if arg != "" {
		handleNewWithArg(config, chatID, threadID, arg)
		return
	}

	// Without args - restart session in current topic
	if threadID > 0 {
		sessionName := getSessionByTopic(config, threadID)
		if sessionName == "" {
			telegram.SendMessage(config, chatID, threadID, "❌ No session mapped to this topic. Use /new <name> to create one.")
			return
		}
		sessionInfo := config.Sessions[sessionName]
		workDir := getSessionWorkDir(config, sessionName, sessionInfo)
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			os.MkdirAll(workDir, 0755)
		}
		worktreeName := ""
		if sessionInfo.IsWorktree {
			worktreeName = sessionInfo.WorktreeName
		}
		resumeSessionID := sessionInfo.ClaudeSessionID

		if err := ensureHooksForSession(config, sessionName, sessionInfo); err != nil {
			listenLog("[/new] Failed to verify/install hooks for %s: %v", sessionName, err)
		}

		if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
		} else {
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("🚀 Session '%s' restarted", sessionName))
		}
	} else {
		telegram.SendMessage(config, chatID, threadID, "Usage: /new <name> to create a new session")
	}
}

// handleNewWithArg handles /new <name> with arguments
func handleNewWithArg(config *Config, chatID, threadID int64, arg string) {
	providerName := ""
	sessionInput := arg
	gitURLHandled := false

	if strings.Contains(arg, " --provider ") {
		parts := strings.SplitN(arg, " --provider ", 2)
		sessionInput = strings.TrimSpace(parts[0])
		providerName = strings.TrimSpace(parts[1])
	} else if !isGitURL(arg) {
		if idx := strings.Index(arg, "@"); idx > 0 {
			sessionInput = arg[:idx]
			providerName = strings.TrimSpace(arg[idx+1:])
		}
	}

	gitURL := ""
	sessionName := sessionInput

	if isGitURL(sessionInput) {
		gitURL = sessionInput
		sessionName = extractRepoName(sessionInput)

		if sessionName == "" {
			telegram.SendMessage(config, chatID, threadID, "❌ Invalid git URL: could not extract repository name")
			return
		}

		if providerName != "" {
			provider := getProvider(config, providerName)
			if provider == nil {
				available := getProviderNames(config)
				msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
					providerName, strings.Join(available, ", "))
				telegram.SendMessage(config, chatID, threadID, msg)
				return
			}
		}

		existing, exists := config.Sessions[sessionName]
		if exists && existing != nil && existing.TopicID != 0 {
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
			return
		}

		workDir := filepath.Join(configpkg.GetProjectsDir(config), sessionName)
		if exists && existing != nil && existing.Path != "" {
			workDir = existing.Path
		}

		displayURL := redactGitURL(gitURL)
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("📥 Cloning %s into session '%s'...", displayURL, sessionName))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		result, err := cloneRepo(ctx, gitURL, workDir)
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
			telegram.SendMessage(config, chatID, threadID, errMsg)
			return
		}

		if result == CloneResultCloned {
			telegram.SendMessage(config, chatID, threadID, "✅ Repository cloned")
		} else if result == CloneResultAlreadyExists {
			telegram.SendMessage(config, chatID, threadID, "✅ Repository ready (using existing clone)")
		}

		gitURLHandled = true
	}

	if providerName != "" {
		provider := getProvider(config, providerName)
		if provider == nil {
			available := getProviderNames(config)
			msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
				providerName, strings.Join(available, ", "))
			telegram.SendMessage(config, chatID, threadID, msg)
			return
		}
	}

	if providerName == "" && !gitURLHandled {
		existing, exists := config.Sessions[sessionName]
		if exists && existing != nil && existing.TopicID != 0 {
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
			return
		}

		var buttons [][]InlineKeyboardButton
		providerNames := getProviderNames(config)
		for _, name := range providerNames {
			label := name
			if config.ActiveProvider == name {
				label += " ⭐"
			}
			callbackData := fmt.Sprintf("new:%s:%s", sessionName, name)
			buttons = append(buttons, []InlineKeyboardButton{
				{Text: label, CallbackData: callbackData},
			})
		}

		msg := fmt.Sprintf("🤖 Select provider for '%s':", sessionName)
		telegram.SendMessageWithKeyboard(config, chatID, threadID, msg, buttons)
		return
	}

	existing, exists := config.Sessions[sessionName]
	if exists && existing != nil && existing.TopicID != 0 {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
		return
	}
	if providerName == "" {
		providerName = config.ActiveProvider
	}
	topicID, err := telegram.CreateForumTopic(config, sessionName, providerName, "")
	if err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
		return
	}
	workDir := configpkg.ResolveProjectPath(config, sessionName)
	if exists && existing != nil && existing.Path != "" {
		workDir = existing.Path
	}
	config.Sessions[sessionName] = &SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	configpkg.Save(config)

	if err := ensureHooksForSession(config, sessionName, config.Sessions[sessionName]); err != nil {
		listenLog("[/new] Failed to install hooks for %s: %v", sessionName, err)
	}

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}
	providerMsg := ""
	if providerName != "" {
		providerMsg = fmt.Sprintf("\n🤖 Provider: %s", providerName)
	}
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		telegram.SendMessage(config, config.GroupID, topicID, fmt.Sprintf("❌ Failed to start session: %v", err))
	} else {
		telegram.SendMessage(config, config.GroupID, topicID, fmt.Sprintf("🚀 Session '%s' started!%s\n\nSend messages here to interact with Claude.", sessionName, providerMsg))
	}
}
