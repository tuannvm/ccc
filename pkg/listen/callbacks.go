package listen

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/lookup"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/pkg/auth"
	"github.com/tuannvm/ccc/pkg/session"
)

// HandleCallbackQuery processes callback queries from inline keyboard button presses
func HandleCallbackQuery(cfg *configpkg.Config, cb *telegram.CallbackQuery) {
	if cb == nil {
		return
	}

	if !auth.IsAuthorizedCallback(cfg, cb) {
		return
	}

	telegram.AnswerCallbackQuery(cfg, cb.ID)

	parts := strings.Split(cb.Data, ":")
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "new":
		if len(parts) == 3 {
			sessionName := parts[1]
			providerName := parts[2]
			HandleNewWithProvider(cfg, cb, sessionName, providerName)
		}
		return

	case "team":
		if len(parts) == 3 {
			teamName := parts[1]
			providerName := parts[2]
			HandleTeamWithProvider(cfg, cb, teamName, providerName)
		}
		return

	case "provider":
		if len(parts) == 3 {
			sessionName := parts[1]
			providerName := parts[2]
			HandleProviderChange(cfg, cb, sessionName, providerName)
		}
		return

	default:
		HandleQuestionCallback(cfg, cb, parts)
	}
}

// HandleQuestionCallback handles legacy question selection callbacks
func HandleQuestionCallback(cfg *configpkg.Config, cb *telegram.CallbackQuery, parts []string) {
	if len(parts) < 3 {
		return
	}

	sessionName := parts[0]
	questionIndex, _ := strconv.Atoi(parts[1])
	var totalQuestions, optionIndex int
	if len(parts) == 4 {
		totalQuestions, _ = strconv.Atoi(parts[2])
		optionIndex, _ = strconv.Atoi(parts[3])
	} else {
		optionIndex, _ = strconv.Atoi(parts[2])
	}

	if cb.Message != nil {
		originalText := cb.Message.Text
		newText := fmt.Sprintf("%s\n\n✓ Selected option %d", originalText, optionIndex+1)
		telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID, newText)
	}

	sessionInfo, exists := cfg.Sessions[sessionName]
	if !exists || sessionInfo == nil {
		return
	}

	workDir := lookup.GetSessionWorkDir(cfg, sessionName, sessionInfo)
	worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)

	if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err != nil {
		return
	}

	target, _ := tmux.GetWindowTarget(sessionName)
	for range optionIndex {
		exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Down").Run()
		time.Sleep(50 * time.Millisecond)
	}
	exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Enter").Run()
	loggingpkg.ListenLog("[callback] Selected option %d for %s (question %d/%d)", optionIndex, sessionName, questionIndex+1, totalQuestions)

	if totalQuestions > 0 && questionIndex == totalQuestions-1 {
		time.Sleep(300 * time.Millisecond)
		exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Enter").Run()
		loggingpkg.ListenLog("[callback] Auto-submitted answers for %s", sessionName)
	}
}

// HandleNewWithProvider creates a session after provider selection via inline keyboard
func HandleNewWithProvider(cfg *configpkg.Config, cb *telegram.CallbackQuery, sessionName, providerName string) {
	existing, exists := cfg.Sessions[sessionName]
	if exists && existing != nil && existing.TopicID != 0 {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("⚠️ Session '%s' already exists.\n\nUse /new without args in that topic to restart.", sessionName))
		}
		return
	}

	topicID, err := telegram.CreateForumTopic(cfg, sessionName, providerName, "")
	if err != nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create topic: %v", err))
		}
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

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	resultMsg := fmt.Sprintf("🚀 Session '%s' started!\n🤖 Provider: %s\n\nSend messages here to interact with Claude.", sessionName, providerName)
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		resultMsg = fmt.Sprintf("❌ Failed to start session: %v", err)
	}

	if cb.Message != nil {
		telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

// HandleProviderChange changes provider for an existing session via inline keyboard
func HandleProviderChange(cfg *configpkg.Config, cb *telegram.CallbackQuery, sessionName, providerName string) {
	sess, exists := cfg.Sessions[sessionName]
	if !exists || sess == nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Session '%s' not found.", sessionName))
		}
		return
	}

	provider := providerpkg.GetProvider(cfg, providerName)
	if provider == nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Provider '%s' not found. Available: %v", providerName, providerpkg.GetProviderNames(cfg)))
		}
		return
	}

	sess.ProviderName = providerName
	configpkg.Save(cfg)

	resultMsg := fmt.Sprintf("✅ Provider changed to %s for session '%s'\n\nRestart with /new to apply the new provider.", provider.Name(), sessionName)
	if cb.Message != nil {
		telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

// HandleTeamWithProvider creates a team session after provider selection via inline keyboard
func HandleTeamWithProvider(cfg *configpkg.Config, cb *telegram.CallbackQuery, teamName, providerName string) {
	for topicID, sessInfo := range cfg.TeamSessions {
		if sessInfo != nil {
			sessName := getSessionNameFromInfo(sessInfo)
			if sessName == teamName {
				if cb.Message != nil {
					telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
						fmt.Sprintf("⚠️ Team session '%s' already exists (topic: %d).\n\nUse /team without args in that topic to restart.", teamName, topicID))
				}
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
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				"❌ Team runtime not available. Check your installation.")
		}
		return
	}

	if err := runtime.EnsureLayout(sessInfo, workDir); err != nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create team layout: %v", err))
		}
		return
	}

	topicID, err := telegram.CreateForumTopic(cfg, teamName, providerName, "")
	if err != nil {
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+teamName).Run()
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create topic: %v", err))
		}
		return
	}

	sessInfo.TopicID = topicID
	cfg.SetTeamSession(topicID, sessInfo)
	if err := configpkg.Save(cfg); err != nil {
		telegram.DeleteForumTopic(cfg, topicID)
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+teamName).Run()
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to save config: %v", err))
		}
		return
	}

	if err := EnsureHooks(cfg, teamName, sessInfo); err != nil {
		loggingpkg.ListenLog("[/team] Failed to install hooks for %s: %v", teamName, err)
	}

	if err := runtime.StartClaude(sessInfo, workDir); err != nil {
		loggingpkg.ListenLog("[/team] Failed to start Claude for %s: %v", teamName, err)
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
		telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

// getSessionNameFromInfo extracts session name from SessionInfo
func getSessionNameFromInfo(info *configpkg.SessionInfo) string {
	if info.SessionName != "" {
		return info.SessionName
	}
	return tmux.GetSessionNameFromPath(info.Path)
}
