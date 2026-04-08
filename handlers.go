package main

import (
	"fmt"
	"os"
	"os/exec"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"

	"github.com/tuannvm/ccc/session"
)

// Callback handlers for inline keyboard interactions.

// handleNewWithProvider creates a session after provider selection via inline keyboard
func handleNewWithProvider(config *Config, cb *CallbackQuery, sessionName, providerName string) {
	existing, exists := config.Sessions[sessionName]
	if exists && existing != nil && existing.TopicID != 0 {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("⚠️ Session '%s' already exists.\n\nUse /new without args in that topic to restart.", sessionName))
		}
		return
	}

	topicID, err := telegram.CreateForumTopic(config, sessionName, providerName, "")
	if err != nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create topic: %v", err))
		}
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

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	resultMsg := fmt.Sprintf("🚀 Session '%s' started!\n🤖 Provider: %s\n\nSend messages here to interact with Claude.", sessionName, providerName)
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		resultMsg = fmt.Sprintf("❌ Failed to start session: %v", err)
	}

	if cb.Message != nil {
		telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

// handleProviderChange changes provider for an existing session via inline keyboard
func handleProviderChange(config *Config, cb *CallbackQuery, sessionName, providerName string) {
	sess, exists := config.Sessions[sessionName]
	if !exists || sess == nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Session '%s' not found.", sessionName))
		}
		return
	}

	provider := providerpkg.GetProvider(config, providerName)
	if provider == nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Provider '%s' not found. Available: %v", providerName, providerpkg.GetProviderNames(config)))
		}
		return
	}

	sess.ProviderName = providerName
	configpkg.Save(config)

	resultMsg := fmt.Sprintf("✅ Provider changed to %s for session '%s'\n\nRestart with /new to apply the new provider.", provider.Name(), sessionName)
	if cb.Message != nil {
		telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

// handleTeamWithProvider creates a team session after provider selection via inline keyboard
func handleTeamWithProvider(config *Config, cb *CallbackQuery, teamName, providerName string) {
	for topicID, sessInfo := range config.TeamSessions {
		if sessInfo != nil {
			sessName := getSessionNameFromInfo(sessInfo)
			if sessName == teamName {
				if cb.Message != nil {
					telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
						fmt.Sprintf("⚠️ Team session '%s' already exists (topic: %d).\n\nUse /team without args in that topic to restart.", teamName, topicID))
				}
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
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				"❌ Team runtime not available. Check your installation.")
		}
		return
	}

	if err := runtime.EnsureLayout(sessInfo, workDir); err != nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create team layout: %v", err))
		}
		return
	}

	topicID, err := telegram.CreateForumTopic(config, teamName, providerName, "")
	if err != nil {
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+teamName).Run()
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create topic: %v", err))
		}
		return
	}

	sessInfo.TopicID = topicID
	config.SetTeamSession(topicID, sessInfo)
	if err := configpkg.Save(config); err != nil {
		telegram.DeleteForumTopic(config, topicID)
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+teamName).Run()
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to save config: %v", err))
		}
		return
	}

	if err := ensureHooksForSession(config, teamName, sessInfo); err != nil {
		listenLog("[/team] Failed to install hooks for %s: %v", teamName, err)
	}

	if err := runtime.StartClaude(sessInfo, workDir); err != nil {
		listenLog("[/team] Failed to start Claude for %s: %v", teamName, err)
	}

	resultMsg := fmt.Sprintf("✅ Team session '%s' created!\n\n", teamName)
	resultMsg += fmt.Sprintf("📂 Path: %s\n", workDir)
	resultMsg += fmt.Sprintf("🤖 Provider: %s\n", providerName)
	resultMsg += fmt.Sprintf("💬 Topic ID: %d\n\n", topicID)
	resultMsg += "📱 Send messages:\n"
	resultMsg += "  /planner <msg>   - Send to planner\n"
	resultMsg += "  /executor <msg>  - Send to executor\n"
	resultMsg += "  /reviewer <msg>  - Send to reviewer\n"
	resultMsg += "  <msg> (no cmd)   - Send to executor (default)"

	if cb.Message != nil {
		telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}
