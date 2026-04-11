package listen

import (
	"net/http"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/auth"
	configpkg "github.com/tuannvm/ccc/pkg/config"
	execpkg "github.com/tuannvm/ccc/pkg/exec"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/tmux"
	updatepkg "github.com/tuannvm/ccc/pkg/update"
)

// Run starts the Telegram long-poll listener: acquires lock, initializes
// logging/config, then enters the poll-dispatch loop.
func Run(version string) error {
	init, err := InitializeListener()
	if err != nil {
		return err
	}
	defer init.LockFile.Close()
	defer init.ReleaseLock()

	config := init.Config
	offset := 0
	client := &http.Client{Timeout: 35 * time.Second}

	for {
		result := PollUpdates(client, config, offset)
		if result.ShouldSkip {
			continue
		}

		for _, update := range result.Response.Result {
			offset = update.UpdateID + 1

			if update.CallbackQuery != nil {
				HandleCallbackQuery(config, update.CallbackQuery)
				continue
			}

			msg := update.Message
			if msg.Chat.ID == 0 && msg.Text == "" && msg.Voice == nil && msg.Photo == nil && msg.Document == nil {
				continue
			}

			if !auth.IsAuthorizedMessage(config, msg) {
				continue
			}

			chatID := msg.Chat.ID
			threadID := msg.MessageThreadID
			isGroup := msg.Chat.Type == "supergroup"

			if msg.Voice != nil && isGroup && threadID > 0 {
				HandleVoiceMessage(config, msg, chatID, threadID)
				continue
			}

			if len(msg.Photo) > 0 && isGroup && threadID > 0 {
				HandlePhotoMessage(config, msg, chatID, threadID)
				continue
			}

			if msg.Document != nil && isGroup && threadID > 0 {
				HandleDocumentMessage(config, msg, chatID, threadID)
				continue
			}

			text := strings.TrimSpace(msg.Text)
			if text == "" {
				continue
			}

			text = StripBotMention(text)

			loggingpkg.ListenLog("[%s] @%s: %s", msg.Chat.Type, msg.From.Username, configpkg.RedactGitURLsInText(text))

			if auth.HandleOTPResponse(config, text, chatID, threadID) {
				continue
			}

			if strings.HasPrefix(text, "/c ") {
				HandleShellCommand(config, chatID, threadID, strings.TrimPrefix(text, "/c "))
				continue
			}

			if text == "/update" {
				updatepkg.ApplyUpdate(config, chatID, threadID, offset)
				continue
			}

			if text == "/restart" {
				HandleRestartCommand(config, chatID, threadID)
				continue
			}

			if text == "/stats" {
				HandleStatsCommand(config, chatID, threadID)
				continue
			}

			if text == "/version" {
				HandleVersionCommand(config, chatID, threadID, version)
				continue
			}

			if text == "/auth" {
				go auth.HandleAuth(config, chatID, threadID)
				continue
			}

			if text == "/stop" {
				HandleStopCommand(config, chatID, threadID, isGroup)
				continue
			}

			if auth.IsAuthWaitingCode() && !strings.HasPrefix(text, "/") {
				go auth.HandleAuthCode(config, chatID, threadID, text)
				continue
			}

			if text == "/continue" && isGroup && threadID > 0 {
				HandleContinueCommand(config, chatID, threadID)
				continue
			}

			if text == "/delete" && isGroup && threadID > 0 {
				HandleDeleteCommand(config, chatID, threadID)
				continue
			}

			if text == "/providers" || strings.HasPrefix(text, "/provider") {
				HandleProvidersCommand(config, chatID, threadID, text, isGroup)
				continue
			}

			if strings.HasPrefix(text, "/resume") && isGroup && threadID > 0 {
				HandleResumeCommand(config, chatID, threadID, text)
				continue
			}

			if text == "/cleanup" {
				HandleCleanupCommand(config, chatID, threadID)
				continue
			}

			if strings.HasPrefix(text, "/team") && isGroup {
				HandleTeamCreateCommand(config, chatID, threadID, text)
				continue
			}

			if strings.HasPrefix(text, "/new") && isGroup {
				HandleNewCommand(config, chatID, threadID, text)
				continue
			}

			if strings.HasPrefix(text, "/worktree") && isGroup {
				HandleWorktreeCommand(config, chatID, threadID, text, tmux.WorktreeAutoGenerate)
				continue
			}

			if isGroup && threadID > 0 {
				HandleSessionMessage(config, text, chatID, threadID, update.UpdateID)
				continue
			}

			if !isGroup {
				HandlePrivateChat(config, msg, chatID, threadID, execpkg.RunClaudeOneShot)
			}
		}
	}
}
