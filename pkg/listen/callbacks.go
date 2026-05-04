package listen

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/auth"
	configpkg "github.com/tuannvm/ccc/pkg/config"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/lookup"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/session"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
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

	if cfg.Sessions == nil {
		cfg.Sessions = make(map[string]*configpkg.SessionInfo)
	}
	cfg.Sessions[sessionName] = &configpkg.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	if err := configpkg.Save(cfg); err != nil {
		delete(cfg.Sessions, sessionName) // rollback on failure
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("⚠️ Failed to save config: %v", err))
		}
		return
	}
	pinSessionHeader(cfg, sessionName, cfg.Sessions[sessionName])
	if err := EnsureAgentHooks(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
		loggingpkg.ListenLog("[callback:new] Failed to install hooks for %s: %v", sessionName, err)
	}

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	resultMsg := fmt.Sprintf("%s started\n%s\n\nSend messages here to interact with %s.", sessionName, selectedProviderSummary(providerName), agentDisplayName(providerName))
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

	previousProvider := sess.ProviderName
	sess.ProviderName = providerName
	if err := configpkg.Save(cfg); err != nil {
		sess.ProviderName = previousProvider
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("⚠️ Failed to save config: %v", err))
		}
		return
	}
	pinSessionHeader(cfg, sessionName, sess)

	resultMsg := fmt.Sprintf("provider changed\nsession: %s\nprovider: %s\nsource: session\n\nRestart with /new to apply.", sessionName, provider.Name())
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

	if err := EnsureAgentHooks(cfg, teamName, sessInfo); err != nil {
		loggingpkg.ListenLog("[/team] Failed to install hooks for %s: %v", teamName, err)
	}

	if err := runtime.StartClaude(sessInfo, workDir); err != nil {
		loggingpkg.ListenLog("[/team] Failed to start Claude for %s: %v", teamName, err)
	}

	resultMsg := fmt.Sprintf("✅ Team session '%s' created!\n\n", teamName)
	resultMsg += fmt.Sprintf("📂 Path: %s\n", workDir)
	resultMsg += selectedProviderSummary(providerName) + "\n"
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
