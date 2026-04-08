package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"

	"github.com/tuannvm/ccc/session"
)

// handleTeamCreateCommand handles the /team command - create team session
func handleTeamCreateCommand(config *Config, chatID, threadID int64, text string) {
	config, _ = configpkg.Load()
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/team"))

	// /team <name>[@provider] - create brand new team session + topic
	if arg != "" {
		teamName, providerName := parseNameAndProvider(arg)

		if providerName != "" {
			provider := providerpkg.GetProvider(config, providerName)
			if provider == nil {
				available := providerpkg.GetProviderNames(config)
				msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
					providerName, strings.Join(available, ", "))
				telegram.SendMessage(config, chatID, threadID, msg)
				return
			}
		}

		if providerName == "" {
			// Check if team session already exists
			for topicID, sessInfo := range config.TeamSessions {
				if sessInfo != nil {
					sessName := getSessionNameFromInfo(sessInfo)
					if sessName == teamName {
						telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Team session '%s' already exists (topic: %d). Use /team without args in that topic to restart.", teamName, topicID))
						return
					}
				}
			}

			var buttons [][]InlineKeyboardButton
			providerNames := providerpkg.GetProviderNames(config)
			for _, name := range providerNames {
				label := name
				if config.ActiveProvider == name {
					label += " ⭐"
				}
				callbackData := fmt.Sprintf("team:%s:%s", teamName, name)
				buttons = append(buttons, []InlineKeyboardButton{
					{Text: label, CallbackData: callbackData},
				})
			}

			msg := fmt.Sprintf("🤖 Select provider for team '%s':", teamName)
			telegram.SendMessageWithKeyboard(config, chatID, threadID, msg, buttons)
			return
		}

		createTeamSession(config, chatID, threadID, teamName, providerName)
		return
	}

	// /team without args - restart in current topic
	if config.IsTeamSession(threadID) {
		sessInfo, exists := config.GetTeamSession(threadID)
		if !exists || sessInfo == nil {
			telegram.SendMessage(config, chatID, threadID, "❌ Team session not found. Use /team <name> to create one.")
			return
		}

		runtime := session.GetRuntime(session.SessionKindTeam)
		if runtime == nil {
			telegram.SendMessage(config, chatID, threadID, "❌ Team runtime not available.")
			return
		}

		if err := runtime.StartClaude(sessInfo, sessInfo.Path); err != nil {
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to start Claude: %v", err))
			return
		}

		telegram.SendMessage(config, chatID, threadID, "✅ Team session restarted!")
		return
	}

	telegram.SendMessage(config, chatID, threadID, "❌ No team session linked to this topic. Use /team <name> to create one.")
}

// createTeamSession creates a new team session with the given provider
func createTeamSession(config *Config, chatID, threadID int64, teamName, providerName string) {
	// Check if team session already exists
	for topicID, sessInfo := range config.TeamSessions {
		if sessInfo != nil {
			sessName := getSessionNameFromInfo(sessInfo)
			if sessName == teamName {
				telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Team session '%s' already exists (topic: %d). Use /team without args in that topic to restart.", teamName, topicID))
				return
			}
		}
	}

	workDir := configpkg.ResolveProjectPath(config, teamName)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	sessInfo := &SessionInfo{
		Path:         workDir,
		SessionName:  teamName,
		ProviderName: providerName,
		Type:         session.SessionKindTeam,
		LayoutName:   "team-3pane",
		Panes:        make(map[session.PaneRole]*PaneInfo),
	}
	sessInfo.Panes[session.RolePlanner] = &PaneInfo{Role: session.RolePlanner}
	sessInfo.Panes[session.RoleExecutor] = &PaneInfo{Role: session.RoleExecutor}
	sessInfo.Panes[session.RoleReviewer] = &PaneInfo{Role: session.RoleReviewer}

	runtime := session.GetRuntime(session.SessionKindTeam)
	if runtime == nil {
		telegram.SendMessage(config, chatID, threadID, "❌ Team runtime not available. Check your installation.")
		return
	}

	if err := runtime.EnsureLayout(sessInfo, workDir); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create team layout: %v", err))
		return
	}

	topicID, err := telegram.CreateForumTopic(config, teamName, providerName, "")
	if err != nil {
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+teamName).Run()
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
		return
	}

	sessInfo.TopicID = topicID
	config.SetTeamSession(topicID, sessInfo)
	if err := configpkg.Save(config); err != nil {
		telegram.DeleteForumTopic(config, topicID)
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+teamName).Run()
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to save config: %v", err))
		return
	}

	if err := ensureHooksForSession(config, teamName, sessInfo); err != nil {
		listenLog("[/team] Failed to install hooks for %s: %v", teamName, err)
	}

	if err := runtime.StartClaude(sessInfo, workDir); err != nil {
		listenLog("[/team] Failed to start Claude for %s: %v", teamName, err)
	}

	msg := fmt.Sprintf("✅ Team session '%s' created!\n\n", teamName)
	msg += fmt.Sprintf("📂 Path: %s\n", workDir)
	msg += fmt.Sprintf("🤖 Provider: %s\n", providerName)
	msg += fmt.Sprintf("💬 Topic ID: %d\n\n", topicID)
	msg += "📱 Send messages:\n"
	msg += "  /planner <msg>   - Send to planner\n"
	msg += "  /executor <msg>  - Send to executor\n"
	msg += "  /reviewer <msg>  - Send to reviewer\n"
	msg += "  <msg> (no cmd)   - Send to executor (default)"
	telegram.SendMessage(config, chatID, threadID, msg)
}
