package main

import (
	"net/url"

	"github.com/tuannvm/ccc/pkg/telegram"
)

// telegramAPI performs a Telegram API call
func telegramAPI(config *Config, method string, params url.Values) (*TelegramResponse, error) {
	return telegram.TelegramAPI(config, method, params)
}

// sendMessage sends a text message to Telegram
func sendMessage(config *Config, chatID int64, threadID int64, text string) error {
	return telegram.SendMessage(config, chatID, threadID, text)
}

// sendMessageGetID sends a message and returns the message ID for later editing
func sendMessageGetID(config *Config, chatID int64, threadID int64, text string) (int64, error) {
	return telegram.SendMessageGetID(config, chatID, threadID, text)
}

// sendMessageHTMLGetID sends a message with HTML parse mode and returns the message ID
func sendMessageHTMLGetID(config *Config, chatID int64, threadID int64, text string) (int64, error) {
	return telegram.SendMessageHTMLGetID(config, chatID, threadID, text)
}

// sendMessageWithMode sends a message with the specified parse mode
func sendMessageWithMode(config *Config, chatID int64, threadID int64, text string, parseMode string) (int64, error) {
	return telegram.SendMessageWithMode(config, chatID, threadID, text, parseMode)
}

// editMessageHTML edits a message using HTML parse mode
func editMessageHTML(config *Config, chatID int64, messageID int64, threadID int64, text string) error {
	return telegram.EditMessageHTML(config, chatID, messageID, threadID, text)
}

// editMessageWithMode edits a message with the specified parse mode
func editMessageWithMode(config *Config, chatID int64, messageID int64, threadID int64, text string, parseMode string) error {
	return telegram.EditMessageWithMode(config, chatID, messageID, threadID, text, parseMode)
}

// sendMessageWithKeyboard sends a message with an inline keyboard
func sendMessageWithKeyboard(config *Config, chatID int64, threadID int64, text string, buttons [][]InlineKeyboardButton) error {
	return telegram.SendMessageWithKeyboard(config, chatID, threadID, text, buttons)
}

// answerCallbackQuery responds to a callback query
func answerCallbackQuery(config *Config, callbackID string) {
	telegram.AnswerCallbackQuery(config, callbackID)
}

// editMessageRemoveKeyboard edits a message and removes its keyboard
func editMessageRemoveKeyboard(config *Config, chatID int64, messageID int, newText string) {
	telegram.EditMessageRemoveKeyboard(config, chatID, messageID, newText)
}

// sendTypingAction sends a typing action to the chat
func sendTypingAction(config *Config, chatID int64, threadID int64) {
	telegram.SendTypingAction(config, chatID, threadID)
}
