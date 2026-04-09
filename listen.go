package main

import (
	"net/http"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/auth"
	execpkg "github.com/tuannvm/ccc/pkg/exec"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/tmux"
	updatepkg "github.com/tuannvm/ccc/pkg/update"
)

func listen() error {
	init, err := listenpkg.InitializeListener()
	if err != nil {
		return err
	}
	defer init.LockFile.Close()
	defer init.ReleaseLock()

	config := init.Config
	offset := 0
	client := &http.Client{Timeout: 35 * time.Second}

	for {
		result := listenpkg.PollUpdates(client, config, offset)
		if result.ShouldSkip {
			continue
		}

		for _, update := range result.Response.Result {
			offset = update.UpdateID + 1

			if update.CallbackQuery != nil {
				listenpkg.HandleCallbackQuery(config, update.CallbackQuery)
				continue
			}

			msg := update.Message

			if !auth.IsAuthorizedMessage(config, msg) {
				continue
			}

			chatID := msg.Chat.ID
			threadID := msg.MessageThreadID
			isGroup := msg.Chat.Type == "supergroup"

			if msg.Voice != nil && isGroup && threadID > 0 {
				listenpkg.HandleVoiceMessage(config, msg, chatID, threadID)
				continue
			}

			if len(msg.Photo) > 0 && isGroup && threadID > 0 {
				listenpkg.HandlePhotoMessage(config, msg, chatID, threadID)
				continue
			}

			if msg.Document != nil && isGroup && threadID > 0 {
				listenpkg.HandleDocumentMessage(config, msg, chatID, threadID)
				continue
			}

			text := strings.TrimSpace(msg.Text)
			if text == "" {
				continue
			}

			text = listenpkg.StripBotMention(text)

			loggingpkg.ListenLog("[%s] @%s: %s", msg.Chat.Type, msg.From.Username, configpkg.RedactGitURLsInText(text))

			if auth.HandleOTPResponse(config, text, chatID, threadID) {
				continue
			}

			if strings.HasPrefix(text, "/c ") {
				listenpkg.HandleShellCommand(config, chatID, threadID, strings.TrimPrefix(text, "/c "))
				continue
			}

			if text == "/update" {
				updatepkg.ApplyUpdate(config, chatID, threadID, offset)
				continue
			}

			if text == "/restart" {
				listenpkg.HandleRestartCommand(config, chatID, threadID)
				continue
			}

			if text == "/stats" {
				listenpkg.HandleStatsCommand(config, chatID, threadID)
				continue
			}

			if text == "/version" {
				listenpkg.HandleVersionCommand(config, chatID, threadID, version)
				continue
			}

			if text == "/auth" {
				go auth.HandleAuth(config, chatID, threadID)
				continue
			}

			if text == "/stop" {
				listenpkg.HandleStopCommand(config, chatID, threadID, isGroup)
				continue
			}

			if auth.IsAuthWaitingCode() && !strings.HasPrefix(text, "/") {
				go auth.HandleAuthCode(config, chatID, threadID, text)
				continue
			}

			if text == "/continue" && isGroup && threadID > 0 {
				listenpkg.HandleContinueCommand(config, chatID, threadID)
				continue
			}

			if text == "/delete" && isGroup && threadID > 0 {
				listenpkg.HandleDeleteCommand(config, chatID, threadID)
				continue
			}

			if text == "/providers" || strings.HasPrefix(text, "/provider") {
				listenpkg.HandleProvidersCommand(config, chatID, threadID, text, isGroup)
				continue
			}

			if strings.HasPrefix(text, "/resume") && isGroup && threadID > 0 {
				listenpkg.HandleResumeCommand(config, chatID, threadID, text)
				continue
			}

			if text == "/cleanup" {
				listenpkg.HandleCleanupCommand(config, chatID, threadID)
				continue
			}

			if strings.HasPrefix(text, "/team") && isGroup {
				listenpkg.HandleTeamCreateCommand(config, chatID, threadID, text)
				continue
			}

			if strings.HasPrefix(text, "/new") && isGroup {
				listenpkg.HandleNewCommand(config, chatID, threadID, text)
				continue
			}

			if strings.HasPrefix(text, "/worktree") && isGroup {
				listenpkg.HandleWorktreeCommand(config, chatID, threadID, text, tmux.WorktreeAutoGenerate)
				continue
			}

			if isGroup && threadID > 0 {
				listenpkg.HandleSessionMessage(config, text, chatID, threadID, update.UpdateID)
				continue
			}

			if !isGroup {
				listenpkg.HandlePrivateChat(config, msg, chatID, threadID, execpkg.RunClaudeOneShot)
			}
		}
	}
}
