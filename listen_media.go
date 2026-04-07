package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Media message handlers for voice, photo, and document messages.

// handleVoiceMessage processes voice messages in session topics
func handleVoiceMessage(config *Config, msg TelegramMessage, chatID, threadID int64) {
	config, _ = loadConfig()
	sessionName := getSessionByTopic(config, threadID)
	if sessionName == "" {
		return
	}
	sessionInfo := config.Sessions[sessionName]
	if sessionInfo == nil {
		return
	}

	sendMessage(config, chatID, threadID, "🎤 Transcribing...")
	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("voice_%d.ogg", time.Now().UnixNano()))
	if err := downloadTelegramFile(config, msg.Voice.FileID, audioPath); err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		transcription, err := transcribeAudio(config, audioPath)
		os.Remove(audioPath)
		if err != nil {
			sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Transcription failed: %v", err))
		} else if transcription != "" {
			listenLog("[voice] @%s: %s", msg.From.Username, transcription)
			sendMessage(config, chatID, threadID, fmt.Sprintf("📝 %s", transcription))
			voiceText := "[Audio transcription, may contain errors]: " + transcription
			voiceLedgerID := fmt.Sprintf("tg:%d:voice", msg.MessageID)
			appendMessage(&MessageRecord{
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
			if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
				target, _ := getCccWindowTarget(sessionName)
				if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessionName), voiceText); err == nil {
					updateDelivery(sessionName, voiceLedgerID, "terminal_delivered", true)
				}
			}
		}
	}
}

// handlePhotoMessage processes photo messages in session topics
func handlePhotoMessage(config *Config, msg TelegramMessage, chatID, threadID int64) {
	config, _ = loadConfig()
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
	if err := downloadTelegramFile(config, photo.FileID, imgPath); err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		var prompt string
		caption := msg.Caption
		if caption != "" {
			prompt = fmt.Sprintf("read @%s — %s", imgPath, caption)
		} else {
			prompt = fmt.Sprintf("read @%s", imgPath)
		}
		sendMessage(config, chatID, threadID, fmt.Sprintf("📷 Image saved, sending to Claude..."))
		photoLedgerID := fmt.Sprintf("tg:%d:photo", msg.MessageID)
		appendMessage(&MessageRecord{
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
		if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
			target, _ := getCccWindowTarget(sessionName)
			if err := sendToTmuxFromTelegramWithDelay(target, tmuxSafeName(sessionName), prompt, 2*time.Second); err == nil {
				updateDelivery(sessionName, photoLedgerID, "terminal_delivered", true)
			}
		}
	}
}

// handleDocumentMessage processes document messages in session topics
func handleDocumentMessage(config *Config, msg TelegramMessage, chatID, threadID int64) {
	config, _ = loadConfig()
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
		destDir = resolveProjectPath(config, sessionName)
	}
	destPath := filepath.Join(destDir, msg.Document.FileName)
	if err := downloadTelegramFile(config, msg.Document.FileID, destPath); err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		caption := msg.Caption
		if caption == "" {
			caption = fmt.Sprintf("I sent you this file: %s", destPath)
		} else {
			caption = fmt.Sprintf("%s\n\nFile: %s", caption, destPath)
		}
		sendMessage(config, chatID, threadID, fmt.Sprintf("📎 File saved: %s", destPath))
		docLedgerID := fmt.Sprintf("tg:%d:doc", msg.MessageID)
		appendMessage(&MessageRecord{
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
		if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
			target, _ := getCccWindowTarget(sessionName)
			if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessionName), caption); err == nil {
				updateDelivery(sessionName, docLedgerID, "terminal_delivered", true)
			}
		}
	}
}
