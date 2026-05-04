package listen

import (
	"fmt"
	"os"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/ledger"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/lookup"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// HandleSessionMessage routes a text message to the appropriate session
func HandleSessionMessage(cfg *configpkg.Config, text string, chatID, threadID int64, updateID int) {
	cfg, _ = configpkg.Load()

	// Check if this is a team session
	if cfg.IsTeamSession(threadID) {
		if handled := HandleTeamSessionMessage(cfg, text, threadID, chatID, threadID); handled {
			return
		}
	}

	sessName := lookup.GetSessionByTopic(cfg, threadID)
	if sessName != "" {
		var target string
		var err error

		currentSession := tmux.GetCurrentSessionName()
		needsSwitch := currentSession != tmux.SafeName(sessName)

		if needsSwitch {
			sessionInfo := cfg.Sessions[sessName]
			workDir := lookup.GetSessionWorkDir(cfg, sessName, sessionInfo)
			if _, err := os.Stat(workDir); os.IsNotExist(err) {
				os.MkdirAll(workDir, 0755)
			}

			worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)

			if err := EnsureHooks(cfg, sessName, sessionInfo); err != nil {
				loggingpkg.ListenLog("[sendToTmux] Failed to verify/install hooks for %s: %v", sessName, err)
			}

			// Only skip pane restart when Claude is already running.
			// When Claude is not running (shell only or empty), we need
			// skipRestart=false so SwitchSessionInWindow starts Claude.
			skipRestart := false
			if winTarget, err := tmux.GetWindowTarget(sessName); err == nil {
				if tmux.WindowHasAgentRunning(winTarget, "", effectiveProviderName(cfg, sessionInfo)) {
					skipRestart = true
				}
			}

			if err := tmux.SwitchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, skipRestart); err != nil {
				telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
				return
			}

			target, err = tmux.GetWindowTarget(sessName)
			if err != nil {
				telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to get ccc window: %v", err))
				return
			}

			if skipRestart {
				time.Sleep(200 * time.Millisecond)
			} else {
				time.Sleep(1 * time.Second)
			}

			// Replay any undelivered terminal messages
			undelivered := ledger.FindUndelivered(sessName, "terminal")
			for _, ur := range undelivered {
				if ur.Type == "user_prompt" && ur.Origin == "telegram" {
					if err := hooks.SendFromTelegram(target, tmux.SafeName(sessName), ur.Text); err == nil {
						ledger.UpdateDelivery(sessName, ur.ID, "terminal_delivered", true)
					}
					time.Sleep(500 * time.Millisecond)
				}
			}

			loggingpkg.ListenLog("sendToTmux: target=%s session=%s (switched from %s)", target, sessName, currentSession)
		} else {
			target, err = tmux.GetWindowTarget(sessName)
			if err != nil {
				telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to get ccc window: %v", err))
				return
			}
			loggingpkg.ListenLog("sendToTmux: target=%s session=%s (already active)", target, sessName)
		}

		// Record in ledger before sending
		ledgerID := fmt.Sprintf("tg:%d", updateID)
		ledger.AppendMessage(&ledger.MessageRecord{
			ID:                ledgerID,
			Session:           sessName,
			Type:              "user_prompt",
			Text:              text,
			Origin:            "telegram",
			TerminalDelivered: false,
			TelegramDelivered: true,
		})

		if err := hooks.SendFromTelegramToProvider(target, tmux.SafeName(sessName), text, effectiveProviderName(cfg, cfg.Sessions[sessName])); err != nil {
			loggingpkg.ListenLog("sendToTmux FAILED: target=%s err=%v", target, err)
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to send: %v", err))
		} else {
			ledger.UpdateDelivery(sessName, ledgerID, "terminal_delivered", true)
		}
	} else {
		telegram.SendMessage(cfg, chatID, threadID, "⚠️ No session linked to this topic. Use /new <name> to create one.")
	}
}
