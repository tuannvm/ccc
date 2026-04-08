package main

import (
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"
)

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
		// Callbacks from root-level functions
		LoadToolState:      loadToolState,
		AddTextToToolState: addTextToToolState,
		SaveToolState:      saveToolState,
		FormatToolMessage:  formatToolMessage,
		EditMessageHTML:    telegram.EditMessageHTML,
		SendMessageHTML:    sendAssistantMessage,
		SendMessageGetID:   telegram.SendMessageGetID,
		SendMessage:        telegram.SendMessage,
		IsDelivered:        ledger.IsDelivered,
		AppendMessage: func(msg *ledger.MessageRecord) {
			ledger.AppendMessage(msg)
		},
		ClearToolState:              clearToolState,
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

// hookLog writes debug log entries
func hookLog(format string, args ...any) {
	hooks.HookLog(format, args...)
}

// sendAssistantMessage sends an assistant text message with optional streaming
func sendAssistantMessage(cfg *Config, chatID int64, threadID int64, text string) (int64, error) {
	return hooks.SendAssistantMessage(cfg, chatID, threadID, text)
}

// truncate shortens a string to n characters
func truncate(s string, n int) string {
	return hooks.Truncate(s, n)
}
