package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/diagnostics"
	"github.com/tuannvm/ccc/pkg/auth"
	"github.com/tuannvm/ccc/pkg/hooks"
)

// Main listen loop — polls Telegram for updates and dispatches commands.

func listen() error {
	// Small random delay to avoid race conditions when multiple instances start
	time.Sleep(time.Duration(os.Getpid()%500) * time.Millisecond)

	// Use a lock file to ensure only one instance runs
	lockPath := filepath.Join(configpkg.CacheDir(), "ccc.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	defer lockFile.Close()

	// Try to acquire exclusive lock (non-blocking)
	releaseLock := acquireFileLock(lockFile)
	defer releaseLock()

	// Write our PID to the lock file
	lockFile.Truncate(0)
	lockFile.Seek(0, 0)
	fmt.Fprintf(lockFile, "%d\n", os.Getpid())

	initListenLog()
	if listenLogFile != nil {
		defer listenLogFile.Close()
	}

	config, err := configpkg.Load()
	if err != nil {
		return fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	listenLog("Bot started (chat: %d, group: %d, sessions: %d)", config.ChatID, config.GroupID, len(config.Sessions))

	telegram.SetBotCommands(config.BotToken)

	// Recover undelivered Telegram messages from ledger
	for sessName, info := range config.Sessions {
		if info == nil || info.TopicID == 0 || config.GroupID == 0 {
			continue
		}
		undelivered := ledger.FindUndelivered(sessName, "telegram")
		for _, ur := range undelivered {
			if ur.Type == "assistant_text" || ur.Type == "notification" {
				telegram.SendMessage(config, config.GroupID, info.TopicID, fmt.Sprintf("*%s:*\n%s", sessName, ur.Text))
				ledger.UpdateDelivery(sessName, ur.ID, "telegram_delivered", true)
			}
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	offset := 0
	client := &http.Client{Timeout: 35 * time.Second}

	go func() {
		sig := <-sigChan
		listenLog("Shutting down (signal: %v)", sig)
		os.Exit(0)
	}()

	// Typing indicator goroutine: sends "typing" action for sessions with thinking flag
	go func() {
		for {
			time.Sleep(4 * time.Second)
			cfg, err := configpkg.Load()
			if err != nil || cfg == nil {
				continue
			}
			for sessName, info := range cfg.Sessions {
				if info == nil || info.TopicID == 0 || cfg.GroupID == 0 {
					continue
				}
				if flagInfo, err := os.Stat(hooks.ThinkingFlag(sessName)); err == nil {
					// Auto-expire after 10 minutes to handle missed stop hooks
					if time.Since(flagInfo.ModTime()) > 10*time.Minute {
						hooks.ClearThinking(sessName)
						continue
					}
					telegram.SendTypingAction(cfg, cfg.GroupID, info.TopicID)
				}
			}
		}
	}()

	for {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", config.BotToken, offset)
		resp, err := telegram.TelegramClientGet(client, config.BotToken, reqURL)
		if err != nil {
			listenLog("Network error: %v (retrying...)", err)
			time.Sleep(5 * time.Second)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.MaxResponseSize))
		resp.Body.Close()

		var updates TelegramUpdate
		if err := json.Unmarshal(body, &updates); err != nil {
			listenLog("Parse error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if !updates.OK {
			listenLog("Telegram API error: %s", updates.Description)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates.Result {
			offset = update.UpdateID + 1

			// Handle callback queries (button presses)
			if update.CallbackQuery != nil {
				handleCallbackQuery(config, update.CallbackQuery)
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
				handleVoiceMessage(config, msg, chatID, threadID)
				continue
			}

			// Handle photo messages
			if len(msg.Photo) > 0 && isGroup && threadID > 0 {
				handlePhotoMessage(config, msg, chatID, threadID)
				continue
			}

			// Handle document messages
			if msg.Document != nil && isGroup && threadID > 0 {
				handleDocumentMessage(config, msg, chatID, threadID)
				continue
			}

			text := strings.TrimSpace(msg.Text)
			if text == "" {
				continue
			}

			// Strip bot mention from commands (e.g., /ping@botname -> /ping)
			if strings.HasPrefix(text, "/") {
				if idx := strings.Index(text, "@"); idx != -1 {
					spaceIdx := strings.Index(text, " ")
					if spaceIdx == -1 || idx < spaceIdx {
						text = text[:idx] + text[strings.Index(text+" ", " "):]
					}
				}
				text = strings.TrimSpace(text)
			}

			listenLog("[%s] @%s: %s", msg.Chat.Type, msg.From.Username, configpkg.RedactGitURLsInText(text))

			// Handle OTP code responses (for permission approval)
			if handleOTPResponse(config, text, chatID, threadID) {
				continue
			}

			// Handle commands
			if strings.HasPrefix(text, "/c ") {
				cmdStr := strings.TrimPrefix(text, "/c ")
				output, err := executeCommand(cmdStr)
				if err != nil {
					output = fmt.Sprintf("⚠️ %s\n\nExit: %v", output, err)
				}
				telegram.SendMessage(config, chatID, threadID, output)
				continue
			}

			if text == "/update" {
				updateCCC(config, chatID, threadID, offset)
				continue
			}

			if text == "/restart" {
				telegram.SendMessage(config, chatID, threadID, "🔄 Restarting ccc service...")
				// Re-exec ourselves to restart cleanly
				go func() {
					time.Sleep(500 * time.Millisecond)
					exe, err := os.Executable()
					if err != nil {
						return
					}
					exec.Command(exe, "listen").Start()
					os.Exit(0)
				}()
				continue
			}

			if text == "/stats" {
				stats := diagnostics.GetSystemStats()
				telegram.SendMessage(config, chatID, threadID, stats)
				continue
			}

			if text == "/version" {
				telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("ccc %s", version))
				continue
			}

			if text == "/auth" {
				go handleAuth(config, chatID, threadID)
				continue
			}

			// /stop command - interrupt current Claude execution
			if text == "/stop" {
				handleStopCommand(config, chatID, threadID, isGroup)
				continue
			}

			// If auth is waiting for code, send it
			if authWaitingCode && !strings.HasPrefix(text, "/") {
				go handleAuthCode(config, chatID, threadID, text)
				continue
			}

			// /continue command - restart session preserving conversation history
			if text == "/continue" && isGroup && threadID > 0 {
				handleContinueCommand(config, chatID, threadID)
				continue
			}

			// /delete command - delete session and thread
			if text == "/delete" && isGroup && threadID > 0 {
				handleDeleteCommand(config, chatID, threadID)
				continue
			}

			// /providers command - list available providers or change session provider
			if text == "/providers" || strings.HasPrefix(text, "/provider") {
				handleProvidersCommand(config, chatID, threadID, text, isGroup)
				continue
			}

			// /resume command - manage Claude session IDs
			if strings.HasPrefix(text, "/resume") && isGroup && threadID > 0 {
				handleResumeCommand(config, chatID, threadID, text)
				continue
			}

			// /cleanup command - delete tmux sessions and Telegram topics (NOT folders)
			if text == "/cleanup" {
				handleCleanupCommand(config, chatID, threadID)
				continue
			}

			// /team command - create team session
			if strings.HasPrefix(text, "/team") && isGroup {
				handleTeamCreateCommand(config, chatID, threadID, text)
				continue
			}

			// /new command - create/restart session
			if strings.HasPrefix(text, "/new") && isGroup {
				handleNewCommand(config, chatID, threadID, text)
				continue
			}

			// /worktree command - create worktree session from existing session
			if strings.HasPrefix(text, "/worktree") && isGroup {
				handleWorktreeCommand(config, chatID, threadID, text)
				continue
			}

			// Check if message is in a topic (interactive session)
			if isGroup && threadID > 0 {
				handleSessionMessage(config, text, chatID, threadID, update.UpdateID)
				continue
			}

			// Private chat: run one-shot Claude
			if !isGroup {
				handlePrivateChat(config, msg, chatID, threadID)
			}
		}
	}
}
