package main

import (
	"net/http"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/auth"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	updatepkg "github.com/tuannvm/ccc/pkg/update"
)

// Main listen loop — polls Telegram for updates and dispatches commands.

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

			// Handle callback queries (button presses)
			if update.CallbackQuery != nil {
				listenpkg.HandleCallbackQuery(config, update.CallbackQuery)
				continue
			}

			msg := update.Message

			// Authorization check: depends on multi-user mode
			if !auth.IsAuthorizedMessage(config, msg) {
				continue
			}

			chatID := msg.Chat.ID
			threadID := msg.MessageThreadID
			isGroup := msg.Chat.Type == "supergroup"

			// Handle voice messages
			if msg.Voice != nil && isGroup && threadID > 0 {
				listenpkg.HandleVoiceMessage(config, msg, chatID, threadID)
				continue
			}

			// Handle photo messages
			if len(msg.Photo) > 0 && isGroup && threadID > 0 {
				listenpkg.HandlePhotoMessage(config, msg, chatID, threadID)
				continue
			}

			// Handle document messages
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

			// Handle OTP code responses (for permission approval)
			if auth.HandleOTPResponse(config, text, chatID, threadID) {
				continue
			}

			// Handle commands
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

			// /stop command - interrupt current Claude execution
			if text == "/stop" {
				listenpkg.HandleStopCommand(config, chatID, threadID, isGroup)
				continue
			}

			// If auth is waiting for code, send it
			if auth.IsAuthWaitingCode() && !strings.HasPrefix(text, "/") {
				go auth.HandleAuthCode(config, chatID, threadID, text)
				continue
			}

			// /continue command - restart session preserving conversation history
			if text == "/continue" && isGroup && threadID > 0 {
				listenpkg.HandleContinueCommand(config, chatID, threadID)
				continue
			}

			// /delete command - delete session and thread
			if text == "/delete" && isGroup && threadID > 0 {
				listenpkg.HandleDeleteCommand(config, chatID, threadID)
				continue
			}

			// /providers command - list available providers or change session provider
			if text == "/providers" || strings.HasPrefix(text, "/provider") {
				listenpkg.HandleProvidersCommand(config, chatID, threadID, text, isGroup)
				continue
			}

			// /resume command - manage Claude session IDs
			if strings.HasPrefix(text, "/resume") && isGroup && threadID > 0 {
				listenpkg.HandleResumeCommand(config, chatID, threadID, text)
				continue
			}

			// /cleanup command - delete tmux sessions and Telegram topics (NOT folders)
			if text == "/cleanup" {
				listenpkg.HandleCleanupCommand(config, chatID, threadID)
				continue
			}

			// /team command - create team session
			if strings.HasPrefix(text, "/team") && isGroup {
				listenpkg.HandleTeamCreateCommand(config, chatID, threadID, text)
				continue
			}

			// /new command - create/restart session
			if strings.HasPrefix(text, "/new") && isGroup {
				listenpkg.HandleNewCommand(config, chatID, threadID, text)
				continue
			}

			// /worktree command - create worktree session from existing session
			if strings.HasPrefix(text, "/worktree") && isGroup {
				handleWorktreeCommand(config, chatID, threadID, text)
				continue
			}

			// Check if message is in a topic (interactive session)
			if isGroup && threadID > 0 {
				listenpkg.HandleSessionMessage(config, text, chatID, threadID, update.UpdateID)
				continue
			}

			// Private chat: run one-shot Claude
			if !isGroup {
				handlePrivateChat(config, msg, chatID, threadID)
			}
		}
	}
}
