package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/config"
)

const MaxResponseSize = 10 * 1024 * 1024 // 10MB

// TelegramAPI performs a Telegram API call
func TelegramAPI(cfg *config.Config, method string, params url.Values) (*TelegramResponse, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", cfg.BotToken, method)
	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return nil, RedactTokenError(err, cfg.BotToken)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseSize))
	var result TelegramResponse
	json.Unmarshal(body, &result)
	return &result, nil
}

func SendMessage(cfg *config.Config, chatID int64, threadID int64, text string) error {
	_, err := SendMessageGetID(cfg, chatID, threadID, text)
	return err
}

// SendMessageGetID sends a message and returns the message ID for later editing
func SendMessageGetID(cfg *config.Config, chatID int64, threadID int64, text string) (int64, error) {
	return SendMessageWithMode(cfg, chatID, threadID, text, "Markdown")
}

// SendMessageHTMLGetID sends a message with HTML parse mode and returns the message ID
func SendMessageHTMLGetID(cfg *config.Config, chatID int64, threadID int64, text string) (int64, error) {
	return SendMessageWithMode(cfg, chatID, threadID, text, "HTML")
}

func SendMessageWithMode(cfg *config.Config, chatID int64, threadID int64, text string, parseMode string) (int64, error) {
	const maxLen = 4000

	// Split long messages
	messages := splitMessage(text, maxLen)
	var lastMsgID int64

	for _, msg := range messages {
		params := url.Values{
			"chat_id": {fmt.Sprintf("%d", chatID)},
			"text":    {msg},
		}
		if parseMode != "" {
			params.Set("parse_mode", parseMode)
		}
		if threadID > 0 {
			params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
		}

		result, err := TelegramAPI(cfg, "sendMessage", params)
		if err != nil {
			return 0, err
		}
		if !result.OK {
			// If Markdown/HTML parsing fails, retry as plain text
			if strings.Contains(result.Description, "parse entities") && parseMode != "" {
				params.Del("parse_mode")
				params.Set("text", "⚠️\n[this message displayed as plain text, since markdown parse failed]\n\n"+msg)
				result, err = TelegramAPI(cfg, "sendMessage", params)
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

func SendPlainMessageGetID(cfg *config.Config, chatID int64, threadID int64, text string) (int64, error) {
	return SendMessageWithMode(cfg, chatID, threadID, text, "")
}

func EditPlainMessage(cfg *config.Config, chatID int64, messageID int64, text string) error {
	const maxLen = 4000
	messages := splitMessage(text, maxLen)

	params := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"message_id": {fmt.Sprintf("%d", messageID)},
		"text":       {messages[0]},
	}

	result, err := TelegramAPI(cfg, "editMessageText", params)
	if err != nil {
		return err
	}
	if !result.OK {
		if strings.Contains(result.Description, "message is not modified") {
			return nil
		}
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

func PinMessage(cfg *config.Config, chatID int64, messageID int64) error {
	params := url.Values{
		"chat_id":              {fmt.Sprintf("%d", chatID)},
		"message_id":           {fmt.Sprintf("%d", messageID)},
		"disable_notification": {"true"},
	}

	result, err := TelegramAPI(cfg, "pinChatMessage", params)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

// EditMessageHTML edits a message using HTML parse mode
func EditMessageHTML(cfg *config.Config, chatID int64, messageID int64, threadID int64, text string) error {
	return EditMessageWithMode(cfg, chatID, messageID, threadID, text, "HTML")
}

func EditMessageWithMode(cfg *config.Config, chatID int64, messageID int64, threadID int64, text string, parseMode string) error {
	const maxLen = 4000

	// Split message - first part goes to edit, rest as new messages
	messages := splitMessage(text, maxLen)

	// Edit existing message with first part
	params := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"message_id": {fmt.Sprintf("%d", messageID)},
		"text":       {messages[0]},
	}
	if parseMode != "" {
		params.Set("parse_mode", parseMode)
	}

	result, err := TelegramAPI(cfg, "editMessageText", params)
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
		SendMessage(cfg, chatID, threadID, messages[i])
	}

	return nil
}

func SendMessageWithKeyboard(cfg *config.Config, chatID int64, threadID int64, text string, buttons [][]InlineKeyboardButton) error {
	const maxLen = 4000

	// Split long messages - send all but last as regular messages, last with keyboard
	messages := splitMessage(text, maxLen)

	// Send all but the last message as regular messages
	for i := range len(messages) - 1 {
		SendMessage(cfg, chatID, threadID, messages[i])
		time.Sleep(100 * time.Millisecond)
	}

	// Send the last message with keyboard
	keyboard := map[string]any{
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

	result, err := TelegramAPI(cfg, "sendMessage", params)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

func AnswerCallbackQuery(cfg *config.Config, callbackID string) {
	params := url.Values{
		"callback_query_id": {callbackID},
	}
	TelegramAPI(cfg, "answerCallbackQuery", params)
}

func EditMessageRemoveKeyboard(cfg *config.Config, chatID int64, messageID int, newText string) {
	const maxLen = 4000
	if len(newText) > maxLen {
		newText = newText[:maxLen-3] + "..."
	}

	params := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"message_id": {fmt.Sprintf("%d", messageID)},
		"text":       {newText},
	}
	TelegramAPI(cfg, "editMessageText", params)
}

func SendTypingAction(cfg *config.Config, chatID int64, threadID int64) {
	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"action":  {"typing"},
	}
	if threadID > 0 {
		params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
	}
	TelegramAPI(cfg, "sendChatAction", params)
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

// RedactTokenError replaces the bot token in error messages with "***"
func RedactTokenError(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "***"))
}

// TelegramClientGet performs an HTTP GET with a custom client and redacts the bot token from any errors
func TelegramClientGet(client *http.Client, token string, reqURL string) (*http.Response, error) {
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, RedactTokenError(err, token)
	}
	return resp, nil
}
