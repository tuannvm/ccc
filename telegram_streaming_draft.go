package main

import (
	"github.com/tuannvm/ccc/pkg/telegram"
)

// sendDraftMessage sends a streaming draft message (API 9.5)
func sendDraftMessage(config *Config, chatID int64, threadID int64, text string) error {
	return telegram.SendDraftMessage(config, chatID, threadID, text)
}

// streamResponse streams AI response using sendMessageDraft for real-time typing effect
func streamResponse(config *Config, chatID int64, threadID int64, chunks <-chan telegram.StreamChunk, done <-chan bool) (string, error) {
	return telegram.StreamResponse(config, chatID, threadID, chunks, done)
}

// finalizeStream converts a draft message to a permanent message
func finalizeStream(config *Config, chatID int64, threadID int64, text string, parseMode string) (int64, error) {
	return telegram.FinalizeStream(config, chatID, threadID, text, parseMode)
}

// streamAndFinalize combines streaming and finalization in one call
func streamAndFinalize(config *Config, chatID int64, threadID int64, text <-chan string, parseMode string) (int64, error) {
	return telegram.StreamAndFinalize(config, chatID, threadID, text, parseMode)
}

// sendStreamingMessage sends a message with optional streaming
func sendStreamingMessage(config *Config, chatID int64, threadID int64, text string, enableStreaming bool) (int64, error) {
	return telegram.SendStreamingMessage(config, chatID, threadID, text, enableStreaming)
}
