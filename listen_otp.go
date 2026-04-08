package main

import (
	"fmt"
	"strings"

	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/auth"
)

// handleOTPResponse handles OTP code responses for permission approval
func handleOTPResponse(config *Config, text string, chatID, threadID int64) bool {
	if !auth.IsOTPEnabled(config) || strings.HasPrefix(text, "/") {
		return false
	}

	pendingSession := auth.FindPendingOTPSession()
	if pendingSession == "" {
		return false
	}

	code := strings.TrimSpace(text)
	if auth.ValidateOTP(config.OTPSecret, code) {
		auth.WriteOTPResponse(pendingSession, true)
		delete(otpAttempts, pendingSession)
		telegram.SendMessage(config, chatID, threadID, "✅ Permission approved (valid for 5 min)")
	} else {
		otpAttempts[pendingSession]++
		remaining := 5 - otpAttempts[pendingSession]
		if remaining <= 0 {
			auth.WriteOTPResponse(pendingSession, false)
			delete(otpAttempts, pendingSession)
			telegram.SendMessage(config, chatID, threadID, "❌ Too many failed attempts - permission denied")
		} else {
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Invalid code — %d attempts remaining", remaining))
		}
	}
	return true
}
