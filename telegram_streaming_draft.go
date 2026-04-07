package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// sendDraftMessage sends a streaming draft message (API 9.5)
// Unlike editMessageText, draft updates don't show "edited" tag and have higher rate limits
func sendDraftMessage(config *Config, chatID int64, threadID int64, text string) error {
	const maxDraftLen = 4096 // Telegram message length limit

	// Truncate if over limit (draft updates must fit within message size limit)
	// Use rune count for the check to handle multilingual text and emoji correctly
	// CJK characters and emoji use multiple bytes but count as single characters
	runes := []rune(text)
	if len(runes) > maxDraftLen-3 {
		text = string(runes[:maxDraftLen-3]) + "..."
	}

	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"text":    {text},
	}
	if threadID > 0 {
		params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
	}

	result, err := telegramAPI(config, "sendMessageDraft", params)
	if err != nil {
		// Draft failures are non-critical - log but don't fail the stream
		return fmt.Errorf("draft update failed: %w", err)
	}
	if !result.OK && !strings.Contains(result.Description, "message is not modified") {
		// Ignore "not modified" errors (content hasn't changed)
		return fmt.Errorf("draft error: %s", result.Description)
	}
	return nil
}

// StreamChunk represents a chunk of text to be streamed
type StreamChunk struct {
	Text string
	Done bool // Stream is complete
}

// streamResponse streams AI response using sendMessageDraft for real-time typing effect
// IMPORTANT: Callers MUST call finalizeStream() after streaming is complete to convert draft to permanent message
// Returns the final message ID for potential further edits
func streamResponse(config *Config, chatID int64, threadID int64, chunks <-chan StreamChunk, done <-chan bool) (string, error) {
	var fullText strings.Builder
	var lastText string
	// Initialize to zero time so first update can be sent immediately
	lastUpdate := time.Time{}

	// Throttle: don't send updates more frequently than this
	const minUpdateInterval = 50 * time.Millisecond
	// Also throttle by token count to avoid excessive API calls
	const minTokensPerUpdate = 5

	tokensSinceLastUpdate := 0

	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				// Channel closed - send final draft update before returning
				finalText := fullText.String()
				if finalText != lastText {
					sendDraftMessage(config, chatID, threadID, finalText)
				}
				return finalText, nil
			}

			fullText.WriteString(chunk.Text)
			tokensSinceLastUpdate += len(chunk.Text)

			// Check if we should send an update
			shouldUpdate := tokensSinceLastUpdate >= minTokensPerUpdate &&
				time.Since(lastUpdate) >= minUpdateInterval

			if shouldUpdate {
				currentText := fullText.String()
				if currentText != lastText {
					// Send draft update (fail silently to avoid interrupting stream)
					if err := sendDraftMessage(config, chatID, threadID, currentText); err != nil {
						// Log but continue - draft updates are best-effort
						// Errors are common during rapid streaming and shouldn't break the flow
					}
					lastText = currentText
					lastUpdate = time.Now()
					tokensSinceLastUpdate = 0
				}
			}

			if chunk.Done {
				// Stream complete - send final draft update before returning
				finalText := fullText.String()
				if finalText != lastText {
					sendDraftMessage(config, chatID, threadID, finalText)
				}
				return finalText, nil
			}

		case <-done:
			// Early termination requested - send final draft update
			finalText := fullText.String()
			if finalText != lastText {
				sendDraftMessage(config, chatID, threadID, finalText)
			}
			return finalText, nil
		}
	}
}

// finalizeStream converts a draft message to a permanent message
// This MUST be called after streamResponse completes to make the message permanent
func finalizeStream(config *Config, chatID int64, threadID int64, text string, parseMode string) (int64, error) {
	// Send the final message as a permanent message
	// This replaces the draft with a real message that can be replied to, forwarded, etc.
	return sendMessageWithMode(config, chatID, threadID, text, parseMode)
}

// streamAndFinalize combines streaming and finalization in one call
// Use this for simple streaming where you have the full text available incrementally
func streamAndFinalize(config *Config, chatID int64, threadID int64, text <-chan string, parseMode string) (int64, error) {
	// Convert string channel to StreamChunk channel
	chunks := make(chan StreamChunk)
	go func() {
		defer close(chunks)
		for chunk := range text {
			chunks <- StreamChunk{Text: chunk}
		}
	}()

	// Stream the response
	finalText, err := streamResponse(config, chatID, threadID, chunks, make(chan bool))
	if err != nil {
		return 0, err
	}

	// Finalize with permanent message
	return finalizeStream(config, chatID, threadID, finalText, parseMode)
}

// sendStreamingMessage sends a message with optional streaming
// If enableStreaming is true, uses sendMessageDraft for real-time typing effect
// Otherwise, falls back to standard sendMessageGetID
func sendStreamingMessage(config *Config, chatID int64, threadID int64, text string, enableStreaming bool) (int64, error) {
	if !enableStreaming {
		// Fallback to standard message sending
		return sendMessageGetID(config, chatID, threadID, text)
	}

	// For long messages, break into chunks to show progress
	// Short messages are sent as-is for efficiency
	const chunkSize = 100 // Send every 100 characters for typing effect

	if len(text) <= chunkSize {
		// Short message - send as single chunk
		chunks := make(chan string, 1)
		chunks <- text
		close(chunks)
		return streamAndFinalize(config, chatID, threadID, chunks, "Markdown")
	}

	// Long message - stream incrementally
	chunks := make(chan string, 10)
	go func() {
		defer close(chunks)
		runes := []rune(text)
		for i := 0; i < len(runes); i += chunkSize {
			end := min(i+chunkSize, len(runes))
			chunks <- string(runes[i:end])
			// Small delay between chunks for natural typing effect
			time.Sleep(20 * time.Millisecond)
		}
	}()

	return streamAndFinalize(config, chatID, threadID, chunks, "Markdown")
}
