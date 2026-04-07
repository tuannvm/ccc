package main

import (
	"fmt"
	"os"
	"time"
)

// handleSessionMessage routes a text message to the appropriate session
func handleSessionMessage(config *Config, text string, chatID, threadID int64, updateID int) {
	config, _ = loadConfig()

	// Check if this is a team session (NEW: multi-pane support)
	if config.IsTeamSession(threadID) {
		// Handle team session routing (goes to specific pane)
		if handled := handleTeamSessionMessage(config, text, threadID, chatID, threadID); handled {
			return
		}
		// If handleTeamSessionMessage returns false, fall through to standard handling
	}

	sessName := getSessionByTopic(config, threadID)
	if sessName != "" {
		var target string
		var err error

		// Only switch if the requested session is different from current
		// Compare using tmux-safe names since window names are sanitized
		currentSession := getCurrentSessionName()
		needsSwitch := currentSession != tmuxSafeName(sessName)

		if needsSwitch {
			// Switch to the correct session in the single ccc window
			sessionInfo := config.Sessions[sessName]
			workDir := getSessionWorkDir(config, sessName, sessionInfo)
			if _, err := os.Stat(workDir); os.IsNotExist(err) {
				os.MkdirAll(workDir, 0755)
			}

			// Preserve worktree name if this is a worktree session
			worktreeName := ""
			if sessionInfo.IsWorktree {
				worktreeName = sessionInfo.WorktreeName
			}

			// Use stored Claude session ID to resume existing conversation
			resumeSessionID := sessionInfo.ClaudeSessionID

			// Ensure hooks are installed before resuming session
			if err := ensureHooksForSession(config, sessName, sessionInfo); err != nil {
				listenLog("[sendToTmux] Failed to verify/install hooks for %s: %v", sessName, err)
			}

			// Switch to the session in the single ccc window (always restart when switching sessions)
			if err := switchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
				sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
				return
			}

			// Get the target for the project window
			target, err = getCccWindowTarget(sessName)
			if err != nil {
				sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to get ccc window: %v", err))
				return
			}

			// Wait for Claude to be ready after switching
			time.Sleep(1 * time.Second)

			// Replay any undelivered terminal messages for this session
			undelivered := findUndelivered(sessName, "terminal")
			for _, ur := range undelivered {
				if ur.Type == "user_prompt" && ur.Origin == "telegram" {
					if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessName), ur.Text); err == nil {
						updateDelivery(sessName, ur.ID, "terminal_delivered", true)
					}
					time.Sleep(500 * time.Millisecond)
				}
			}

			listenLog("sendToTmux: target=%s session=%s (switched from %s)", target, sessName, currentSession)
		} else {
			// Already in the correct session, just get the target
			target, err = getCccWindowTarget(sessName)
			if err != nil {
				sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to get ccc window: %v", err))
				return
			}
			listenLog("sendToTmux: target=%s session=%s (already active)", target, sessName)
		}

		// Record in ledger before sending
		ledgerID := fmt.Sprintf("tg:%d", updateID)
		appendMessage(&MessageRecord{
			ID:                ledgerID,
			Session:           sessName,
			Type:              "user_prompt",
			Text:              text,
			Origin:            "telegram",
			TerminalDelivered: false,
			TelegramDelivered: true,
		})

		if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessName), text); err != nil {
			listenLog("sendToTmux FAILED: target=%s err=%v", target, err)
			sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to send: %v", err))
		} else {
			updateDelivery(sessName, ledgerID, "terminal_delivered", true)
		}
	} else {
		sendMessage(config, chatID, threadID, "⚠️ No session linked to this topic. Use /new <name> to create one.")
	}
}
