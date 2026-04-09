package main

import (
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/pkg/auth"
)

// newHandlerCallbacks builds the standard callback struct for hook handlers.
func newHandlerCallbacks() *hooks.HandlerCallbacks {
	return &hooks.HandlerCallbacks{
		LoadConfig:              configpkg.Load,
		SaveConfig:              configpkg.Save,
		FindSession:             findSession,
		PersistClaudeSessionID:  persistClaudeSessionID,
		GetSessionWorkDir:       getSessionWorkDir,
		LoadToolState:           hooks.LoadToolState,
		SaveToolState:           hooks.SaveToolState,
		ClearToolState:          hooks.ClearToolState,
		AddTextToToolState:      hooks.AddTextToToolState,
		FormatToolMessage:       hooks.FormatToolMessage,
		SetThinking:             hooks.SetThinking,
		ClearThinking:           hooks.ClearThinking,
		TelegramActiveFlag:      hooks.TelegramActiveFlag,
		SendMessage:             telegram.SendMessage,
		SendMessageHTML:         telegram.SendMessageHTMLGetID,
		SendMessageGetID:        telegram.SendMessageGetID,
		EditMessageHTML:         telegram.EditMessageHTML,
		SendMessageWithKeyboard: telegram.SendMessageWithKeyboard,
		IsDelivered:             ledger.IsDelivered,
		AppendMessage:           func(msg *ledger.MessageRecord) { ledger.AppendMessage(msg) },
		UpdateDelivery: func(sessName, msgID, field string, value bool) {
			ledger.UpdateDelivery(sessName, msgID, field, value)
		},
		IsOTPEnabled:       auth.IsOTPEnabled,
		HasValidOTPGrant:   auth.HasValidOTPGrant,
		WriteOTPRequest:    func(sessionID string, req *hooks.OTPPermissionRequest) { auth.WriteOTPRequest(sessionID, req) },
		WriteOTPGrant:      auth.WriteOTPGrant,
		WaitForOTPResponse: auth.WaitForOTPResponse,
		TmuxSafeName:       tmux.SafeName,
		WritePromptAck:     hooks.WritePromptAck,
		InferRoleFromTranscriptPath: func(transcriptPath string) string {
			return string(inferRoleFromTranscriptPath(transcriptPath))
		},
		ToolInputSummary: hooks.ToolInputSummary,
		ReadHookStdin:    hooks.ReadHookStdin,
		CCCPath:          tmux.CCCPath,
	}
}

func handleStopHook() error {
	return hooks.HandleStopHook(newHandlerCallbacks())
}

func handlePermissionHook() error {
	return hooks.HandlePermissionHook(newHandlerCallbacks())
}

func handleUserPromptHook() error {
	return hooks.HandleUserPromptHook(newHandlerCallbacks())
}

func handlePostToolHook() error {
	return hooks.HandlePostToolHook(newHandlerCallbacks())
}

func handleNotificationHook() error {
	return hooks.HandleNotificationHook(newHandlerCallbacks())
}

// deliverUnsentTexts scans transcript tail and sends any assistant text
// blocks not yet delivered to Telegram (using ledger dedup).
func deliverUnsentTexts(cfg *Config, sessName string, topicID int64, transcriptPath string, insertIntoToolMsg bool, claudeSessionID string) int {
	return hooks.DeliverUnsentTexts(&hooks.DeliverUnsentTextsConfig{
		Config:            cfg,
		SessionName:       sessName,
		TopicID:           topicID,
		TranscriptPath:    transcriptPath,
		InsertIntoToolMsg: insertIntoToolMsg,
		ClaudeSessionID:   claudeSessionID,
		LoadToolState:      hooks.LoadToolState,
		AddTextToToolState: hooks.AddTextToToolState,
		SaveToolState:      hooks.SaveToolState,
		FormatToolMessage:  hooks.FormatToolMessage,
		EditMessageHTML:    telegram.EditMessageHTML,
		SendMessageHTML:    hooks.SendAssistantMessage,
		SendMessageGetID:   telegram.SendMessageGetID,
		SendMessage:        telegram.SendMessage,
		IsDelivered:        ledger.IsDelivered,
		AppendMessage: func(msg *ledger.MessageRecord) {
			ledger.AppendMessage(msg)
		},
		ClearToolState:              hooks.ClearToolState,
		InferRoleFromTranscriptPath: inferRoleFromTranscriptPath,
	})
}

// handleStopRetry retries transcript reading 3 times at 2-second intervals
func handleStopRetry(sessName string, topicID int64, transcriptPath string) error {
	return hooks.HandleStopRetry(&hooks.HandleStopRetryConfig{
		SessionName:        sessName,
		TopicID:            topicID,
		TranscriptPath:     transcriptPath,
		LoadConfig:         configpkg.Load,
		DeliverUnsentTexts: deliverUnsentTexts,
	})
}

// --- Hook install delegates (merged from hook_install.go) ---

func installSkill() error {
	return hooks.InstallSkill()
}

func installHooksToCurrentDir() error {
	return hooks.InstallHooksToCurrentDir()
}

func ensureHooksForSession(config *Config, sessionName string, sessionInfo *SessionInfo) error {
	return hooks.EnsureHooksForSession(&hooks.EnsureHooksForSessionConfig{
		Config:            config,
		SessionName:       sessionName,
		SessionInfo:       sessionInfo,
		GetSessionWorkDir: getSessionWorkDir,
	})
}
