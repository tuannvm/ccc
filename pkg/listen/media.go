package listen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/lookup"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/pkg/transcribe"
)

// HandleVoiceMessage processes voice messages in session topics
func HandleVoiceMessage(cfg *configpkg.Config, msg telegram.TelegramMessage, chatID, threadID int64) {
	cfg, _ = configpkg.Load()
	sessionName := lookup.GetSessionByTopic(cfg, threadID)
	if sessionName == "" {
		return
	}
	sessionInfo := lookup.GetSessionInfo(cfg, sessionName, threadID)
	if sessionInfo == nil {
		return
	}

	telegram.SendMessage(cfg, chatID, threadID, "🎤 Transcribing...")
	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("voice_%d.ogg", time.Now().UnixNano()))
	if err := telegram.DownloadTelegramFile(cfg, msg.Voice.FileID, audioPath); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		transcription, err := transcribe.TranscribeAudio(cfg, audioPath)
		os.Remove(audioPath)
		if err != nil {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Transcription failed: %v", err))
		} else if transcription != "" {
			loggingpkg.ListenLog("[voice] @%s: %s", msg.From.Username, transcription)
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("📝 %s", transcription))
			voiceText := "[Audio transcription, may contain errors]: " + transcription
			voiceLedgerID := fmt.Sprintf("tg:%d:voice", msg.MessageID)
			ledger.AppendMessage(&ledger.MessageRecord{
				ID: voiceLedgerID, Session: sessionName, Type: "user_prompt",
				Text: voiceText, Origin: "telegram",
				TerminalDelivered: false, TelegramDelivered: true,
			})
			workDir := lookup.GetSessionWorkDir(cfg, sessionName, sessionInfo)
			worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)
			if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
				target, _ := tmux.GetWindowTarget(sessionName)
				if err := hooks.SendFromTelegram(target, tmux.SafeName(sessionName), voiceText); err == nil {
					ledger.UpdateDelivery(sessionName, voiceLedgerID, "terminal_delivered", true)
				}
			}
		}
	}
}

// HandlePhotoMessage processes photo messages in session topics
func HandlePhotoMessage(cfg *configpkg.Config, msg telegram.TelegramMessage, chatID, threadID int64) {
	cfg, _ = configpkg.Load()
	sessionName := lookup.GetSessionByTopic(cfg, threadID)
	if sessionName == "" {
		return
	}
	sessionInfo := lookup.GetSessionInfo(cfg, sessionName, threadID)
	if sessionInfo == nil {
		return
	}
	if len(msg.Photo) == 0 {
		return
	}
	photo := msg.Photo[len(msg.Photo)-1]
	imgPath := filepath.Join(os.TempDir(), fmt.Sprintf("telegram_%d.jpg", time.Now().UnixNano()))
	if err := telegram.DownloadTelegramFile(cfg, photo.FileID, imgPath); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		var prompt string
		caption := msg.Caption
		if caption != "" {
			prompt = fmt.Sprintf("read @%s — %s", imgPath, caption)
		} else {
			prompt = fmt.Sprintf("read @%s", imgPath)
		}
		telegram.SendMessage(cfg, chatID, threadID, "📷 Image saved, sending to Claude...")
		photoLedgerID := fmt.Sprintf("tg:%d:photo", msg.MessageID)
		ledger.AppendMessage(&ledger.MessageRecord{
			ID: photoLedgerID, Session: sessionName, Type: "user_prompt",
			Text: prompt, Origin: "telegram",
			TerminalDelivered: false, TelegramDelivered: true,
		})
		workDir := lookup.GetSessionWorkDir(cfg, sessionName, sessionInfo)
		worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)
		if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
			target, _ := tmux.GetWindowTarget(sessionName)
			if err := hooks.SendFromTelegramWithDelay(target, tmux.SafeName(sessionName), prompt, 2*time.Second); err == nil {
				ledger.UpdateDelivery(sessionName, photoLedgerID, "terminal_delivered", true)
			}
		}
	}
}

// HandleDocumentMessage processes document messages in session topics
func HandleDocumentMessage(cfg *configpkg.Config, msg telegram.TelegramMessage, chatID, threadID int64) {
	cfg, _ = configpkg.Load()
	sessionName := lookup.GetSessionByTopic(cfg, threadID)
	if sessionName == "" {
		return
	}
	sessionInfo := lookup.GetSessionInfo(cfg, sessionName, threadID)
	if sessionInfo == nil {
		return
	}
	destDir := sessionInfo.Path
	if destDir == "" {
		destDir = configpkg.ResolveProjectPath(cfg, sessionName)
	}
	// Sanitize filename to prevent path traversal
	safeName := filepath.Base(msg.Document.FileName)
	if safeName == "" || safeName == "." || safeName == ".." {
		telegram.SendMessage(cfg, chatID, threadID, "❌ Invalid file name")
		return
	}
	destDirAbs, _ := filepath.Abs(destDir)
	destPath := filepath.Join(destDirAbs, safeName)
	if rel, err := filepath.Rel(destDirAbs, destPath); err != nil || strings.HasPrefix(rel, "..") {
		telegram.SendMessage(cfg, chatID, threadID, "❌ Invalid file path")
		return
	}
	if err := telegram.DownloadTelegramFile(cfg, msg.Document.FileID, destPath); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
	} else {
		caption := msg.Caption
		if caption == "" {
			caption = fmt.Sprintf("I sent you this file: %s", destPath)
		} else {
			caption = fmt.Sprintf("%s\n\nFile: %s", caption, destPath)
		}
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("📎 File saved: %s", destPath))
		docLedgerID := fmt.Sprintf("tg:%d:doc", msg.MessageID)
		ledger.AppendMessage(&ledger.MessageRecord{
			ID: docLedgerID, Session: sessionName, Type: "user_prompt",
			Text: caption, Origin: "telegram",
			TerminalDelivered: false, TelegramDelivered: true,
		})
		workDir := lookup.GetSessionWorkDir(cfg, sessionName, sessionInfo)
		worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)
		if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
			target, _ := tmux.GetWindowTarget(sessionName)
			if err := hooks.SendFromTelegram(target, tmux.SafeName(sessionName), caption); err == nil {
				ledger.UpdateDelivery(sessionName, docLedgerID, "terminal_delivered", true)
			}
		}
	}
}
