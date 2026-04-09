package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/session"
)

// HandlerCallbacks provides callbacks for root-level dependencies
type HandlerCallbacks struct {
	// Config management
	LoadConfig     func() (*config.Config, error)
	SaveConfig     func(cfg *config.Config) error

	// Session management
	FindSession              func(cfg *config.Config, cwd, sessionID string) (string, int64)
	PersistClaudeSessionID   func(cfg *config.Config, sessName, claudeSessionID, transcriptPath string)
	GetSessionWorkDir        func(cfg *config.Config, sessionName string, sessionInfo *config.SessionInfo) string

	// Tool state management
	LoadToolState      func(sessionName string) *ToolState
	SaveToolState      func(sessionName string, state *ToolState)
	ClearToolState     func(sessionName string)
	AddTextToToolState func(sessName string, text string, ts int64)
	FormatToolMessage  func(state *ToolState) string

	// Thinking indicators
	SetThinking    func(sessionName string)
	ClearThinking  func(sessionName string)

	// Telegram functions
	TelegramActiveFlag   func(tmuxName string) string
	SendMessage          func(cfg *config.Config, chatID int64, threadID int64, text string) error
	SendMessageHTML      func(cfg *config.Config, chatID int64, threadID int64, text string) (int64, error)
	SendMessageGetID     func(cfg *config.Config, chatID int64, threadID int64, text string) (int64, error)
	EditMessageHTML      func(cfg *config.Config, chatID int64, msgID int64, threadID int64, text string) error
	SendMessageWithKeyboard func(cfg *config.Config, chatID int64, threadID int64, text string, buttons [][]telegram.InlineKeyboardButton) error

	// Ledger functions
	IsDelivered      func(sessName, id, origin string) bool
	AppendMessage    func(msg *ledger.MessageRecord)
	UpdateDelivery   func(sessName, msgID, field string, value bool)

	// OTP functions
	IsOTPEnabled         func(cfg *config.Config) bool
	HasValidOTPGrant    func(tmuxName string) bool
	WriteOTPRequest     func(sessionID string, req *OTPPermissionRequest)
	WriteOTPGrant       func(tmuxName string)
	WaitForOTPResponse  func(sessionID, tmuxName string, timeout time.Duration) (bool, error)

	// Misc
	TmuxSafeName                func(name string) string
	WritePromptAck              func(sessionName string)
	InferRoleFromTranscriptPath func(transcriptPath string) string
	ToolInputSummary            func(hookData HookData) string
	ReadHookStdin               func() ([]byte, error)

	// CCC path for spawning retry process
	CCCPath string
}

// OTPPermissionRequest represents an OTP permission request
type OTPPermissionRequest struct {
	SessionName string `json:"session_name"`
	ToolName    string `json:"tool_name"`
	ToolInput   string `json:"tool_input"`
	Timestamp   int64  `json:"timestamp"`
}

const (
	otpPermissionTimeout = 5 * time.Minute
	otpGrantDuration     = 5 * time.Minute
)

// OTP file paths (using config.CacheDir() instead of hardcoded /tmp)
var (
	otpRequestPrefix = filepath.Join(config.CacheDir(), "otp-request-")
)

// HandleStopHook handles the Stop hook event
func HandleStopHook(callbacks *HandlerCallbacks) error {
	defer func() { recover() }()

	rawData, _ := callbacks.ReadHookStdin()
	if len(rawData) == 0 {
		return nil
	}

	hookData, err := ParseHookData(rawData)
	if err != nil {
		return nil
	}

	cfg, err := callbacks.LoadConfig()
	if err != nil || cfg == nil {
		return nil
	}

	sessName, topicID := callbacks.FindSession(cfg, hookData.Cwd, hookData.SessionID)
	if sessName == "" || cfg.GroupID == 0 || topicID == 0 {
		HookLog("stop-hook: no matching session found: cwd=%s session_id=%s", hookData.Cwd, hookData.SessionID)
		return nil
	}

	// Persist claude session ID to config for future lookups
	callbacks.PersistClaudeSessionID(cfg, sessName, hookData.SessionID, hookData.TranscriptPath)

	HookLog("stop-hook: session=%s claude_session_id=%s transcript=%s", sessName, hookData.SessionID, hookData.TranscriptPath)

	// Clear flags when Claude stops
	tmuxName := callbacks.TmuxSafeName(sessName)
	os.Remove(callbacks.TelegramActiveFlag(tmuxName))
	callbacks.ClearThinking(sessName)

	// Deliver unsent texts as separate messages (these come after all tools)
	HookLog("stop-hook: delivering unsent texts")
	deliverCfg := &DeliverUnsentTextsConfig{
		Config:              cfg,
		SessionName:         sessName,
		TopicID:             topicID,
		TranscriptPath:      hookData.TranscriptPath,
		InsertIntoToolMsg:   false,
		ClaudeSessionID:     hookData.SessionID,
		LoadToolState:       callbacks.LoadToolState,
		AddTextToToolState:  callbacks.AddTextToToolState,
		SaveToolState:       callbacks.SaveToolState,
		FormatToolMessage:   callbacks.FormatToolMessage,
		EditMessageHTML:     callbacks.EditMessageHTML,
		SendMessageHTML:     callbacks.SendMessageHTML,
		SendMessageGetID:    callbacks.SendMessageGetID,
		SendMessage:         callbacks.SendMessage,
		IsDelivered:         callbacks.IsDelivered,
		AppendMessage:       callbacks.AppendMessage,
		ClearToolState:      callbacks.ClearToolState,
		InferRoleFromTranscriptPath: func(path string) session.PaneRole {
			role := callbacks.InferRoleFromTranscriptPath(path)
			return session.PaneRole(role)
		},
	}
	sent := DeliverUnsentTexts(deliverCfg)
	HookLog("stop-hook: sent=%d", sent)
	callbacks.ClearToolState(sessName)

	// Background retry: transcript may not be flushed yet when stop hook fires.
	// Spawn a detached subprocess that retries 3 times at 2-second intervals.
	cmd := exec.Command(callbacks.CCCPath, "hook-stop-retry", sessName, fmt.Sprintf("%d", topicID), hookData.TranscriptPath)
	cmd.Start()

	return nil
}

// HandlePermissionHook handles the PreToolUse hook event
func HandlePermissionHook(callbacks *HandlerCallbacks) error {
	defer func() { recover() }()

	rawData, _ := callbacks.ReadHookStdin()
	if len(rawData) == 0 {
		return nil
	}

	hookData, err := ParseHookData(rawData)
	if err != nil {
		return nil
	}

	cfg, err := callbacks.LoadConfig()
	if err != nil || cfg == nil {
		return nil
	}

	sessName, topicID := callbacks.FindSession(cfg, hookData.Cwd, hookData.SessionID)
	if sessName == "" || cfg.GroupID == 0 {
		return nil
	}

	// Persist claude session ID to config for future lookups
	callbacks.PersistClaudeSessionID(cfg, sessName, hookData.SessionID, hookData.TranscriptPath)

	HookLog("pre-tool: session=%s tool=%s", sessName, hookData.ToolName)

	// Deliver any unsent assistant text before showing tool calls
	if topicID != 0 && hookData.TranscriptPath != "" {
		deliverCfg := &DeliverUnsentTextsConfig{
			Config:              cfg,
			SessionName:         sessName,
			TopicID:             topicID,
			TranscriptPath:      hookData.TranscriptPath,
			InsertIntoToolMsg:   true,
			ClaudeSessionID:     hookData.SessionID,
			LoadToolState:       callbacks.LoadToolState,
			AddTextToToolState:  callbacks.AddTextToToolState,
			SaveToolState:       callbacks.SaveToolState,
			FormatToolMessage:   callbacks.FormatToolMessage,
			EditMessageHTML:     callbacks.EditMessageHTML,
			SendMessageHTML:     callbacks.SendMessageHTML,
			SendMessageGetID:    callbacks.SendMessageGetID,
			SendMessage:         callbacks.SendMessage,
			IsDelivered:         callbacks.IsDelivered,
			AppendMessage:       callbacks.AppendMessage,
			ClearToolState:      callbacks.ClearToolState,
			InferRoleFromTranscriptPath: func(path string) session.PaneRole {
				role := callbacks.InferRoleFromTranscriptPath(path)
				return session.PaneRole(role)
			},
		}
		DeliverUnsentTexts(deliverCfg)
	}

	// Update tool call display
	if hookData.ToolName != "" && hookData.ToolName != "AskUserQuestion" && topicID != 0 {
		state := callbacks.LoadToolState(sessName)
		state.Tools = append(state.Tools, ToolCall{
			Name:  hookData.ToolName,
			Input: callbacks.ToolInputSummary(hookData),
			Time:  time.Now().UnixMilli(),
		})
		text := callbacks.FormatToolMessage(state)
		if state.MsgID == 0 {
			msgID, err := callbacks.SendMessageHTML(cfg, cfg.GroupID, topicID, text)
			if err == nil && msgID > 0 {
				state.MsgID = msgID
			}
		} else {
			callbacks.EditMessageHTML(cfg, cfg.GroupID, state.MsgID, topicID, text)
		}
		callbacks.SaveToolState(sessName, state)

		// Record tool call in ledger
		callbacks.AppendMessage(&ledger.MessageRecord{
			ID:                fmt.Sprintf("tool:%s:%s:%d", hookData.SessionID, ledger.ContentHash(hookData.ToolName+callbacks.ToolInputSummary(hookData)), time.Now().UnixNano()),
			Session:           sessName,
			Type:              "tool_call",
			Text:              hookData.ToolName + ": " + callbacks.ToolInputSummary(hookData),
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

			var buttons [][]telegram.InlineKeyboardButton
			for i, opt := range q.Options {
				if opt.Label == "" {
					continue
				}
				totalQuestions := len(hookData.ToolInput.Questions)
				callbackData := fmt.Sprintf("%s:%d:%d:%d", sessName, qIdx, totalQuestions, i)
				if len(callbackData) > 64 {
					callbackData = callbackData[:64]
				}
				buttons = append(buttons, []telegram.InlineKeyboardButton{
					{Text: opt.Label, CallbackData: callbackData},
				})
			}

			if len(buttons) > 0 {
				callbacks.SendMessageWithKeyboard(cfg, cfg.GroupID, topicID, msg, buttons)
			}
		}
		return nil
	}

	// OTP permission check for all other tools
	if !callbacks.IsOTPEnabled(cfg) {
		// No OTP configured, auto-allow everything
		OutputPermissionDecision("allow", "OTP not configured")
		return nil
	}

	// OTP only applies when input came from Telegram (flag file exists and is recent).
	tmuxName := callbacks.TmuxSafeName(sessName)
	flagInfo, err := os.Stat(callbacks.TelegramActiveFlag(tmuxName))
	if err != nil || time.Since(flagInfo.ModTime()) > otpGrantDuration {
		return nil // no flag or expired, let Claude handle permissions normally
	}

	// Check for a valid OTP grant (approved within the last 5 minutes)
	if callbacks.HasValidOTPGrant(tmuxName) {
		OutputPermissionDecision("allow", "OTP grant still valid")
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
	callbacks.WriteOTPRequest(sessionID, req)

	if !alreadyRequested {
		msg := fmt.Sprintf("🔐 Permission request:\n\n🔧 %s\n📋 %s\n\nSend your OTP code to approve:", toolDesc, inputStr)
		callbacks.SendMessage(cfg, cfg.GroupID, topicID, msg)
	}

	HookLog("otp-request: waiting for OTP response for session=%s tool=%s already=%v", sessName, hookData.ToolName, alreadyRequested)

	// Wait for OTP response from listener
	approved, err := callbacks.WaitForOTPResponse(sessionID, tmuxName, otpPermissionTimeout)
	if err != nil {
		HookLog("otp-request: timeout or error: %v", err)
		callbacks.SendMessage(cfg, cfg.GroupID, topicID, "⏰ OTP timeout - permission denied")
		OutputPermissionDecision("deny", "OTP approval timed out")
		return nil
	}

	if approved {
		HookLog("otp-request: approved for session=%s tool=%s", sessName, hookData.ToolName)
		callbacks.WriteOTPGrant(tmuxName)
		OutputPermissionDecision("allow", "Approved via OTP")
	} else {
		HookLog("otp-request: denied for session=%s tool=%s", sessName, hookData.ToolName)
		OutputPermissionDecision("deny", "Denied via OTP")
	}

	return nil
}

// OutputPermissionDecision writes the PreToolUse hook response to stdout
func OutputPermissionDecision(decision, reason string) {
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

// HandleUserPromptHook handles the UserPromptSubmit hook event
func HandleUserPromptHook(callbacks *HandlerCallbacks) error {
	defer func() { recover() }()

	rawData, _ := callbacks.ReadHookStdin()
	if len(rawData) == 0 {
		return nil
	}

	hookData, err := ParseHookData(rawData)
	if err != nil || hookData.Prompt == "" {
		return nil
	}

	cfg, err := callbacks.LoadConfig()
	if err != nil || cfg == nil {
		return nil
	}

	sessName, topicID := callbacks.FindSession(cfg, hookData.Cwd, hookData.SessionID)
	if sessName == "" || cfg.GroupID == 0 || topicID == 0 {
		return nil
	}

	callbacks.PersistClaudeSessionID(cfg, sessName, hookData.SessionID, hookData.TranscriptPath)

	// Collapse tool message from previous turn
	callbacks.ClearToolState(sessName)

	// Skip if this prompt came from Telegram (already visible in the chat).
	tmuxName := callbacks.TmuxSafeName(sessName)
	if flagInfo, err := os.Stat(callbacks.TelegramActiveFlag(tmuxName)); err == nil {
		if time.Since(flagInfo.ModTime()) < 30*time.Second {
			os.Remove(callbacks.TelegramActiveFlag(tmuxName))
			callbacks.WritePromptAck(sessName)
			callbacks.SetThinking(sessName)
			// Record: came from Telegram, both sides have it
			callbacks.AppendMessage(&ledger.MessageRecord{
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

	callbacks.SetThinking(sessName)

	// Record: came from terminal, Telegram not yet delivered
	msgID := fmt.Sprintf("prompt:%s:%d", hookData.SessionID, time.Now().UnixNano())
	callbacks.AppendMessage(&ledger.MessageRecord{
		ID:                msgID,
		Session:           sessName,
		Type:              "user_prompt",
		Text:              hookData.Prompt,
		Origin:            "terminal",
		TerminalDelivered: true,
		TelegramDelivered: false,
	})

	callbacks.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("💬 %s", hookData.Prompt))
	callbacks.UpdateDelivery(sessName, msgID, "telegram_delivered", true)
	return nil
}

// HandlePostToolHook handles the PostToolUse hook event
func HandlePostToolHook(callbacks *HandlerCallbacks) error {
	// No-op: tool completion is implied by the next tool starting
	return nil
}

// HandleNotificationHook handles the Notification hook event
func HandleNotificationHook(callbacks *HandlerCallbacks) error {
	defer func() { recover() }()

	rawData, _ := callbacks.ReadHookStdin()
	if len(rawData) == 0 {
		return nil
	}

	hookData, err := ParseHookData(rawData)
	if err != nil {
		return nil
	}

	cfg, err := callbacks.LoadConfig()
	if err != nil || cfg == nil {
		return nil
	}

	sessName, topicID := callbacks.FindSession(cfg, hookData.Cwd, hookData.SessionID)
	if sessName == "" || cfg.GroupID == 0 || topicID == 0 {
		return nil
	}

	callbacks.PersistClaudeSessionID(cfg, sessName, hookData.SessionID, hookData.TranscriptPath)

	// idle_prompt means Claude is waiting for user input — clear typing indicator
	if hookData.NotificationType == "idle_prompt" {
		callbacks.ClearThinking(sessName)
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
		callbacks.AppendMessage(&ledger.MessageRecord{
			ID:                msgID,
			Session:           sessName,
			Type:              "notification",
			Text:              msg,
			Origin:            "claude",
			TerminalDelivered: true,
			TelegramDelivered: false,
		})
		callbacks.SendMessage(cfg, cfg.GroupID, topicID, msg)
		callbacks.UpdateDelivery(sessName, msgID, "telegram_delivered", true)
	}

	return nil
}
