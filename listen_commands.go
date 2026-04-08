package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/telegram"

	"github.com/tuannvm/ccc/pkg/tmux"
)

// Listen command handlers — extracted from the listen() loop for readability.
// Each function handles a specific Telegram bot command received during polling.

// handleContinueCommand handles the /continue command - restart session preserving conversation history
func handleContinueCommand(config *Config, chatID, threadID int64) {
	config, _ = configpkg.Load()
	sessName := getSessionByTopic(config, threadID)
	if sessName == "" {
		telegram.SendMessage(config, chatID, threadID, "❌ No session mapped to this topic. Use /new <name> to create one.")
		return
	}
	sessionInfo := config.Sessions[sessName]
	workDir := getSessionWorkDir(config, sessName, sessionInfo)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}
	worktreeName := ""
	if sessionInfo.IsWorktree {
		worktreeName = sessionInfo.WorktreeName
	}
	resumeSessionID := sessionInfo.ClaudeSessionID

	if err := tmux.SwitchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
	} else {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("🔄 Session '%s' restarted with conversation history", sessName))
	}
}

// handleDeleteCommand handles the /delete command - delete session and thread
func handleDeleteCommand(config *Config, chatID, threadID int64) {
	config, _ = configpkg.Load()
	sessName := getSessionByTopic(config, threadID)
	if sessName == "" {
		telegram.SendMessage(config, chatID, threadID, "❌ No session mapped to this topic.")
		return
	}

	// Check if this is the currently active session and stop Claude if so
	target, err := tmux.FindExistingWindow(sessName)
	if err == nil {
		cmd := exec.Command(tmux.TmuxPath, "display-message", "-t", target, "-p", "#{window_name}")
		out, err := cmd.Output()
		if err == nil {
			currentWindowName := strings.TrimSpace(string(out))
			expectedName := tmux.SafeName(sessName)
			if currentWindowName == expectedName {
				exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "C-c").Run()
				time.Sleep(100 * time.Millisecond)
				exec.Command(tmux.TmuxPath, "kill-window", "-t", target).Run()
			}
		}
	}

	topicID := config.Sessions[sessName].TopicID
	delete(config.Sessions, sessName)
	configpkg.Save(config)
	if err := telegram.DeleteForumTopic(config, topicID); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Session deleted but failed to delete thread: %v", err))
	}
}

// handleProvidersCommand handles the /providers and /provider commands
func handleProvidersCommand(config *Config, chatID, threadID int64, text string, isGroup bool) {
	config, _ = configpkg.Load()

	if isGroup && threadID > 0 {
		sessName := getSessionByTopic(config, threadID)
		if sessName != "" {
			sessionInfo := config.Sessions[sessName]
			current := sessionInfo.ProviderName
			if current == "" {
				current = config.ActiveProvider
				if current == "" {
					current = "anthropic"
				}
			}

			var buttons [][]InlineKeyboardButton
			providerNames := getProviderNames(config)
			for _, name := range providerNames {
				label := name
				if current == name || (sessionInfo.ProviderName == "" && config.ActiveProvider == name) {
					label += " ✓"
				}
				callbackData := fmt.Sprintf("provider:%s:%s", sessName, name)
				buttons = append(buttons, []InlineKeyboardButton{
					{Text: label, CallbackData: callbackData},
				})
			}

			msg := fmt.Sprintf("🤖 **%s**\n\nCurrent provider: %s\n\nSelect a new provider:", sessName, current)
			telegram.SendMessageWithKeyboard(config, chatID, threadID, msg, buttons)
			return
		}
	}

	// Not in a topic - show all available providers
	var msg []string
	msg = append(msg, "📋 Available providers:")
	providerNames := getProviderNames(config)
	for _, name := range providerNames {
		active := ""
		if config.ActiveProvider == name || (config.ActiveProvider == "" && name == "anthropic") {
			active = " (active)"
		}
		if name == "anthropic" {
			msg = append(msg, fmt.Sprintf("  • %s%s (built-in, uses default env vars)", name, active))
		} else {
			msg = append(msg, fmt.Sprintf("  • %s%s", name, active))
		}
	}
	if len(msg) == 1 {
		msg = append(msg, "\nNo additional providers configured.\n\nConfigure providers in ~/.config/ccc/config.json.")
	}
	telegram.SendMessage(config, chatID, threadID, strings.Join(msg, "\n"))
}

// handleCleanupCommand handles the /cleanup command - delete tmux sessions and Telegram topics
func handleCleanupCommand(config *Config, chatID, threadID int64) {
	config, _ = configpkg.Load()
	if len(config.Sessions) == 0 {
		telegram.SendMessage(config, chatID, threadID, "No sessions to clean up.")
		return
	}

	var cleaned []string
	var errors []string

	for sessName, info := range config.Sessions {
		if target, err := tmux.FindExistingWindow(sessName); err == nil && target != "" {
			exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "C-c").Run()
			time.Sleep(100 * time.Millisecond)
			exec.Command(tmux.TmuxPath, "kill-window", "-t", target).Run()
		}

		if info.TopicID > 0 && config.GroupID > 0 {
			if err := telegram.DeleteForumTopic(config, info.TopicID); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", sessName, err))
			}
		}

		cleaned = append(cleaned, sessName)
	}

	config.Sessions = make(map[string]*SessionInfo)
	configpkg.Save(config)

	msg := fmt.Sprintf("🧹 Cleaned %d sessions: %s", len(cleaned), strings.Join(cleaned, ", "))
	if len(errors) > 0 {
		msg += fmt.Sprintf("\n\n⚠️ Errors:\n%s", strings.Join(errors, "\n"))
	}
	telegram.SendMessage(config, chatID, threadID, msg)
}

// parseNameAndProvider parses "name@provider" or "name --provider provider" syntax
func parseNameAndProvider(arg string) (name, provider string) {
	if strings.Contains(arg, " --provider ") {
		parts := strings.SplitN(arg, " --provider ", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if !isGitURL(arg) {
		if idx := strings.Index(arg, "@"); idx > 0 {
			return arg[:idx], strings.TrimSpace(arg[idx+1:])
		}
	}
	return arg, ""
}

// resolveTranscriptDir finds the transcript directory for a session
func resolveTranscriptDir(config *Config, sessionInfo *SessionInfo, home string, pathComponent string) string {
	providerName := sessionInfo.ProviderName
	if providerName == "" {
		providerName = config.ActiveProvider
	}
	var transcriptDir string
	if providerName != "" && config.Providers != nil {
		if p := config.Providers[providerName]; p != nil && p.ConfigDir != "" {
			configDir := p.ConfigDir
			if strings.HasPrefix(configDir, "~/") {
				configDir = filepath.Join(home, configDir[2:])
			} else if configDir == "~" {
				configDir = home
			}
			transcriptDir = filepath.Join(configDir, "projects", pathComponent)
		}
	}
	if transcriptDir == "" {
		transcriptDir = filepath.Join(home, ".claude", "projects", pathComponent)
	}
	return transcriptDir
}
