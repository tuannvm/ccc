package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// Media message handlers for voice, photo, and document messages.

// handleVoiceMessage processes voice messages in session topics
func handleVoiceMessage(config *Config, msg TelegramMessage, chatID, threadID int64) {
	config, _ = configpkg.Load()
	sessionName := getSessionByTopic(config, threadID)
	if sessionName == "" {
		return
	}
	sessionInfo := config.Sessions[sessionName]
	if sessionInfo == nil {
		return
	}

	telegram.SendMessage(config, chatID, threadID, "🎤 Transcribing...")
	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("voice_%d.ogg", time.Now().UnixNano()))
	if err := telegram.DownloadTelegramFile(config, msg.Voice.FileID, audioPath); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		transcription, err := transcribeAudio(config, audioPath)
		os.Remove(audioPath)
		if err != nil {
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Transcription failed: %v", err))
		} else if transcription != "" {
			listenLog("[voice] @%s: %s", msg.From.Username, transcription)
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("📝 %s", transcription))
			voiceText := "[Audio transcription, may contain errors]: " + transcription
			voiceLedgerID := fmt.Sprintf("tg:%d:voice", msg.MessageID)
			ledger.AppendMessage(&MessageRecord{
				ID: voiceLedgerID, Session: sessionName, Type: "user_prompt",
				Text: voiceText, Origin: "telegram",
				TerminalDelivered: false, TelegramDelivered: true,
			})
			workDir := getSessionWorkDir(config, sessionName, sessionInfo)
			worktreeName := ""
			if sessionInfo.IsWorktree {
				worktreeName = sessionInfo.WorktreeName
			}
			resumeSessionID := sessionInfo.ClaudeSessionID
			if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
				target, _ := tmux.GetWindowTarget(sessionName)
				if err := sendToTmuxFromTelegram(target, tmux.SafeName(sessionName), voiceText); err == nil {
					ledger.UpdateDelivery(sessionName, voiceLedgerID, "terminal_delivered", true)
				}
			}
		}
	}
}

// handlePhotoMessage processes photo messages in session topics
func handlePhotoMessage(config *Config, msg TelegramMessage, chatID, threadID int64) {
	config, _ = configpkg.Load()
	sessionName := getSessionByTopic(config, threadID)
	if sessionName == "" {
		return
	}
	sessionInfo := config.Sessions[sessionName]
	if sessionInfo == nil {
		return
	}
	photo := msg.Photo[len(msg.Photo)-1]
	imgPath := filepath.Join(os.TempDir(), fmt.Sprintf("telegram_%d.jpg", time.Now().UnixNano()))
	if err := telegram.DownloadTelegramFile(config, photo.FileID, imgPath); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		var prompt string
		caption := msg.Caption
		if caption != "" {
			prompt = fmt.Sprintf("read @%s — %s", imgPath, caption)
		} else {
			prompt = fmt.Sprintf("read @%s", imgPath)
		}
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("📷 Image saved, sending to Claude..."))
		photoLedgerID := fmt.Sprintf("tg:%d:photo", msg.MessageID)
		ledger.AppendMessage(&MessageRecord{
			ID: photoLedgerID, Session: sessionName, Type: "user_prompt",
			Text: caption, Origin: "telegram",
			TerminalDelivered: false, TelegramDelivered: true,
		})
		workDir := getSessionWorkDir(config, sessionName, sessionInfo)
		worktreeName := ""
		if sessionInfo.IsWorktree {
			worktreeName = sessionInfo.WorktreeName
		}
		resumeSessionID := sessionInfo.ClaudeSessionID
		if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
			target, _ := tmux.GetWindowTarget(sessionName)
			if err := sendToTmuxFromTelegramWithDelay(target, tmux.SafeName(sessionName), prompt, 2*time.Second); err == nil {
				ledger.UpdateDelivery(sessionName, photoLedgerID, "terminal_delivered", true)
			}
		}
	}
}

// handleDocumentMessage processes document messages in session topics
func handleDocumentMessage(config *Config, msg TelegramMessage, chatID, threadID int64) {
	config, _ = configpkg.Load()
	sessionName := getSessionByTopic(config, threadID)
	if sessionName == "" {
		return
	}
	sessionInfo := config.Sessions[sessionName]
	if sessionInfo == nil {
		return
	}
	destDir := sessionInfo.Path
	if destDir == "" {
		destDir = configpkg.ResolveProjectPath(config, sessionName)
	}
	destPath := filepath.Join(destDir, msg.Document.FileName)
	if err := telegram.DownloadTelegramFile(config, msg.Document.FileID, destPath); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		caption := msg.Caption
		if caption == "" {
			caption = fmt.Sprintf("I sent you this file: %s", destPath)
		} else {
			caption = fmt.Sprintf("%s\n\nFile: %s", caption, destPath)
		}
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("📎 File saved: %s", destPath))
		docLedgerID := fmt.Sprintf("tg:%d:doc", msg.MessageID)
		ledger.AppendMessage(&MessageRecord{
			ID: docLedgerID, Session: sessionName, Type: "user_prompt",
			Text: caption, Origin: "telegram",
			TerminalDelivered: false, TelegramDelivered: true,
		})
		workDir := getSessionWorkDir(config, sessionName, sessionInfo)
		worktreeName := ""
		if sessionInfo.IsWorktree {
			worktreeName = sessionInfo.WorktreeName
		}
		resumeSessionID := sessionInfo.ClaudeSessionID
		if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
			target, _ := tmux.GetWindowTarget(sessionName)
			if err := sendToTmuxFromTelegram(target, tmux.SafeName(sessionName), caption); err == nil {
				ledger.UpdateDelivery(sessionName, docLedgerID, "terminal_delivered", true)
			}
		}
	}
}
