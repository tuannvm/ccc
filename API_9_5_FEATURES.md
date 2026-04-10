# Telegram Bot API 9.5 Features

This document describes the new Telegram Bot API 9.5 features integrated into ccc.

## Overview

Telegram Bot API 9.5 (released March 1, 2026) introduced several new features that enhance the ccc bot experience:

- **date_time MessageEntity**: Localized date/time display with automatic formatting
- **icon_custom_emoji_id**: Custom emoji icons for forum topics
- **sender_tag**: Member tagging system for multi-user mode
- **sendMessageDraft**: Real-time streaming for AI responses (typing effect)

## Implementation

All API 9.5 features are implemented in `pkg/telegram/` (api.go, types.go).

## date_time MessageEntity

### What it does

The `date_time` entity allows Telegram to display timestamps in the user's locale with automatic formatting. Instead of showing raw timestamps like "2026-03-21 14:30:00", you can show:
- Relative time: "in 5 minutes", "2 hours ago"
- Formatted: "Monday, 22:45", "March 17, 2022 at 22:45:00"

### Format Codes

| Format | Example | Description |
|--------|---------|-------------|
| `r` | "in 5 minutes" | Relative time |
| `w` | "Monday" | Day of week |
| `d` | "17.03.22" | Short date |
| `D` | "March 17, 2022" | Long date |
| `t` | "22:45" | Short time |
| `T` | "22:45:00" | Long time |
| `wd` | "Monday, 17.03.22" | Weekday + short date |
| `wt` | "Monday, 22:45" | Weekday + short time |
| `wDT` | "Monday, March 17, 2022 at 22:45:00" | Full datetime |
| `dT` | "17.03.22, 22:45" | Short date + time |

### Usage

```go
// Send message with timestamp
sendMessageWithTimestamp(
    config,
    chatID,
    threadID,
    "🚀 Session started",
    time.Now(),
    FormatWeekdayTime, // "Monday, 22:45"
)
```

## Custom Emoji Icons (icon_custom_emoji_id)

### What it does

Forum topics can now use custom emoji as icons instead of just letter prefixes and colors. This makes topics more visually distinct and brandable.

### Configuration

Add custom emoji IDs to your config:

```json
{
  "custom_emoji_ids": {
    "zai": "5372874709367178364",
    "deepseek": "5372874709367178365",
    "minimax": "5372874709367178366",
    "claude": "5372874709367178367",
    "worktree": "5372874709367178368"
  }
}
```

### Getting Emoji IDs

1. **Use Telegram's built-in forum icons:**
   ```go
   stickers, _ := getForumTopicIconStickers(config)
   ```

2. **Add custom emoji to your group:**
   - Open your group info
   - Go to "Emoji" section
   - Add custom emoji stickers
   - Use the emoji ID from the sticker

### Provider Mappings

| Provider | Default Emoji |
|----------|---------------|
| zai, z | 🤖 Robot face |
| deepseek, d, ds | 🧠 Brain |
| minimax, m | ✨ Sparkles |
| claude, c, anthropic | 🤔 Thinking face |
| worktree | 🌳 Tree/branch |

## sender_tag Field

### What it does

The `sender_tag` field in messages allows member tagging in multi-user mode. This enables:
- Tagging specific users in group messages
- Permission management based on tags
- Organized multi-user workflows

### Implementation

The `sender_tag` field is now included in the `TelegramMessage` struct:

```go
type TelegramMessage struct {
    // ... other fields
    SenderTag string `json:"sender_tag,omitempty"`
}
```

## Migration Guide

### From letter prefixes to custom emoji

**Before:**
```
Z my-session
D my-other-session
```

**After (with custom emoji):**
```
🤖 my-session
🧠 my-other-session
```

### Adding timestamps to messages

**Before:**
```go
sendMessage(config, chatID, threadID, "Session started at 2026-03-21 14:30:00")
```

**After:**
```go
sendMessageWithTimestamp(config, chatID, threadID, "Session started", time.Now(), FormatWeekdayTime)
// Shows: "Session started\n📅 Monday, 22:45"
```

## API Reference

### pkg/telegram/ (API 9.5 functions)

```go
// DateTimeEntity represents a date_time MessageEntity
type DateTimeEntity struct {
    Type     string `json:"type"`     // "date_time"
    Offset   int    `json:"offset"`   // UTF-16 offset
    Length   int    `json:"length"`   // UTF-16 length
    UnixTime int64  `json:"unix_time"` // Unix timestamp
    Format   string `json:"date_time_format,omitempty"` // Format string
}

// sendMessageWithDateTime sends a message with date_time entities
func sendMessageWithDateTime(config *Config, chatID int64, threadID int64, text string, dateEntities []DateTimeEntity) error

// sendMessageWithTimestamp sends a message with formatted timestamp
func sendMessageWithTimestamp(config *Config, chatID int64, threadID int64, baseMessage string, timestamp time.Time, format string) error

// createForumTopicWithEmoji creates a topic with custom emoji icon
func createForumTopicWithEmoji(config *Config, name string, providerName string, baseSessionName string) (int64, error)

// getForumTopicIconStickers retrieves available custom emoji stickers
func getForumTopicIconStickers(config *Config) ([]Sticker, error)

// utf16Len calculates UTF-16 code unit length for Telegram entity offsets
func utf16Len(s string) int

// sendMessageDraft sends a streaming draft message (API 9.5)
func sendDraftMessage(config *Config, chatID int64, threadID int64, text string) error

// streamResponse streams AI response using sendMessageDraft for real-time typing effect
func streamResponse(config *Config, chatID int64, threadID int64, chunks <-chan StreamChunk, done <-chan bool) (string, error)

// finalizeStream converts a draft message to a permanent message
func finalizeStream(config *Config, chatID int64, threadID int64, text string, parseMode string) (int64, error)

// streamAndFinalize combines streaming and finalization in one call
func streamAndFinalize(config *Config, chatID int64, threadID int64, text <-chan string, parseMode string) (int64, error)
```

## sendMessageDraft - Streaming Responses (API 9.5)

### What it does

The `sendMessageDraft` method enables real-time streaming of AI responses with a typing effect. Unlike the old approach using `editMessageText`, draft updates:
- Don't show an "edited" tag
- Have much higher rate limits
- Are silent (no notifications)
- Provide smooth, native typing animation

### Streaming Workflow

```
1. sendDraftMessage("Think")     → User sees: "Think"
2. sendDraftMessage("Thinking")  → User sees: "Thinking" (smooth)
3. sendDraftMessage("Thinking about it...")
4. sendMessage(final_text)       → Converts to permanent message
```

### Usage

```go
// Simple streaming with text channel
textChan := make(chan string)
go func() {
    defer close(textChan)
    for _, chunk := range streamClaudeResponse() {
        textChan <- chunk
    }
}()

streamAndFinalize(config, chatID, threadID, textChan, "Markdown")
```

### Advanced Usage

```go
// Full control with StreamChunk
chunks := make(chan StreamChunk)
done := make(chan bool)

go func() {
    defer close(chunks)
    for {
        select {
        case chunk := <-streamSource:
            chunks <- StreamChunk{Text: chunk}
        case <-timeout:
            chunks <- StreamChunk{Done: true}
            return
        }
    }
}()

// Stream the response
finalText, err := streamResponse(config, chatID, threadID, chunks, done)

// Finalize with permanent message
finalizeStream(config, chatID, threadID, finalText, "Markdown")
```

### Important Notes

- **Must finalize**: Always call `finalizeStream()` after streaming completes to convert draft to permanent message
- **Markdown safety**: Stream plain text, only apply Markdown in final `finalizeStream()` call
- **Throttling**: Updates are throttled to 50ms intervals or 5+ tokens to avoid rate limits
- **Error handling**: Draft update failures are non-critical and logged but don't interrupt the stream

## Changelog

### Version 9.5.0 (2026-03-21)

- Added `date_time` MessageEntity support for localized timestamps
- Added `icon_custom_emoji_id` support for forum topics
- Added `sender_tag` field to `TelegramMessage` struct
- Added `CustomEmojiIDs` config field for user-configurable emoji IDs
- Added `sendMessageDraft` for real-time streaming AI responses
- Added `sendDraftMessage()`, `streamResponse()`, `finalizeStream()`, `streamAndFinalize()` helper functions
- Added helper functions for timestamp formatting
- Added emoji ID mappings for common providers
- All API 9.5 features consolidated in `pkg/telegram/`
