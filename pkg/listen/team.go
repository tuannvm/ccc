package listen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"

	"github.com/tuannvm/ccc/pkg/routing"
	"github.com/tuannvm/ccc/pkg/session"
)

// HandleTeamCreateCommand handles the /team command - create team session
func HandleTeamCreateCommand(cfg *configpkg.Config, chatID, threadID int64, text string) {
	cfg, _ = configpkg.Load()
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/team"))

	if arg != "" {
		teamName, providerName := configpkg.ParseNameAndProvider(arg)

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
			for topicID, sessInfo := range cfg.TeamSessions {
				if sessInfo != nil {
					sessName := getSessionNameFromInfo(sessInfo)
					if sessName == teamName {
						telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Team session '%s' already exists (topic: %d). Use /team without args in that topic to restart.", teamName, topicID))
						return
					}
				}
			}

			var buttons [][]telegram.InlineKeyboardButton
			providerNames := providerpkg.GetProviderNames(cfg)
			for _, name := range providerNames {
				label := name
				if cfg.ActiveProvider == name {
					label += " ⭐"
				}
				callbackData := fmt.Sprintf("team:%s:%s", teamName, name)
				buttons = append(buttons, []telegram.InlineKeyboardButton{
					{Text: label, CallbackData: callbackData},
				})
			}

			msg := fmt.Sprintf("🤖 Select provider for team '%s':", teamName)
			telegram.SendMessageWithKeyboard(cfg, chatID, threadID, msg, buttons)
			return
		}

		CreateTeamSession(cfg, chatID, threadID, teamName, providerName)
		return
	}

	// /team without args - restart in current topic
	if cfg.IsTeamSession(threadID) {
		sessInfo, exists := cfg.GetTeamSession(threadID)
		if !exists || sessInfo == nil {
			telegram.SendMessage(cfg, chatID, threadID, "❌ Team session not found. Use /team <name> to create one.")
			return
		}

		runtime := session.GetRuntime(session.SessionKindTeam)
		if runtime == nil {
			telegram.SendMessage(cfg, chatID, threadID, "❌ Team runtime not available.")
			return
		}

		if err := runtime.StartClaude(sessInfo, sessInfo.Path); err != nil {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to start Claude: %v", err))
			return
		}

		telegram.SendMessage(cfg, chatID, threadID, "✅ Team session restarted!")
		return
	}

	telegram.SendMessage(cfg, chatID, threadID, "❌ No team session linked to this topic. Use /team <name> to create one.")
}

// CreateTeamSession creates a new team session with the given provider
func CreateTeamSession(cfg *configpkg.Config, chatID, threadID int64, teamName, providerName string) {
	for topicID, sessInfo := range cfg.TeamSessions {
		if sessInfo != nil {
			sessName := getSessionNameFromInfo(sessInfo)
			if sessName == teamName {
				telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Team session '%s' already exists (topic: %d). Use /team without args in that topic to restart.", teamName, topicID))
				return
			}
		}
	}

	workDir := configpkg.ResolveProjectPath(cfg, teamName)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	sessInfo := &configpkg.SessionInfo{
		Path:         workDir,
		SessionName:  teamName,
		ProviderName: providerName,
		Type:         session.SessionKindTeam,
		LayoutName:   "team-3pane",
		Panes:        make(map[session.PaneRole]*configpkg.PaneInfo),
	}
	sessInfo.Panes[session.RolePlanner] = &configpkg.PaneInfo{Role: session.RolePlanner}
	sessInfo.Panes[session.RoleExecutor] = &configpkg.PaneInfo{Role: session.RoleExecutor}
	sessInfo.Panes[session.RoleReviewer] = &configpkg.PaneInfo{Role: session.RoleReviewer}

	runtime := session.GetRuntime(session.SessionKindTeam)
	if runtime == nil {
		telegram.SendMessage(cfg, chatID, threadID, "❌ Team runtime not available. Check your installation.")
		return
	}

	if err := runtime.EnsureLayout(sessInfo, workDir); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to create team layout: %v", err))
		return
	}

	topicID, err := telegram.CreateForumTopic(cfg, teamName, providerName, "")
	if err != nil {
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+teamName).Run()
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
		return
	}

	sessInfo.TopicID = topicID
	cfg.SetTeamSession(topicID, sessInfo)
	if err := configpkg.Save(cfg); err != nil {
		telegram.DeleteForumTopic(cfg, topicID)
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+teamName).Run()
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to save config: %v", err))
		return
	}

	if err := EnsureHooks(cfg, teamName, sessInfo); err != nil {
		loggingpkg.ListenLog("[/team] Failed to install hooks for %s: %v", teamName, err)
	}

	if err := runtime.StartClaude(sessInfo, workDir); err != nil {
		loggingpkg.ListenLog("[/team] Failed to start Claude for %s: %v", teamName, err)
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
	telegram.SendMessage(cfg, chatID, threadID, msg)
}

// HandleTeamSessionMessage routes a Telegram message to the appropriate pane in a team session
func HandleTeamSessionMessage(cfg *configpkg.Config, text string, topicID int64, chatID int64, threadID int64) bool {
	if !cfg.IsTeamSession(topicID) {
		return false
	}

	sessInfo, exists := cfg.GetTeamSession(topicID)
	if !exists || sessInfo == nil {
		telegram.SendMessage(cfg, chatID, threadID, "❌ Team session not found. Use /team new to create one.")
		return true
	}

	layout, ok := session.GetLayout(sessInfo.GetLayoutName())
	if !ok {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Unknown layout: %s", sessInfo.GetLayoutName()))
		return true
	}

	router := routing.GetRouter(sessInfo.GetType())
	role, messageText, err := router.RouteMessage(text, layout)
	if err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Routing error: %v", err))
		return true
	}

	sessionName := getSessionNameFromInfo(sessInfo)
	target, err := tmux.GetTeamRoleTarget(sessionName, role)
	if err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to get pane target: %v", err))
		return true
	}

	currentSession := tmux.GetCurrentSessionName()
	needsSwitch := currentSession != tmux.SafeName(sessionName)
	if needsSwitch {
		if err := tmux.SwitchToTeamWindow(sessionName, role); err != nil {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch to session: %v", err))
			return true
		}
	}

	if err := hooks.SendFromTelegram(target, tmux.SafeName(sessionName), messageText); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to send message: %v", err))
		return true
	}

	loggingpkg.ListenLog("[team-session] Topic:%d Role:%s Session:%s: %s", topicID, role, sessionName, messageText)
	return true
}
