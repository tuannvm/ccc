package main

import (
	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/session"
)

// ========== Config Type Aliases ==========

// Config is an alias for config.Config
type Config = config.Config

// SessionInfo is an alias for config.SessionInfo
type SessionInfo = config.SessionInfo

// PaneInfo is an alias for config.PaneInfo
type PaneInfo = config.PaneInfo

// ProviderConfig is an alias for config.ProviderConfig
type ProviderConfig = config.ProviderConfig

// ========== Session Interface Implementation ==========

// Ensure SessionInfo implements session.Session interface
var _ session.Session = (*SessionInfo)(nil)

// ========== Telegram Type Aliases ==========

// TelegramMessage represents a Telegram message
type TelegramMessage = telegram.TelegramMessage

// TelegramVoice represents a voice message
type TelegramVoice = telegram.TelegramVoice

// TelegramPhoto represents a photo
type TelegramPhoto = telegram.TelegramPhoto

// TelegramDocument represents a document
type TelegramDocument = telegram.TelegramDocument

// CallbackQuery represents a Telegram callback query (button press)
type CallbackQuery = telegram.CallbackQuery

// TelegramUpdate represents an update from Telegram
type TelegramUpdate = telegram.TelegramUpdate

// TelegramResponse represents a response from Telegram API
type TelegramResponse = telegram.TelegramResponse

// TopicResult represents the result of creating a forum topic
type TopicResult = telegram.TopicResult

// InlineKeyboardButton represents a Telegram inline keyboard button
type InlineKeyboardButton = telegram.InlineKeyboardButton

// ========== Hook Types ==========

// HookData represents data received from Claude hook
type HookData = hooks.HookData

// HookToolInput holds parsed tool input for known tool types
type HookToolInput = hooks.HookToolInput

// parseHookData unmarshals raw JSON and populates ToolInput
func parseHookData(data []byte) (HookData, error) {
	return hooks.ParseHookData(data)
}

// ========== Ledger Type Aliases ==========

// MessageRecord tracks the delivery state of a single message
type MessageRecord = ledger.MessageRecord

// ========== OTP Type Aliases ==========

// OTPPermissionRequest is an alias for hooks.OTPPermissionRequest
type OTPPermissionRequest = hooks.OTPPermissionRequest
