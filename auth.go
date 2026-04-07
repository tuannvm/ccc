package main

import "sync"

// Authorization helpers for Telegram message/callback validation.

// isAuthorizedCallback checks if a callback query is authorized based on multi-user mode
func isAuthorizedCallback(config *Config, cb *CallbackQuery) bool {
	if cb == nil || cb.Message == nil {
		return false
	}

	if config.MultiUserMode {
		// Multi-user mode: anyone in the configured group can interact
		if config.GroupID == 0 {
			return false // Group not configured
		}
		return cb.Message.Chat.ID == config.GroupID
	}
	// Single-user mode (default): only the authorized user can interact
	return cb.From.ID == config.ChatID
}

// isAuthorizedMessage checks if a message is authorized based on multi-user mode
func isAuthorizedMessage(config *Config, msg TelegramMessage) bool {
	if config.MultiUserMode {
		// Multi-user mode: anyone in the configured group can interact
		if config.GroupID == 0 {
			return false // Group not configured
		}
		return msg.Chat.ID == config.GroupID
	}
	// Single-user mode (default): only the authorized user can interact
	return msg.From.ID == config.ChatID
}

// Auth state for OTP permission approval
var authInProgress sync.Mutex
var authWaitingCode bool
var otpAttempts = make(map[string]int) // session -> failed attempts
