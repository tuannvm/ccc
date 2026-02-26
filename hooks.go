package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// telegramActiveFlag returns the path of the flag file that indicates
// a Telegram message is being processed by a tmux session.
func telegramActiveFlag(tmuxName string) string {
	return "/tmp/ccc-telegram-active-" + tmuxName
}

// readHookStdin reads stdin JSON with a timeout
func readHookStdin() ([]byte, error) {
	stdinData := make(chan []byte, 1)
	go func() {
		defer func() { recover() }()
		data, _ := io.ReadAll(os.Stdin)
		stdinData <- data
	}()

	select {
	case rawData := <-stdinData:
		return rawData, nil
	case <-time.After(2 * time.Second):
		return nil, nil
	}
}

// findSessionByClaudeID matches a claude session ID to a configured session
func findSessionByClaudeID(config *Config, claudeSessionID string) (string, int64) {
	if claudeSessionID == "" {
		return "", 0
	}
	for name, info := range config.Sessions {
		if name == "" || info == nil {
			continue
		}
		if info.ClaudeSessionID == claudeSessionID {
			return name, info.TopicID
		}
	}
	return "", 0
}

// findSessionByCwd matches a hook's cwd to a configured session (fallback)
func findSessionByCwd(config *Config, cwd string) (string, int64) {
	for name, info := range config.Sessions {
		if name == "" || info == nil {
			continue
		}
		if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") || strings.HasSuffix(cwd, "/"+name) {
			return name, info.TopicID
		}
	}
	return "", 0
}

// findSession matches by claude_session_id first, then falls back to cwd
func findSession(config *Config, cwd string, claudeSessionID string) (string, int64) {
	if name, topicID := findSessionByClaudeID(config, claudeSessionID); name != "" {
		return name, topicID
	}
	return findSessionByCwd(config, cwd)
}

// persistClaudeSessionID saves the claude session ID to config if changed
func persistClaudeSessionID(config *Config, sessName string, claudeSessionID string) {
	if claudeSessionID == "" || sessName == "" {
		return
	}
	info, exists := config.Sessions[sessName]
	if !exists || info == nil {
		return
	}
	if info.ClaudeSessionID != claudeSessionID {
		info.ClaudeSessionID = claudeSessionID
		saveConfig(config)
		hookLog("persisted claude_session_id=%s for session=%s", claudeSessionID, sessName)
	}
}

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
		return nil
	}

	// Persist claude session ID to config for future lookups
	persistClaudeSessionID(config, sessName, hookData.SessionID)

	hookLog("stop-hook: session=%s claude_session_id=%s transcript=%s", sessName, hookData.SessionID, hookData.TranscriptPath)

	// Clear Telegram active flag when Claude stops
	tmuxName := "claude-" + strings.ReplaceAll(sessName, ".", "_")
	os.Remove(telegramActiveFlag(tmuxName))

	// Retry extractLastTurn a few times - transcript may not be fully flushed yet
	var blocks []string
	for attempt := 0; attempt < 5; attempt++ {
		blocks = extractLastTurn(hookData.TranscriptPath)
		if len(blocks) > 0 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	hookLog("stop-hook: extractLastTurn returned %d blocks", len(blocks))
	if len(blocks) == 0 {
		// No text blocks found, just send completion marker
		sendMessage(config, config.GroupID, topicID, fmt.Sprintf("*%s:* ✅", sessName))
		return nil
	}

	for i, block := range blocks {
		hookLog("stop-hook: block[%d] len=%d preview=%s", i, len(block), truncate(block, 80))
		text := block
		if i == len(blocks)-1 {
			text = fmt.Sprintf("*%s:*\n%s", sessName, block)
		}
		sendMessageGetID(config, config.GroupID, topicID, text)
	}

	return nil
}

// extractLastTurn reads the JSONL transcript and extracts text blocks from
// the last assistant turn (after the last real user message).
func extractLastTurn(transcriptPath string) []string {
	if transcriptPath == "" {
		return nil
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	type contentBlock struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Name    string `json:"name,omitempty"`
		Content string `json:"content,omitempty"`
	}

	// transcriptLine handles both nested (message.content) and flat (root-level
	// content) JSONL formats. Claude Code v2.1.45+ may emit either.
	type transcriptLine struct {
		Type      string          `json:"type"`
		RequestID string          `json:"requestId,omitempty"`
		Role      string          `json:"role,omitempty"`
		Content   json.RawMessage `json:"content,omitempty"`
		Message   struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}

	// Parse all lines
	type parsedEntry struct {
		ttype     string
		requestID string
		role      string
		content   json.RawMessage
	}

	var entries []parsedEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var tl transcriptLine
		if json.Unmarshal(line, &tl) != nil {
			continue
		}
		// Use nested message fields if present, otherwise fall back to root-level fields
		role := tl.Message.Role
		content := tl.Message.Content
		if role == "" {
			role = tl.Role
		}
		if len(content) == 0 {
			content = tl.Content
		}
		entries = append(entries, parsedEntry{
			ttype:     tl.Type,
			requestID: tl.RequestID,
			role:      role,
			content:   content,
		})
	}

	if len(entries) == 0 {
		return nil
	}

	// Find the last real user message (not a tool_result)
	lastUserIdx := -1
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.ttype != "user" && e.role != "user" {
			continue
		}
		// Check if content is a tool_result
		if isToolResult(e.content) {
			continue
		}
		lastUserIdx = i
		break
	}

	// Collect text from assistant messages after the last user message.
	// Streaming dedup: same requestId may have multiple entries with progressively
	// updated text; for each requestId, the last entry's text blocks win.
	startIdx := lastUserIdx + 1
	if lastUserIdx < 0 {
		startIdx = 0
	}

	reqTexts := make(map[string][]string) // requestId -> text blocks from last entry
	var orderedKeys []string              // preserve order of first appearance
	var noIDTexts []string                // texts from entries without requestId

	for i := startIdx; i < len(entries); i++ {
		e := entries[i]
		if e.ttype != "assistant" && e.role != "assistant" {
			continue
		}

		var blocks []contentBlock
		if json.Unmarshal(e.content, &blocks) != nil {
			continue
		}

		var entryTexts []string
		for _, b := range blocks {
			if b.Type != "text" {
				continue
			}
			text := strings.TrimSpace(b.Text)
			if text != "" && text != "(no content)" {
				entryTexts = append(entryTexts, text)
			}
		}

		if len(entryTexts) == 0 {
			continue
		}

		if e.requestID == "" {
			noIDTexts = append(noIDTexts, entryTexts...)
		} else {
			if _, seen := reqTexts[e.requestID]; !seen {
				orderedKeys = append(orderedKeys, e.requestID)
			}
			reqTexts[e.requestID] = entryTexts // last entry with text wins
		}
	}

	var texts []string
	for _, key := range orderedKeys {
		texts = append(texts, reqTexts[key]...)
	}
	texts = append(texts, noIDTexts...)

	return texts
}

// isToolResult checks if content JSON contains tool_result entries
func isToolResult(content json.RawMessage) bool {
	if len(content) == 0 {
		return false
	}
	// Try as array of objects
	var blocks []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "tool_result" {
				return true
			}
		}
	}
	return false
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
	persistClaudeSessionID(config, sessName, hookData.SessionID)

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
	tmuxName := "claude-" + strings.ReplaceAll(sessName, ".", "_")
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
	response := map[string]interface{}{
		"hookSpecificOutput": map[string]interface{}{
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

	persistClaudeSessionID(config, sessName, hookData.SessionID)

	sendMessage(config, config.GroupID, topicID, fmt.Sprintf("💬 %s", hookData.Prompt))
	return nil
}

func handleNotificationHook() error {
	return nil
}

// isCccHook checks if a hook entry contains a ccc command
func isCccHook(entry interface{}) bool {
	if m, ok := entry.(map[string]interface{}); ok {
		if cmd, ok := m["command"].(string); ok {
			return strings.Contains(cmd, "ccc hook")
		}
		if hooks, ok := m["hooks"].([]interface{}); ok {
			for _, h := range hooks {
				if hm, ok := h.(map[string]interface{}); ok {
					if cmd, ok := hm["command"].(string); ok {
						if strings.Contains(cmd, "ccc hook") {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func removeCccHooks(hookArray []interface{}) []interface{} {
	var result []interface{}
	for _, entry := range hookArray {
		if !isCccHook(entry) {
			result = append(result, entry)
		}
	}
	return result
}

func installHook() error {
	home, _ := os.UserHomeDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to read settings.json: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse settings.json: %w", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
	}

	cccHooks := map[string][]interface{}{
		"PreToolUse": {
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"command": cccPath + " hook-permission",
						"type":    "command",
						"timeout": 300000,
					},
				},
				"matcher": "",
			},
		},
		"Stop": {
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"command": cccPath + " hook-stop",
						"type":    "command",
					},
				},
			},
		},
		"UserPromptSubmit": {
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"command": cccPath + " hook-user-prompt",
						"type":    "command",
					},
				},
			},
		},
	}

	// Remove ALL existing ccc hooks from all hook types
	allHookTypes := []string{"Stop", "Notification", "PermissionRequest", "PostToolUse", "PreToolUse", "UserPromptSubmit"}
	for _, hookType := range allHookTypes {
		if existing, ok := hooks[hookType].([]interface{}); ok {
			filtered := removeCccHooks(existing)
			if len(filtered) == 0 {
				delete(hooks, hookType)
			} else {
				hooks[hookType] = filtered
			}
		}
	}

	// Add only the hooks we need
	for hookType, newHooks := range cccHooks {
		var existingHooks []interface{}
		if existing, ok := hooks[hookType].([]interface{}); ok {
			existingHooks = existing
		}
		hooks[hookType] = append(newHooks, existingHooks...)
	}

	settings["hooks"] = hooks

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, newData, 0600); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	fmt.Println("✅ Claude hooks installed!")
	return nil
}

func uninstallHook() error {
	home, _ := os.UserHomeDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to read settings.json: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse settings.json: %w", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		fmt.Println("No hooks found")
		return nil
	}

	hookTypes := []string{"Stop", "Notification", "PermissionRequest", "PostToolUse", "PreToolUse", "UserPromptSubmit"}
	for _, hookType := range hookTypes {
		if existing, ok := hooks[hookType].([]interface{}); ok {
			filtered := removeCccHooks(existing)
			if len(filtered) == 0 {
				delete(hooks, hookType)
			} else {
				hooks[hookType] = filtered
			}
		}
	}

	settings["hooks"] = hooks

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, newData, 0600); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	fmt.Println("✅ Claude hooks uninstalled!")
	return nil
}

func installSkill() error {
	home, _ := os.UserHomeDir()
	skillDir := filepath.Join(home, ".claude", "skills")
	skillPath := filepath.Join(skillDir, "ccc-send.md")

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}

	skillContent := `# CCC Send - File Transfer Skill

## Description
Send files to the user via Telegram using the ccc send command.

## Usage
When the user asks you to send them a file, or when you have generated/built a file that the user needs (like an APK, binary, or any other file), use this command:

` + "```bash" + `
ccc send <file_path>
` + "```" + `

## How it works
- **Small files (< 50MB)**: Sent directly via Telegram
- **Large files (≥ 50MB)**: Streamed via relay server with a one-time download link

## Examples

### Send a built APK
` + "```bash" + `
ccc send ./build/app.apk
` + "```" + `

### Send a generated file
` + "```bash" + `
ccc send ./output/report.pdf
` + "```" + `

### Send from subdirectory
` + "```bash" + `
ccc send ~/Downloads/large-file.zip
` + "```" + `

## Important Notes
- The command detects the current session from your working directory
- For large files, the command will wait up to 10 minutes for the user to download
- Each download link is one-time use only
- Use this proactively when you've created files the user needs!
`

	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		return fmt.Errorf("failed to write skill file: %w", err)
	}

	fmt.Println("✅ CCC send skill installed!")
	return nil
}

func uninstallSkill() error {
	home, _ := os.UserHomeDir()
	skillPath := filepath.Join(home, ".claude", "skills", "ccc-send.md")
	os.Remove(skillPath)
	return nil
}

// truncate shortens a string to n characters
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// hookLog writes debug log entries
func hookLog(format string, args ...interface{}) {
	f, err := os.OpenFile("/tmp/ccc-hook-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
