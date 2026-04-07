package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func handleStopHook() error {
	defer func() { recover() }()

	rawData, _ := readHookStdin()
	if len(rawData) == 0 {
		return nil
	}

	hookData, err := parseHookData(rawData)
	if err != nil {
		return nil
	}

	config, err := loadConfig()
	if err != nil || config == nil {
		return nil
	}

	sessName, topicID := findSession(config, hookData.Cwd, hookData.SessionID)
	if sessName == "" || config.GroupID == 0 || topicID == 0 {
		hookLog("stop-hook: no matching session found: cwd=%s session_id=%s", hookData.Cwd, hookData.SessionID)
		return nil
	}

	// Persist claude session ID to config for future lookups
	persistClaudeSessionID(config, sessName, hookData.SessionID, hookData.TranscriptPath)

	hookLog("stop-hook: session=%s claude_session_id=%s transcript=%s", sessName, hookData.SessionID, hookData.TranscriptPath)

	// Clear flags when Claude stops
	tmuxName := tmuxSafeName(sessName)
	os.Remove(telegramActiveFlag(tmuxName))
	clearThinking(sessName)

	// Deliver unsent texts as separate messages (these come after all tools)
	hookLog("stop-hook: delivering unsent texts")
	sent := deliverUnsentTexts(config, sessName, topicID, hookData.TranscriptPath, false, hookData.SessionID)
	hookLog("stop-hook: sent=%d", sent)
	clearToolState(sessName)

	// Background retry: transcript may not be flushed yet when stop hook fires.
	// Spawn a detached subprocess that retries 3 times at 2-second intervals.
	// (goroutines die when the hook process exits, so we need a separate process)
	cmd := exec.Command(cccPath, "hook-stop-retry", sessName, fmt.Sprintf("%d", topicID), hookData.TranscriptPath)
	cmd.Start()

	return nil
}

func handlePermissionHook() error {
	defer func() { recover() }()

	rawData, _ := readHookStdin()
	if len(rawData) == 0 {
		return nil
	}

	hookData, err := parseHookData(rawData)
	if err != nil {
		return nil
	}

	config, err := loadConfig()
	if err != nil || config == nil {
		return nil
	}

	sessName, topicID := findSession(config, hookData.Cwd, hookData.SessionID)
	if sessName == "" || config.GroupID == 0 {
		return nil
	}

	// Persist claude session ID to config for future lookups
	persistClaudeSessionID(config, sessName, hookData.SessionID, hookData.TranscriptPath)

	hookLog("pre-tool: session=%s tool=%s", sessName, hookData.ToolName)

	// Deliver any unsent assistant text before showing tool calls
	if topicID != 0 && hookData.TranscriptPath != "" {
		deliverUnsentTexts(config, sessName, topicID, hookData.TranscriptPath, true, hookData.SessionID)
	}

	// Update tool call display
	if hookData.ToolName != "" && hookData.ToolName != "AskUserQuestion" && topicID != 0 {
		state := loadToolState(sessName)
		state.Tools = append(state.Tools, ToolCall{
			Name:  hookData.ToolName,
			Input: toolInputSummary(hookData),
			Time:  time.Now().UnixMilli(),
		})
		text := formatToolMessage(state)
		if state.MsgID == 0 {
			msgID, err := sendMessageHTMLGetID(config, config.GroupID, topicID, text)
			if err == nil && msgID > 0 {
				state.MsgID = msgID
			}
		} else {
			editMessageHTML(config, config.GroupID, state.MsgID, topicID, text)
		}
		saveToolState(sessName, state)

		// Record tool call in ledger
		appendMessage(&MessageRecord{
			ID:                fmt.Sprintf("tool:%s:%s:%d", hookData.SessionID, contentHash(hookData.ToolName+toolInputSummary(hookData)), time.Now().UnixNano()),
			Session:           sessName,
			Type:              "tool_call",
			Text:              hookData.ToolName + ": " + toolInputSummary(hookData),
			Origin:            "claude",
			TerminalDelivered: true,
			TelegramDelivered: state.MsgID != 0,
			TelegramMsgID:     state.MsgID,
		})
	}

	// Handle AskUserQuestion - forward to Telegram with buttons
	if hookData.ToolName == "AskUserQuestion" && len(hookData.ToolInput.Questions) > 0 {
		for qIdx, q := range hookData.ToolInput.Questions {
			if q.Question == "" {
				continue
			}
			msg := fmt.Sprintf("❓ %s\n\n%s", q.Header, q.Question)

			var buttons [][]InlineKeyboardButton
			for i, opt := range q.Options {
				if opt.Label == "" {
					continue
				}
				totalQuestions := len(hookData.ToolInput.Questions)
				callbackData := fmt.Sprintf("%s:%d:%d:%d", sessName, qIdx, totalQuestions, i)
				if len(callbackData) > 64 {
					callbackData = callbackData[:64]
				}
				buttons = append(buttons, []InlineKeyboardButton{
					{Text: opt.Label, CallbackData: callbackData},
				})
			}

			if len(buttons) > 0 {
				sendMessageWithKeyboard(config, config.GroupID, topicID, msg, buttons)
			}
		}
		return nil
	}

	// OTP permission check for all other tools
	if !isOTPEnabled(config) {
		// No OTP configured, auto-allow everything
		outputPermissionDecision("allow", "OTP not configured")
		return nil
	}

	// OTP only applies when input came from Telegram (flag file exists and is recent).
	// The listener sets this flag before forwarding Telegram messages to tmux.
	// Flag auto-expires after 5 minutes to handle cases where stop hook didn't fire.
	tmuxName := tmuxSafeName(sessName)
	flagInfo, err := os.Stat(telegramActiveFlag(tmuxName))
	if err != nil || time.Since(flagInfo.ModTime()) > otpGrantDuration {
		return nil // no flag or expired, let Claude handle permissions normally
	}

	// Check for a valid OTP grant (approved within the last 5 minutes)
	if hasValidOTPGrant(tmuxName) {
		outputPermissionDecision("allow", "OTP grant still valid")
		return nil
	}

	// Build a human-readable description of what Claude wants to do
	toolDesc := hookData.ToolName
	var inputStr string
	switch hookData.ToolName {
	case "Bash":
		if hookData.ToolInput.Command != "" {
			inputStr = hookData.ToolInput.Command
		}
	case "Read":
		if hookData.ToolInput.FilePath != "" {
			inputStr = hookData.ToolInput.FilePath
		}
	case "Write", "Edit":
		if hookData.ToolInput.FilePath != "" {
			inputStr = hookData.ToolInput.FilePath
		}
	}
	if inputStr == "" {
		inputStr = string(hookData.ToolInputRaw)
	}
	if len(inputStr) > 500 {
		inputStr = inputStr[:500] + "..."
	}

	// Use session_id from hook data as unique identifier
	sessionID := hookData.SessionID
	if sessionID == "" {
		sessionID = sessName
	}

	// Only the first parallel hook sends the Telegram message.
	// If a request file already exists (from another parallel hook), just wait.
	alreadyRequested := false
	if info, err := os.Stat(otpRequestPrefix + sessionID); err == nil {
		alreadyRequested = time.Since(info.ModTime()) < 30*time.Second
	}

	req := &OTPPermissionRequest{
		SessionName: sessName,
		ToolName:    hookData.ToolName,
		ToolInput:   inputStr,
		Timestamp:   time.Now().Unix(),
	}
	writeOTPRequest(sessionID, req)

	if !alreadyRequested {
		msg := fmt.Sprintf("🔐 Permission request:\n\n🔧 %s\n📋 %s\n\nSend your OTP code to approve:", toolDesc, inputStr)
		sendMessage(config, config.GroupID, topicID, msg)
	}

	hookLog("otp-request: waiting for OTP response for session=%s tool=%s already=%v", sessName, hookData.ToolName, alreadyRequested)

	// Wait for OTP response from listener
	approved, err := waitForOTPResponse(sessionID, tmuxName, otpPermissionTimeout)
	if err != nil {
		hookLog("otp-request: timeout or error: %v", err)
		sendMessage(config, config.GroupID, topicID, "⏰ OTP timeout - permission denied")
		outputPermissionDecision("deny", "OTP approval timed out")
		return nil
	}

	if approved {
		hookLog("otp-request: approved for session=%s tool=%s", sessName, hookData.ToolName)
		writeOTPGrant(tmuxName)
		outputPermissionDecision("allow", "Approved via OTP")
	} else {
		hookLog("otp-request: denied for session=%s tool=%s", sessName, hookData.ToolName)
		outputPermissionDecision("deny", "Denied via OTP")
	}

	return nil
}

// outputPermissionDecision writes the PreToolUse hook response to stdout
func outputPermissionDecision(decision, reason string) {
	response := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       decision,
			"permissionDecisionReason": reason,
		},
	}
	data, _ := json.Marshal(response)
	fmt.Println(string(data))
}

func handleUserPromptHook() error {
	defer func() { recover() }()

	rawData, _ := readHookStdin()
	if len(rawData) == 0 {
		return nil
	}

	hookData, err := parseHookData(rawData)
	if err != nil || hookData.Prompt == "" {
		return nil
	}

	config, err := loadConfig()
	if err != nil || config == nil {
		return nil
	}

	sessName, topicID := findSession(config, hookData.Cwd, hookData.SessionID)
	if sessName == "" || config.GroupID == 0 || topicID == 0 {
		return nil
	}

	persistClaudeSessionID(config, sessName, hookData.SessionID, hookData.TranscriptPath)

	// Collapse tool message from previous turn
	collapseToolMessage(config, sessName, topicID)
	clearToolState(sessName)

	// Skip if this prompt came from Telegram (already visible in the chat).
	// The flag is consumed (deleted) so subsequent TUI prompts are not skipped.
	tmuxName := tmuxSafeName(sessName)
	if flagInfo, err := os.Stat(telegramActiveFlag(tmuxName)); err == nil {
		if time.Since(flagInfo.ModTime()) < 30*time.Second {
			os.Remove(telegramActiveFlag(tmuxName))
			writePromptAck(sessName)
			setThinking(sessName)
			// Record: came from Telegram, both sides have it
			appendMessage(&MessageRecord{
				ID:                fmt.Sprintf("prompt:%s:%d", hookData.SessionID, time.Now().UnixNano()),
				Session:           sessName,
				Type:              "user_prompt",
				Text:              hookData.Prompt,
				Origin:            "telegram",
				TerminalDelivered: true,
				TelegramDelivered: true,
			})
			return nil
		}
	}

	setThinking(sessName)

	// Record: came from terminal, Telegram not yet delivered
	msgID := fmt.Sprintf("prompt:%s:%d", hookData.SessionID, time.Now().UnixNano())
	appendMessage(&MessageRecord{
		ID:                msgID,
		Session:           sessName,
		Type:              "user_prompt",
		Text:              hookData.Prompt,
		Origin:            "terminal",
		TerminalDelivered: true,
		TelegramDelivered: false,
	})

	sendMessage(config, config.GroupID, topicID, fmt.Sprintf("💬 %s", hookData.Prompt))
	updateDelivery(sessName, msgID, "telegram_delivered", true)
	return nil
}

func handlePostToolHook() error {
	// No-op: tool completion is implied by the next tool starting
	return nil
}

func handleNotificationHook() error {
	defer func() { recover() }()

	rawData, _ := readHookStdin()
	if len(rawData) == 0 {
		return nil
	}

	hookData, err := parseHookData(rawData)
	if err != nil {
		return nil
	}

	config, err := loadConfig()
	if err != nil || config == nil {
		return nil
	}

	sessName, topicID := findSession(config, hookData.Cwd, hookData.SessionID)
	if sessName == "" || config.GroupID == 0 || topicID == 0 {
		return nil
	}

	persistClaudeSessionID(config, sessName, hookData.SessionID, hookData.TranscriptPath)

	// idle_prompt means Claude is waiting for user input — clear typing indicator
	if hookData.NotificationType == "idle_prompt" {
		clearThinking(sessName)
		return nil
	}

	// Build notification message
	var msg string
	if hookData.Message != "" {
		msg = fmt.Sprintf("🔔 %s", hookData.Message)
	} else if hookData.Title != "" {
		msg = fmt.Sprintf("🔔 %s", hookData.Title)
	} else if hookData.NotificationType != "" {
		msg = fmt.Sprintf("🔔 %s", hookData.NotificationType)
	}

	if msg != "" {
		msgID := fmt.Sprintf("notif:%s:%d", hookData.SessionID, time.Now().UnixNano())
		appendMessage(&MessageRecord{
			ID:                msgID,
			Session:           sessName,
			Type:              "notification",
			Text:              msg,
			Origin:            "claude",
			TerminalDelivered: true,
			TelegramDelivered: false,
		})
		sendMessage(config, config.GroupID, topicID, msg)
		updateDelivery(sessName, msgID, "telegram_delivered", true)
	}

	return nil
}
