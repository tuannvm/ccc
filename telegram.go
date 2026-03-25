package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxResponseSize = 10 * 1024 * 1024 // 10MB

// redactTokenError replaces the bot token in error messages with "***"
func redactTokenError(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "***"))
}

// telegramGet performs an HTTP GET and redacts the bot token from any errors
func telegramGet(token string, url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, redactTokenError(err, token)
	}
	return resp, nil
}

// telegramClientGet performs an HTTP GET with a custom client and redacts the bot token from any errors
func telegramClientGet(client *http.Client, token string, url string) (*http.Response, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, redactTokenError(err, token)
	}
	return resp, nil
}

// updateCCC downloads the latest ccc binary from GitHub releases and restarts
func updateCCC(config *Config, chatID, threadID int64, offset int) {
	sendMessage(config, chatID, threadID, "🔄 Updating ccc...")

	binaryName := fmt.Sprintf("ccc-%s-%s", runtime.GOOS, runtime.GOARCH)
	downloadURL := fmt.Sprintf("https://github.com/tuannvm/ccc/releases/latest/download/%s", binaryName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: HTTP %d (no release for %s?)", resp.StatusCode, binaryName))
		return
	}

	tmpPath := cccPath + ".new"
	f, err := os.Create(tmpPath)
	if err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create temp file: %v", err))
		return
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to write binary: %v", err))
		return
	}

	// Validate downloaded binary size (ccc should be > 1MB)
	if written < 1000000 {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Downloaded file too small (%d bytes), aborting", written))
		return
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to chmod: %v", err))
		return
	}

	// Test the new binary before replacing
	testCmd := exec.Command(tmpPath, "version")
	if err := testCmd.Run(); err != nil {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ New binary failed validation: %v", err))
		return
	}

	// Backup old binary
	backupPath := cccPath + ".bak"
	os.Remove(backupPath) // Remove old backup if exists
	if err := os.Rename(cccPath, backupPath); err != nil {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to backup old binary: %v", err))
		return
	}

	// Replace with new binary
	if err := os.Rename(tmpPath, cccPath); err != nil {
		// Restore backup
		os.Rename(backupPath, cccPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to replace binary: %v", err))
		return
	}

	// Codesign on macOS
	if runtime.GOOS == "darwin" {
		if err := exec.Command("codesign", "-f", "-s", "-", cccPath).Run(); err != nil {
			// Restore backup if codesign fails
			os.Remove(cccPath)
			os.Rename(backupPath, cccPath)
			sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Codesign failed: %v", err))
			return
		}
	}

	// Success - remove backup
	os.Remove(backupPath)

	sendMessage(config, chatID, threadID, "✅ Updated. Restarting...")
	// Confirm offset so the /update message is not reprocessed after restart
	http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=1", config.BotToken, offset))
	os.Exit(0)
}

func telegramAPI(config *Config, method string, params url.Values) (*TelegramResponse, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", config.BotToken, method)
	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return nil, redactTokenError(err, config.BotToken)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	var result TelegramResponse
	json.Unmarshal(body, &result)
	return &result, nil
}

func sendMessage(config *Config, chatID int64, threadID int64, text string) error {
	_, err := sendMessageGetID(config, chatID, threadID, text)
	return err
}

// sendMessageGetID sends a message and returns the message ID for later editing
func sendMessageGetID(config *Config, chatID int64, threadID int64, text string) (int64, error) {
	return sendMessageWithMode(config, chatID, threadID, text, "Markdown")
}

// sendMessageHTMLGetID sends a message with HTML parse mode and returns the message ID
func sendMessageHTMLGetID(config *Config, chatID int64, threadID int64, text string) (int64, error) {
	return sendMessageWithMode(config, chatID, threadID, text, "HTML")
}

func sendMessageWithMode(config *Config, chatID int64, threadID int64, text string, parseMode string) (int64, error) {
	const maxLen = 4000

	// Split long messages
	messages := splitMessage(text, maxLen)
	var lastMsgID int64

	for _, msg := range messages {
		params := url.Values{
			"chat_id":    {fmt.Sprintf("%d", chatID)},
			"text":       {msg},
			"parse_mode": {parseMode},
		}
		if threadID > 0 {
			params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
		}

		result, err := telegramAPI(config, "sendMessage", params)
		if err != nil {
			return 0, err
		}
		if !result.OK {
			// If Markdown/HTML parsing fails, retry as plain text
			if strings.Contains(result.Description, "parse entities") && parseMode != "" {
				params.Del("parse_mode")
				params.Set("text", "⚠️\n[this message displayed as plain text, since markdown parse failed]\n\n"+msg)
				result, err = telegramAPI(config, "sendMessage", params)
				if err != nil {
					return 0, err
				}
				if !result.OK {
					return 0, fmt.Errorf("telegram error: %s", result.Description)
				}
			} else {
				return 0, fmt.Errorf("telegram error: %s", result.Description)
			}
		}

		// Extract message_id from result
		if len(result.Result) > 0 {
			var msgResult struct {
				MessageID int64 `json:"message_id"`
			}
			if json.Unmarshal(result.Result, &msgResult) == nil {
				lastMsgID = msgResult.MessageID
			}
		}

		// Small delay between messages to maintain order
		if len(messages) > 1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return lastMsgID, nil
}

// editMessage edits an existing message, sending overflow as new messages
func editMessage(config *Config, chatID int64, messageID int64, threadID int64, text string) error {
	return editMessageWithMode(config, chatID, messageID, threadID, text, "Markdown")
}

// editMessageHTML edits a message using HTML parse mode
func editMessageHTML(config *Config, chatID int64, messageID int64, threadID int64, text string) error {
	return editMessageWithMode(config, chatID, messageID, threadID, text, "HTML")
}

func editMessageWithMode(config *Config, chatID int64, messageID int64, threadID int64, text string, parseMode string) error {
	const maxLen = 4000

	// Split message - first part goes to edit, rest as new messages
	messages := splitMessage(text, maxLen)

	// Edit existing message with first part
	params := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"message_id": {fmt.Sprintf("%d", messageID)},
		"text":       {messages[0]},
		"parse_mode": {parseMode},
	}

	result, err := telegramAPI(config, "editMessageText", params)
	if err != nil {
		return err
	}
	if !result.OK {
		// If edit fails (e.g., message not modified), ignore
		return nil
	}

	// Send remaining parts as new messages
	for i := 1; i < len(messages); i++ {
		time.Sleep(100 * time.Millisecond)
		sendMessage(config, chatID, threadID, messages[i])
	}

	return nil
}

func sendMessageWithKeyboard(config *Config, chatID int64, threadID int64, text string, buttons [][]InlineKeyboardButton) error {
	const maxLen = 4000

	// Split long messages - send all but last as regular messages, last with keyboard
	messages := splitMessage(text, maxLen)

	// Send all but the last message as regular messages
	for i := 0; i < len(messages)-1; i++ {
		sendMessage(config, chatID, threadID, messages[i])
		time.Sleep(100 * time.Millisecond)
	}

	// Send the last message with keyboard
	keyboard := map[string]interface{}{
		"inline_keyboard": buttons,
	}
	keyboardJSON, _ := json.Marshal(keyboard)

	params := url.Values{
		"chat_id":      {fmt.Sprintf("%d", chatID)},
		"text":         {messages[len(messages)-1]},
		"reply_markup": {string(keyboardJSON)},
	}
	if threadID > 0 {
		params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
	}

	result, err := telegramAPI(config, "sendMessage", params)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

func answerCallbackQuery(config *Config, callbackID string) {
	params := url.Values{
		"callback_query_id": {callbackID},
	}
	telegramAPI(config, "answerCallbackQuery", params)
}

func editMessageRemoveKeyboard(config *Config, chatID int64, messageID int, newText string) {
	const maxLen = 4000
	if len(newText) > maxLen {
		newText = newText[:maxLen-3] + "..."
	}

	params := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"message_id": {fmt.Sprintf("%d", messageID)},
		"text":       {newText},
	}
	telegramAPI(config, "editMessageText", params)
}

func sendTypingAction(config *Config, chatID int64, threadID int64) {
	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"action":  {"typing"},
	}
	if threadID > 0 {
		params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
	}
	telegramAPI(config, "sendChatAction", params)
}

func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var messages []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			messages = append(messages, remaining)
			break
		}

		// Find a good split point (newline or space)
		splitAt := maxLen

		// Try to split at a newline first
		if idx := strings.LastIndex(remaining[:maxLen], "\n"); idx > maxLen/2 {
			splitAt = idx + 1
		} else if idx := strings.LastIndex(remaining[:maxLen], " "); idx > maxLen/2 {
			// Fall back to space
			splitAt = idx + 1
		}

		messages = append(messages, strings.TrimRight(remaining[:splitAt], " \n"))
		remaining = remaining[splitAt:]
	}

	return messages
}

// sendFile sends a file to Telegram (max 50MB)
func sendFile(config *Config, chatID int64, threadID int64, filePath string, caption string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add chat_id
	writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if threadID > 0 {
		writer.WriteField("message_thread_id", fmt.Sprintf("%d", threadID))
	}
	if caption != "" {
		writer.WriteField("caption", caption)
	}

	// Add file
	part, err := writer.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		return err
	}
	io.Copy(part, file)
	writer.Close()

	resp, err := http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", config.BotToken),
		writer.FormDataContentType(),
		body,
	)
	if err != nil {
		return redactTokenError(err, config.BotToken)
	}
	defer resp.Body.Close()

	var result TelegramResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

// downloadTelegramFile downloads a file from Telegram
func downloadTelegramFile(config *Config, fileID string, destPath string) error {
	// Get file path from Telegram
	resp, err := telegramGet(config.BotToken, fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", config.BotToken, fileID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("failed to get file path")
	}

	// Download the file
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", config.BotToken, result.Result.FilePath)
	fileResp, err := telegramGet(config.BotToken, fileURL)
	if err != nil {
		return err
	}
	defer fileResp.Body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, fileResp.Body)
	return err
}

// worktreeColor generates a consistent color for worktree sessions based on the base project name.
// All worktrees belonging to the same base project will have the same color, creating visual grouping.
// Returns the Telegram icon_color integer value as a string (Telegram only allows 6 specific colors).
func worktreeColor(baseSessionName string) string {
	// Hash the base name to get a consistent color using FNV-1a algorithm
	var hash uint32 = 2166136261 // FNV offset basis
	for _, c := range baseSessionName {
		hash ^= uint32(c)
		hash *= 16777619 // FNV prime
	}

	// Telegram only allows these 6 specific icon_color values (decimal integers)
	// See: https://core.telegram.org/bots/api#createforumtopic
	colors := []string{
		"7322096",  // Blue (0x6FB9F0)
		"16766590", // Yellow (0xFFD67E)
		"13338331", // Violet (0xCB86DB)
		"9367192",  // Green (0x8EEE98)
		"16749490", // Rose (0xFF93B2)
		"16478047", // Red (0xFB6F5F)
	}
	return colors[hash%uint32(len(colors))]
}

func createForumTopic(config *Config, name string, providerName string, baseSessionName string) (int64, error) {
	// Use createForumTopicWithEmoji for API 9.5 custom emoji support
	// This automatically handles fallback to letter prefix if emoji is not available
	return createForumTopicWithEmoji(config, name, providerName, baseSessionName)
}

func deleteForumTopic(config *Config, topicID int64) error {
	if config.GroupID == 0 {
		return fmt.Errorf("no group configured")
	}

	params := url.Values{
		"chat_id":           {fmt.Sprintf("%d", config.GroupID)},
		"message_thread_id": {fmt.Sprintf("%d", topicID)},
	}

	result, err := telegramAPI(config, "deleteForumTopic", params)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("failed to delete topic: %s", result.Description)
	}

	return nil
}

// setBotCommands sets the bot commands in Telegram
func setBotCommands(botToken string) {
	commands := []map[string]string{
		{"command": "new", "description": "Create/restart session: /new <name>"},
		{"command": "team", "description": "Create team session (3-pane): /team <name>"},
		{"command": "continue", "description": "Restart session with history"},
		{"command": "delete", "description": "Delete current session and thread"},
		{"command": "resume", "description": "List/switch Claude sessions: /resume [id]"},
		{"command": "worktree", "description": "Create worktree session: /worktree <base> <name>"},
		{"command": "providers", "description": "List available AI providers"},
		{"command": "cleanup", "description": "Delete ALL sessions, folders and threads"},
		{"command": "c", "description": "Execute shell command: /c <cmd>"},
		{"command": "update", "description": "Update ccc binary from GitHub"},
		{"command": "version", "description": "Show ccc version"},
		{"command": "stats", "description": "Show system stats (RAM, disk, etc)"},
		{"command": "auth", "description": "Re-authenticate Claude OAuth"},
		{"command": "stop", "description": "Stop/interrupt current Claude execution"},
	}

	// Set for default scope
	defaultBody, _ := json.Marshal(map[string]interface{}{
		"commands": commands,
	})
	resp, err := http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", botToken),
		"application/json",
		bytes.NewReader(defaultBody),
	)
	if err == nil {
		resp.Body.Close()
	}

	// Set for all group chats (makes the / button appear)
	groupBody, _ := json.Marshal(map[string]interface{}{
		"commands": commands,
		"scope":    map[string]string{"type": "all_group_chats"},
	})
	resp, err = http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", botToken),
		"application/json",
		bytes.NewReader(groupBody),
	)
	if err == nil {
		resp.Body.Close()
	}
}

// ========== API 9.5: Telegram Bot API 9.5 Support ==========
// https://core.telegram.org/bots/api#march-1-2026

// DateTimeEntity represents a date_time MessageEntity for API 9.5
// Allows Telegram to display timestamps in the user's locale with automatic formatting
type DateTimeEntity struct {
	Type     string `json:"type"`     // "date_time"
	Offset   int    `json:"offset"`   // UTF-16 code units to start of entity
	Length   int    `json:"length"`   // Length of entity in UTF-16 code units
	UnixTime int64  `json:"unix_time"` // Unix timestamp
	Format   string `json:"date_time_format,omitempty"` // Format string: r|w?[dD]?[tT]?
}

// ForumTopicIcons represents the result of getForumTopicIconStickers
type ForumTopicIcons struct {
	OK     bool      `json:"ok"`
	Result []Sticker `json:"result,omitempty"`
}

// Sticker represents a sticker (simplified for icon stickers)
type Sticker struct {
	FileID        string `json:"file_id"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
	Emoji         string `json:"emoji,omitempty"`
}

// Common date/time format presets for API 9.5 date_time entities
const (
	FormatRelative    = "r"     // Relative time (e.g., "in 5 minutes", "2 hours ago")
	FormatWeekday     = "w"     // Day of week (e.g., "Monday")
	FormatShortDate   = "d"     // Short date (e.g., "17.03.22")
	FormatLongDate    = "D"     // Long date (e.g., "March 17, 2022")
	FormatShortTime   = "t"     // Short time (e.g., "22:45")
	FormatLongTime    = "T"     // Long time (e.g., "22:45:00")
	FormatWeekdayDate = "wd"    // Weekday + short date (e.g., "Monday, 17.03.22")
	FormatWeekdayTime = "wt"    // Weekday + short time (e.g., "Monday, 22:45")
	FormatDateTime    = "wDT"   // Full datetime (e.g., "Monday, March 17, 2022 at 22:45:00")
	FormatShortDT     = "dT"    // Short date + time (e.g., "17.03.22, 22:45")
)

// getForumTopicIconStickers retrieves custom emoji stickers that can be used as forum topic icons
func getForumTopicIconStickers(config *Config) ([]Sticker, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getForumTopicIconStickers", config.BotToken)
	resp, err := telegramGet(config.BotToken, apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ForumTopicIcons
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("failed to get forum topic icons")
	}
	return result.Result, nil
}

// createForumTopicWithEmoji creates a forum topic with a custom emoji icon
// This is an enhanced version of createForumTopic that uses icon_custom_emoji_id instead of letter prefix
func createForumTopicWithEmoji(config *Config, name string, providerName string, baseSessionName string) (int64, error) {
	if config.GroupID == 0 {
		return 0, fmt.Errorf("no group configured")
	}

	// Determine emoji ID based on provider and whether this is a worktree
	var emojiID string
	isWorktree := baseSessionName != ""
	if isWorktree {
		emojiID = getEmojiIDForWorktree(config, baseSessionName)
	} else if providerName != "" {
		emojiID = getEmojiIDForProvider(config, providerName)
	}

	// Build topic name - when using emoji, we don't need the letter prefix
	topicName := name

	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", config.GroupID)},
		"name":    {topicName},
	}

	// Add custom emoji icon if available
	if emojiID != "" {
		params.Add("icon_custom_emoji_id", emojiID)
		// Note: icon_color is mutually exclusive with icon_custom_emoji_id
		// We cannot use both, so we skip icon_color when using custom emoji
	} else {
		// No custom emoji available - use fallbacks
		if providerName != "" && len(providerName) > 0 {
			// Add letter prefix for provider identification
			prefix := strings.ToUpper(string(providerName[0]))
			topicName = fmt.Sprintf("%s %s", prefix, name)
			params.Set("name", topicName)
		}

		// Add icon color for worktree sessions to group them by base project
		// This applies even when providerName is empty (e.g., worktree for default session)
		if baseSessionName != "" {
			params.Add("icon_color", worktreeColor(baseSessionName))
		}
	}

	result, err := telegramAPI(config, "createForumTopic", params)
	if err != nil {
		return 0, err
	}
	if !result.OK {
		return 0, fmt.Errorf("failed to create topic: %s", result.Description)
	}

	var topic TopicResult
	if err := json.Unmarshal(result.Result, &topic); err != nil {
		return 0, fmt.Errorf("failed to parse topic result: %w", err)
	}

	return topic.MessageThreadID, nil
}

// sendMessageWithDateTime sends a message with date_time entities
func sendMessageWithDateTime(config *Config, chatID int64, threadID int64, text string, dateEntities []DateTimeEntity) error {
	// Build the message with entities
	type Message struct {
		ChatID         int64            `json:"chat_id"`
		Text           string           `json:"text"`
		Entities       []DateTimeEntity `json:"entities"`
		MessageThreadID int64           `json:"message_thread_id,omitempty"`
	}

	msg := Message{
		ChatID:   chatID,
		Text:     text,
		Entities: dateEntities,
	}
	if threadID > 0 {
		msg.MessageThreadID = threadID
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken)
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return redactTokenError(err, config.BotToken)
	}
	defer resp.Body.Close()

	var result TelegramResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

// formatDateTimeEntity creates a DateTimeEntity for a given timestamp
// The caller is responsible for providing the correct textLength (in UTF-16 code units)
// that corresponds to the actual placeholder text in the message
func formatDateTimeEntity(timestamp time.Time, format string, offset int, textLength int) DateTimeEntity {
	return DateTimeEntity{
		Type:     "date_time",
		Offset:   offset,
		Length:   textLength,
		UnixTime: timestamp.Unix(),
		Format:   format,
	}
}

// NewDateEntity creates a date entity with a specific format
// textLength is the UTF-16 length of the placeholder text in the message
func NewDateEntity(t time.Time, format string, textOffset int, textLength int) DateTimeEntity {
	return formatDateTimeEntity(t, format, textOffset, textLength)
}

// formatMessageWithTimestamp adds a timestamp with date_time entity to a message
// Returns the formatted message text and the entities array
func formatMessageWithTimestamp(baseMessage string, timestamp time.Time, format string) (string, []DateTimeEntity) {
	// Use a simple placeholder that will be replaced by Telegram's localized display
	// The placeholder text itself doesn't matter much - Telegram uses unix_time for display
	placeholder := "⏰"
	messageWithTime := fmt.Sprintf("%s\n\n📅 %s", baseMessage, placeholder)

	// Calculate UTF-16 offset where the placeholder starts
	// Need to account for: baseMessage + "\n\n" + "📅 "
	baseMsgUtf16 := utf16Len(baseMessage)
	newlineUtf16 := 2 // "\n\n" = 2 UTF-16 units
	emojiUtf16 := 2   // "📅" emoji = 1 UTF-16 surrogate pair = 2 units
	spaceUtf16 := 1   // " " = 1 UTF-16 unit
	offset := baseMsgUtf16 + newlineUtf16 + emojiUtf16 + spaceUtf16

	// Length is the UTF-16 length of the placeholder
	placeholderUtf16 := utf16Len(placeholder)

	entity := DateTimeEntity{
		Type:     "date_time",
		Offset:   offset,
		Length:   placeholderUtf16,
		UnixTime: timestamp.Unix(),
		Format:   format,
	}

	return messageWithTime, []DateTimeEntity{entity}
}

// utf16Len calculates the UTF-16 code unit length of a string
// This is required because Telegram entity offsets are in UTF-16, not bytes
func utf16Len(s string) int {
	len := 0
	for _, r := range s {
		if r >= 0x10000 {
			len += 2 // Surrogate pair for characters outside BMP
		} else {
			len += 1
		}
	}
	return len
}

// sendMessageWithTimestamp sends a message with a timestamp formatted using date_time entity
func sendMessageWithTimestamp(config *Config, chatID int64, threadID int64, baseMessage string, timestamp time.Time, format string) error {
	text, entities := formatMessageWithTimestamp(baseMessage, timestamp, format)
	return sendMessageWithDateTime(config, chatID, threadID, text, entities)
}

// normalizeProviderAlias converts provider aliases to canonical names
// This ensures that custom emoji lookups work regardless of which alias was used
func normalizeProviderAlias(providerName string) string {
	// Map common aliases to canonical provider names
	// These should match the documented aliases in API_9_5_FEATURES.md
	switch strings.ToLower(providerName) {
	case "z":
		return "zai"
	case "d", "ds":
		return "deepseek"
	case "m":
		return "minimax"
	case "c", "anthropic":
		return "claude"
	default:
		return strings.ToLower(providerName)
	}
}

// getEmojiIDForProvider returns the custom emoji ID for a given provider
// Only returns non-empty if the user has explicitly configured a custom emoji ID
// Supports provider aliases (z→zai, d/ds→deepseek, m→minimax, c/anthropic→claude)
func getEmojiIDForProvider(config *Config, providerName string) string {
	if providerName == "" {
		return ""
	}

	// Normalize the provider name to handle aliases
	canonicalName := normalizeProviderAlias(providerName)

	// Only use user-configured custom emoji IDs
	// Built-in placeholder constants are NOT returned to avoid API errors
	if config.CustomEmojiIDs != nil {
		// Try canonical name first
		if emojiID, ok := config.CustomEmojiIDs[canonicalName]; ok && emojiID != "" {
			return emojiID
		}
		// Fall back to original providerName (in case user configured with alias)
		if emojiID, ok := config.CustomEmojiIDs[providerName]; ok && emojiID != "" {
			return emojiID
		}
	}
	return "" // No valid custom emoji configured - will fall back to letter prefix
}

// getEmojiIDForWorktree returns a consistent emoji ID for worktree sessions
// Only returns non-empty if the user has explicitly configured a custom emoji ID
func getEmojiIDForWorktree(config *Config, baseSessionName string) string {
	// Only use user-configured custom emoji IDs
	// Built-in placeholder constants are NOT returned to avoid API errors
	if config.CustomEmojiIDs != nil {
		if emojiID, ok := config.CustomEmojiIDs["worktree"]; ok && emojiID != "" {
			return emojiID
		}
	}
	return "" // No valid custom emoji configured - will fall back to letter prefix
}

// ========== API 9.5: Streaming with sendMessageDraft ==========

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

// ========== Streaming Integration Helpers ==========

// sendStreamingMessage sends a message with optional streaming
// If enableStreaming is true, uses sendMessageDraft for real-time typing effect
// Otherwise, falls back to standard sendMessageGetID
func sendStreamingMessage(config *Config, chatID int64, threadID int64, text string, enableStreaming bool) (int64, error) {
	if !enableStreaming {
		// Fallback to standard message sending
		return sendMessageGetID(config, chatID, threadID, text)
	}

	// Streaming mode: send as single chunk then finalize
	chunks := make(chan string, 1)
	chunks <- text
	close(chunks)

	return streamAndFinalize(config, chatID, threadID, chunks, "Markdown")
}

// BufferedStreamer accumulates text chunks and streams them
// Use this when you receive multiple text blocks and want to stream them as one response
type BufferedStreamer struct {
	config         *Config
	chatID         int64
	threadID       int64
	enabled        bool
	textBuilder    strings.Builder
	chunkChan      chan string
	flushTimer     *time.Timer
	flushInterval  time.Duration
	doneChan       chan bool         // Signals goroutine to stop
	finalizeDone   chan struct{}     // Signals when finalization is complete
	lastFlush      time.Time
	minChunkSize   int
	finalMessageID int64             // Stores the message ID from finalization
	finalizeErr    error             // Stores finalization error
	mu             sync.Mutex        // Protects finalMessageID, finalizeErr, and concurrent Add
}

// NewBufferedStreamer creates a new buffered streamer
func NewBufferedStreamer(config *Config, chatID int64, threadID int64, enabled bool) *BufferedStreamer {
	bs := &BufferedStreamer{
		config:        config,
		chatID:        chatID,
		threadID:      threadID,
		enabled:       enabled,
		chunkChan:     make(chan string, 100),
		doneChan:      make(chan bool),
		finalizeDone:  make(chan struct{}),
		flushInterval: 100 * time.Millisecond, // Flush every 100ms
		minChunkSize:  10,                    // Or after 10 characters
	}
	return bs
}

// Start begins the background streaming goroutine
func (bs *BufferedStreamer) Start() {
	if !bs.enabled {
		return // Streaming disabled
	}

	go func() {
		ticker := time.NewTicker(bs.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case chunk := <-bs.chunkChan:
				bs.textBuilder.WriteString(chunk)
				bs.lastFlush = time.Now()

				// Flush if we have enough content or timer fires
				if bs.textBuilder.Len() >= bs.minChunkSize {
					bs.sendDraft()
				}

			case <-ticker.C:
				// Periodic flush
				if time.Since(bs.lastFlush) >= bs.flushInterval && bs.textBuilder.Len() > 0 {
					bs.sendDraft()
				}

			case <-bs.doneChan:
				// Done - drain remaining chunks first, then finalize
				for {
					select {
					case chunk := <-bs.chunkChan:
						bs.textBuilder.WriteString(chunk)
					default:
						// No more chunks - finalize and exit
						bs.finalize()
						close(bs.finalizeDone)
						return
					}
				}
			}
		}
	}()
}

// Add adds a text chunk to the stream
// Safe for concurrent use - multiple goroutines can call Add() simultaneously
func (bs *BufferedStreamer) Add(text string) {
	// Always buffer text, even when streaming disabled
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if !bs.enabled {
		// Streaming disabled - just accumulate in buffer
		bs.textBuilder.WriteString(text)
		return
	}

	// Non-blocking send - if channel is full, drop the chunk
	// This prevents deadlock if Done() has already closed doneChan
	select {
	case bs.chunkChan <- text:
		// Chunk sent successfully
	default:
		// Channel full or goroutine exited - silently drop
	}
}

// sendDraft sends the current accumulated text as a draft update
func (bs *BufferedStreamer) sendDraft() {
	if bs.textBuilder.Len() == 0 {
		return
	}

	currentText := bs.textBuilder.String()
	if err := sendDraftMessage(bs.config, bs.chatID, bs.threadID, currentText); err != nil {
		// Draft failures are non-critical
	}
}

// finalize converts the draft to a permanent message and stores the message ID
func (bs *BufferedStreamer) finalize() {
	finalText := bs.textBuilder.String()
	if finalText == "" {
		return
	}

	msgID, err := finalizeStream(bs.config, bs.chatID, bs.threadID, finalText, "Markdown")

	// Store the message ID and error for Done() to retrieve
	bs.mu.Lock()
	bs.finalMessageID = msgID
	bs.finalizeErr = err
	bs.mu.Unlock()
}

// Done finalizes the stream and returns the message ID
func (bs *BufferedStreamer) Done() (int64, error) {
	if !bs.enabled {
		// Not streaming enabled - send normally
		if bs.textBuilder.Len() == 0 {
			return 0, nil
		}
		return sendMessageGetID(bs.config, bs.chatID, bs.threadID, bs.textBuilder.String())
	}

	// Signal goroutine to finalize (goroutine owns channel lifecycle)
	close(bs.doneChan)

	// Wait for finalization to complete (proper synchronization)
	<-bs.finalizeDone

	// Return the stored message ID and error from finalize()
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.finalMessageID, bs.finalizeErr
}
