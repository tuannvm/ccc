package main

import (
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// newHandlerCallbacks builds the standard callback struct for hook handlers.
func newHandlerCallbacks() *hooks.HandlerCallbacks {
	return &hooks.HandlerCallbacks{
		LoadConfig:              configpkg.Load,
		SaveConfig:              configpkg.Save,
		FindSession:             findSession,
		PersistClaudeSessionID:  persistClaudeSessionID,
		GetSessionWorkDir:       getSessionWorkDir,
		LoadToolState:           loadToolState,
		SaveToolState:           saveToolState,
		ClearToolState:          clearToolState,
		AddTextToToolState:      addTextToToolState,
		FormatToolMessage:       formatToolMessage,
		CollapseToolMessage:     collapseToolMessage,
		SetThinking:             setThinking,
		ClearThinking:           clearThinking,
		TelegramActiveFlag:      telegramActiveFlag,
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
		IsOTPEnabled:       isOTPEnabled,
		HasValidOTPGrant:   hasValidOTPGrant,
		WriteOTPRequest:    func(sessionID string, req *hooks.OTPPermissionRequest) { writeOTPRequest(sessionID, req) },
		WriteOTPGrant:      writeOTPGrant,
		WaitForOTPResponse: waitForOTPResponse,
		TmuxSafeName:       tmux.SafeName,
		WritePromptAck:     writePromptAck,
		InferRoleFromTranscriptPath: func(transcriptPath string) string {
			return string(inferRoleFromTranscriptPath(transcriptPath))
		},
		ToolInputSummary: toolInputSummary,
		ReadHookStdin:    readHookStdin,
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

// Ensure session.PaneRole string conversion compiles
var _ = string(inferRoleFromTranscriptPath(""))
