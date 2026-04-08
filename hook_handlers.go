package main

import (
	"github.com/tuannvm/ccc/pkg/hooks"
)

// newHandlerCallbacks builds the standard callback struct for hook handlers.
func newHandlerCallbacks() *hooks.HandlerCallbacks {
	return &hooks.HandlerCallbacks{
		LoadConfig:     loadConfig,
		SaveConfig:     saveConfig,
		FindSession:    findSession,
		PersistClaudeSessionID: persistClaudeSessionID,
		GetSessionWorkDir:      getSessionWorkDir,
		LoadToolState:      loadToolState,
		SaveToolState:      saveToolState,
		ClearToolState:     clearToolState,
		AddTextToToolState: addTextToToolState,
		FormatToolMessage:  formatToolMessage,
		CollapseToolMessage: collapseToolMessage,
		SetThinking:    setThinking,
		ClearThinking:  clearThinking,
		TelegramActiveFlag: telegramActiveFlag,
		SendMessage:          sendMessage,
		SendMessageHTML:      sendMessageHTMLGetID,
		SendMessageGetID:     sendMessageGetID,
		EditMessageHTML:      editMessageHTML,
		SendMessageWithKeyboard: sendMessageWithKeyboard,
		IsDelivered:  isDelivered,
		AppendMessage: func(msg *MessageRecord) { appendMessage(msg) },
		UpdateDelivery: func(sessName, msgID, field string, value bool) {
			updateDelivery(sessName, msgID, field, value)
		},
		IsOTPEnabled:      isOTPEnabled,
		HasValidOTPGrant:  hasValidOTPGrant,
		WriteOTPRequest:   func(sessionID string, req *hooks.OTPPermissionRequest) { writeOTPRequest(sessionID, req) },
		WriteOTPGrant:     writeOTPGrant,
		WaitForOTPResponse: waitForOTPResponse,
		TmuxSafeName:      tmuxSafeName,
		WritePromptAck:    writePromptAck,
		InferRoleFromTranscriptPath: func(transcriptPath string) string {
			return string(inferRoleFromTranscriptPath(transcriptPath))
		},
		ToolInputSummary: toolInputSummary,
		ReadHookStdin:    readHookStdin,
		CCCPath:          cccPath,
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
