package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tuannvm/ccc/session"
)

// telegramActiveFlag returns the path of the flag file that indicates
// a Telegram message is being processed by a tmux session.
func telegramActiveFlag(tmuxName string) string {
	return filepath.Join(cacheDir(), "telegram-active-"+tmuxName)
}

// thinkingFlag returns the path of the flag file that indicates
// Claude is actively processing in a session (for typing indicator).
func thinkingFlag(sessionName string) string {
	return filepath.Join(cacheDir(), "thinking-"+sessionName)
}

func setThinking(sessionName string) {
	os.WriteFile(thinkingFlag(sessionName), []byte("1"), 0600)
}

func clearThinking(sessionName string) {
	os.Remove(thinkingFlag(sessionName))
}

// promptAckPath returns the path of the ack file that confirms
// Claude received a prompt sent from Telegram via tmux send-keys.
func promptAckPath(sessionName string) string {
	return filepath.Join(cacheDir(), "prompt-ack-"+sessionName)
}

func writePromptAck(sessionName string) {
	os.WriteFile(promptAckPath(sessionName), []byte("1"), 0600)
}

func clearPromptAck(sessionName string) {
	os.Remove(promptAckPath(sessionName))
}

// waitPromptAck polls for the ack file, returning true if it appears within timeout
func waitPromptAck(sessionName string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(promptAckPath(sessionName)); err == nil {
			os.Remove(promptAckPath(sessionName))
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// toolStatePath returns the path for tool call display state
func toolStatePath(sessionName string) string {
	return filepath.Join(cacheDir(), "tools-"+sessionName+".json")
}

// ToolState tracks tool calls and the Telegram message ID for live updates
type ToolState struct {
	MsgID int64      `json:"msg_id"`
	Tools []ToolCall `json:"tools"`
}

type ToolCall struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	IsText bool   `json:"is_text,omitempty"` // true for assistant text
	Time   int64  `json:"time,omitempty"`    // unix ms for ordering
}

func loadToolState(sessionName string) *ToolState {
	data, err := os.ReadFile(toolStatePath(sessionName))
	if err != nil {
		return &ToolState{}
	}
	var state ToolState
	if json.Unmarshal(data, &state) != nil {
		return &ToolState{}
	}
	return &state
}

func saveToolState(sessionName string, state *ToolState) {
	data, _ := json.Marshal(state)
	os.WriteFile(toolStatePath(sessionName), data, 0600)
}

func clearToolState(sessionName string) {
	os.Remove(toolStatePath(sessionName))
}


// addTextToToolState adds an assistant text block to the tool state, ordered by timestamp.
func addTextToToolState(sessName string, text string, ts int64) {
	state := loadToolState(sessName)
	if state.MsgID == 0 {
		return
	}
	state.Tools = append(state.Tools, ToolCall{IsText: true, Input: text, Time: ts})
	// Sort all entries by timestamp
	sort.Slice(state.Tools, func(i, j int) bool {
		return state.Tools[i].Time < state.Tools[j].Time
	})
	saveToolState(sessName, state)
}

// collapseToolMessage is a no-op now (no folding).
func collapseToolMessage(config *Config, sessName string, topicID int64) {
}

// htmlEscape escapes special HTML characters
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// formatToolLines builds tool lines without blockquote wrapper
func formatToolLines(state *ToolState) string {
	var lines []string
	for _, t := range state.Tools {
		if t.IsText {
			lines = append(lines, fmt.Sprintf("💬 %s", htmlEscape(t.Input)))
		} else if t.Name == "" {
			lines = append(lines, fmt.Sprintf("⚙️ %s", htmlEscape(t.Input)))
		} else if t.Input != "" {
			lines = append(lines, fmt.Sprintf("⚙️ %s: %s", htmlEscape(t.Name), htmlEscape(t.Input)))
		} else {
			lines = append(lines, fmt.Sprintf("⚙️ %s", htmlEscape(t.Name)))
		}
	}
	return strings.Join(lines, "\n")
}

// formatToolMessage builds blockquote (expanded during tool calls)
func formatToolMessage(state *ToolState) string {
	return "<blockquote>" + formatToolLines(state) + "</blockquote>"
}

// formatToolMessageCollapsed builds expandable blockquote (after tools complete)
func formatToolMessageCollapsed(state *ToolState) string {
	return "<blockquote expandable>" + formatToolLines(state) + "</blockquote>"
}

// toolInputSummary extracts a short description from tool input
func toolInputSummary(hookData HookData) string {
	truncAt := 80
	trunc := func(s string) string {
		if len(s) > truncAt {
			return s[:truncAt] + "..."
		}
		return s
	}

	switch hookData.ToolName {
	case "Bash":
		return trunc(hookData.ToolInput.Command)
	case "Read", "Write":
		return hookData.ToolInput.FilePath
	case "Edit":
		s := hookData.ToolInput.FilePath
		if hookData.ToolInput.OldString != "" {
			preview := hookData.ToolInput.OldString
			if len(preview) > 40 {
				preview = preview[:40] + "..."
			}
			s += " `" + strings.ReplaceAll(preview, "\n", "↵") + "`"
		}
		return s
	case "Grep":
		if hookData.ToolInput.Pattern != "" {
			return trunc(hookData.ToolInput.Pattern)
		}
		return hookData.ToolInput.Description
	case "Glob":
		if hookData.ToolInput.Pattern != "" {
			return trunc(hookData.ToolInput.Pattern)
		}
		return hookData.ToolInput.Description
	case "WebSearch":
		return trunc(hookData.ToolInput.Query)
	case "WebFetch":
		return trunc(hookData.ToolInput.URL)
	case "Task":
		return trunc(hookData.ToolInput.Description)
	default:
		if hookData.ToolInput.Description != "" {
			return trunc(hookData.ToolInput.Description)
		}
		return ""
	}
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

func handleStopHook() error {
	defer func() {
		if r := recover(); r != nil {
			hookLog("stop-hook: panic recovered: %v", r)
		}
	}()

	hookLog("stop-hook: *** FUNCTION CALLED ***")

	rawData, _ := readHookStdin()
	if len(rawData) == 0 {
		hookLog("stop-hook: no stdin data")
		return nil
	}

	hookData, err := parseHookData(rawData)
	if err != nil {
		hookLog("stop-hook: failed to parse hook data: %v", err)
		return nil
	}

	hookLog("stop-hook: received data: cwd=%s session_id=%s transcript=%s stop_active=%v",
		hookData.Cwd, hookData.SessionID, hookData.TranscriptPath, hookData.StopHookActive)

	config, err := loadConfig()
	if err != nil || config == nil {
		hookLog("stop-hook: failed to load config: %v", err)
		return nil
	}

	sessName, topicID := findSession(config, hookData.Cwd, hookData.SessionID)
	if config.GroupID == 0 {
		hookLog("stop-hook: no group_id configured, skipping message delivery")
		return nil
	}
	// If session was found but has no topic ID, skip delivery (private chat sessions)
	// Do NOT fall back to CWD matching as this could misroute to a different session
	if sessName != "" && topicID == 0 {
		hookLog("stop-hook: session found but no topic_id, skipping message delivery: sess=%s", sessName)
		return nil
	}

	// Track whether we used outer CWD fallback (for later persist logic)
	// Save original session name to detect if outer CWD fallback changed it
	// Note: We cannot reliably detect internal CWD fallback within findSession()
	// (tmux vs CWD), so we only track the outer fallback here.
	usedCwdFallback := false
	originalSessName := sessName

	if sessName == "" || topicID == 0 {
		hookLog("stop-hook: no matching session found: cwd=%s session_id=%s sessName=%s topicID=%d groupID=%d",
			hookData.Cwd, hookData.SessionID, sessName, topicID, config.GroupID)
		hookLog("stop-hook: available sessions: %d", len(config.Sessions))
		for name, info := range config.Sessions {
			hookLog("stop-hook:   - %s: topic=%d path=%s claude_id=%s",
				name, info.TopicID, info.Path, info.ClaudeSessionID)
		}

		// Try to find the best matching session by CWD prefix
		// This handles cases where a skill is invoked directly in Claude Code
		// and the session lookup fails due to missing session_id or tmux mismatch
		//
		// IMPORTANT: Only use CWD fallback when we have a transcript path (indicates a CCC session).
		// This prevents orphaned hooks from ad-hoc Claude Code runs from being misrouted.
		// Note: We don't check if the file exists yet because the transcript might not be
		// flushed when the Stop hook fires. The retry logic handles delayed transcript availability.
		if hookData.TranscriptPath == "" {
			hookLog("stop-hook: no transcript path available, skipping CWD fallback to prevent orphaned hook leakage")
			return nil
		}

		bestMatch := ""
		bestMatchLen := 0
		bestMatchTopicID := int64(0)
		bestMatchIsTeam := false

		// Check regular sessions and team sessions together
		// Choose the longest path match, preferring regular sessions for ties
		// This matches the semantics of findSessionByCwd()
		for name, info := range config.Sessions {
			if info == nil || info.Path == "" || info.TopicID == 0 {
				// Skip sessions without valid topic IDs to prevent dumping to main chat
				continue
			}
			// Check if CWD matches session path exactly or starts with path + "/"
			// This prevents /repo from matching /repo-copy or /repo2
			if hookData.Cwd == info.Path || strings.HasPrefix(hookData.Cwd, info.Path+"/") {
				pathLen := len(info.Path)
				// Prefer longer matches; for ties, prefer regular sessions over team sessions
				if pathLen > bestMatchLen || (pathLen == bestMatchLen && bestMatchIsTeam) {
					bestMatch = name
					bestMatchLen = pathLen
					bestMatchTopicID = info.TopicID
					bestMatchIsTeam = false
				}
			}
		}

		// Check team sessions
		if config.TeamSessions != nil {
			for tid, info := range config.TeamSessions {
				if info == nil || info.Path == "" || tid == 0 {
					// Skip sessions without valid topic IDs
					continue
				}
				// Check if CWD matches session path exactly or starts with path + "/"
				if hookData.Cwd == info.Path || strings.HasPrefix(hookData.Cwd, info.Path+"/") {
					pathLen := len(info.Path)
					// Only choose team session if path is strictly longer (not tie)
					// This ensures team sessions don't override regular sessions at same path
					if pathLen > bestMatchLen {
						bestMatch = info.SessionName
						bestMatchLen = pathLen
						bestMatchTopicID = tid
						bestMatchIsTeam = true
					}
				}
			}
		}

		if bestMatch != "" {
			hookLog("stop-hook: using best match session by CWD: %s (path match length: %d)", bestMatch, bestMatchLen)
			sessName = bestMatch
			topicID = bestMatchTopicID
			// Mark as CWD fallback if the session name changed
			usedCwdFallback = (sessName != originalSessName)
		} else {
			hookLog("stop-hook: no suitable session found, skipping message delivery")
			return nil
		}
	}

	// Persist claude session ID to config for future lookups
	// IMPORTANT: Only persist if we found the session through normal lookup
	// (session_id or tmux match). Do NOT persist CWD fallback guesses as they
	// could be from orphaned hooks and would corrupt the real session's ID.
	if !usedCwdFallback {
		persistClaudeSessionID(config, sessName, hookData.SessionID, hookData.TranscriptPath)
	} else {
		hookLog("stop-hook: using CWD fallback session %s, NOT persisting session_id to prevent corruption", sessName)
	}

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

// deliverUnsentTexts scans transcript tail and sends any assistant text
// blocks not yet delivered to Telegram (using ledger dedup).
// If insertIntoToolMsg is true and tool state has a message, texts are inserted
// into the tool blockquote (for text before/between tools in PreToolUse).
// If false, texts are sent as separate messages (for text after tools in Stop hook).
// claudeSessionID is used to look up the role for team sessions.
func deliverUnsentTexts(config *Config, sessName string, topicID int64, transcriptPath string, insertIntoToolMsg bool, claudeSessionID string) int {
	hookLog("deliver-unsent: sess=%s topic=%d transcript=%s", sessName, topicID, transcriptPath)
	blocks := extractRecentAssistantTexts(transcriptPath, 80)
	lastPreview := ""
	if len(blocks) > 0 {
		lastPreview = truncate(blocks[len(blocks)-1].text, 60)
	}
	hookLog("deliver-unsent: found %d blocks, last=%s", len(blocks), lastPreview)

	// Determine message prefix for team sessions
	// For team sessions, look up the role by matching Claude session ID to panes
	rolePrefix := ""
	hookLog("deliver-unsent: checking team session: topicID=%d, claudeSessionID=%s", topicID, claudeSessionID)
	if config.IsTeamSession(topicID) {
		hookLog("deliver-unsent: is team session")
		// Try to find the role by matching Claude session ID to panes
		if sessInfo, exists := config.GetTeamSession(topicID); exists && sessInfo != nil && sessInfo.Panes != nil {
			hookLog("deliver-unsent: got sessInfo with %d panes", len(sessInfo.Panes))
			for role, pane := range sessInfo.Panes {
				if pane != nil {
					hookLog("deliver-unsent: pane role=%s, claudeSessionID=%s", role, pane.ClaudeSessionID)
					if pane.ClaudeSessionID == claudeSessionID {
						// Found the matching pane, get role prefix
						rolePrefixes := map[session.PaneRole]string{
							session.RolePlanner:  "[Planner] ",
							session.RoleExecutor: "[Executor] ",
							session.RoleReviewer: "[Reviewer] ",
						}
						if prefix, ok := rolePrefixes[role]; ok {
							rolePrefix = prefix
							hookLog("deliver-unsent: found role=%s for claude_session_id=%s", role, claudeSessionID)
						}
						break
					}
				}
			}
		}
		// Fallback 1: Try to infer role from transcript path (same logic as persistClaudeSessionID)
		if rolePrefix == "" && transcriptPath != "" {
			hookLog("deliver-unsent: role not found via panes, trying transcript path inference")
			role := inferRoleFromTranscriptPath(transcriptPath)
			if role != "" {
				rolePrefixes := map[session.PaneRole]string{
					session.RolePlanner:  "[Planner] ",
					session.RoleExecutor: "[Executor] ",
					session.RoleReviewer: "[Reviewer] ",
				}
				if prefix, ok := rolePrefixes[role]; ok {
					rolePrefix = prefix
					hookLog("deliver-unsent: inferred role=%s from transcript path", role)
				}
			}
		}
		// Fallback 2: check CCC_ROLE environment variable (may not work due to process isolation)
		if rolePrefix == "" {
			hookLog("deliver-unsent: role not found via transcript, trying CCC_ROLE env var")
			if cccRole := os.Getenv("CCC_ROLE"); cccRole != "" {
				rolePrefixes := map[string]string{
					"planner":  "[Planner] ",
					"executor": "[Executor] ",
					"reviewer": "[Reviewer] ",
				}
				if prefix, ok := rolePrefixes[cccRole]; ok {
					rolePrefix = prefix
					hookLog("deliver-unsent: using CCC_ROLE env var=%s", cccRole)
				}
			} else {
				hookLog("deliver-unsent: CCC_ROLE env var is empty")
			}
		}
	} else {
		hookLog("deliver-unsent: not a team session")
	}
	hookLog("deliver-unsent: final rolePrefix=%s", rolePrefix)

	sent := 0
	for _, block := range blocks {
		blockID := fmt.Sprintf("reply:%s:%s", block.requestID, contentHash(block.text))
		if isDelivered(sessName, blockID, "telegram") {
			continue
		}
		hookLog("deliver-text: rid=%s len=%d insert=%v preview=%s", block.requestID, len(block.text), insertIntoToolMsg, truncate(block.text, 80))

		state := loadToolState(sessName)
		if insertIntoToolMsg && state.MsgID != 0 {
			// Insert into tool blockquote at correct time position
			addTextToToolState(sessName, block.text, time.Now().UnixMilli())
			state = loadToolState(sessName)
			text := formatToolMessage(state)
			editMessageHTML(config, config.GroupID, state.MsgID, topicID, text)
			appendMessage(&MessageRecord{
				ID: blockID, Session: sessName, Type: "assistant_text",
				Text: truncate(block.text, 500), Origin: "claude",
				TerminalDelivered: true, TelegramDelivered: true, TelegramMsgID: state.MsgID,
			})
		} else {
			// Send as separate message with role prefix for team sessions
			// Format: *topic-name:* [Role] message
			msg := fmt.Sprintf("*%s:* %s%s", sessName, rolePrefix, block.text)
			tgMsgID, err := sendAssistantMessage(config, config.GroupID, topicID, msg)
			if err != nil {
				// If thread not found, retry without thread_id
				if strings.Contains(err.Error(), "message thread not found") && topicID != 0 {
					hookLog("deliver-text: thread not found, retrying without thread_id")
					time.Sleep(500 * time.Millisecond)
					tgMsgID, _ = sendMessageGetID(config, config.GroupID, 0, msg)
				} else {
					hookLog("deliver-text: send failed, retrying: %v", err)
					time.Sleep(500 * time.Millisecond)
					tgMsgID, _ = sendMessageGetID(config, config.GroupID, topicID, msg)
				}
			}
			appendMessage(&MessageRecord{
				ID: blockID, Session: sessName, Type: "assistant_text",
				Text: truncate(block.text, 500), Origin: "claude",
				TerminalDelivered: true, TelegramDelivered: tgMsgID > 0, TelegramMsgID: tgMsgID,
			})
		}
		sent++
	}
	return sent
}

// assistantTextBlock pairs extracted text with its requestId for dedup
type assistantTextBlock struct {
	requestID string
	text      string
}

// extractRecentAssistantTexts reads the last N assistant entries from the
// transcript and returns their text blocks. The caller uses ledger dedup
// to avoid resending previously delivered messages.
func extractRecentAssistantTexts(transcriptPath string, tailCount int) []assistantTextBlock {
	if transcriptPath == "" {
		hookLog("extract: empty transcript path")
		return nil
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		hookLog("extract: failed to open transcript %s: %v", transcriptPath, err)
		return nil
	}
	defer f.Close()

	type transcriptLine struct {
		Type              string `json:"type"`
		UUID              string `json:"uuid,omitempty"`
		RequestID         string `json:"requestId,omitempty"`
		IsApiErrorMessage bool   `json:"isApiErrorMessage,omitempty"`
		Message           struct {
			ID      string          `json:"id,omitempty"`
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}

	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}

	// Read only the tail of the file (last 512KB) to avoid scanning the entire transcript
	const tailBytes = 512 * 1024
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	offset := int64(0)
	if fi.Size() > tailBytes {
		offset = fi.Size() - tailBytes
		f.Seek(offset, 0)
	}
	tailData, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	// If we seeked into the middle of a line, skip the first partial line
	if offset > 0 {
		if idx := bytes.IndexByte(tailData, '\n'); idx >= 0 {
			tailData = tailData[idx+1:]
		}
	}

	type entry struct {
		requestID string
		content   json.RawMessage
	}

	var entries []entry
	for _, line := range bytes.Split(tailData, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var tl transcriptLine
		if json.Unmarshal(line, &tl) != nil {
			continue
		}
		if tl.Type != "assistant" || tl.Message.Role != "assistant" {
			continue
		}
		if tl.IsApiErrorMessage {
			continue
		}
		// Fall back to uuid or message.id for ZAI format
		rid := tl.RequestID
		if rid == "" {
			rid = tl.UUID
		}
		if rid == "" {
			rid = tl.Message.ID
		}
		if rid == "" {
			continue
		}
		entries = append(entries, entry{
			requestID: rid,
			content:   tl.Message.Content,
		})
	}

	// Take only the tail
	if len(entries) > tailCount {
		entries = entries[len(entries)-tailCount:]
	}

	// For each requestId, keep only the last entry's text (later entries
	// supersede earlier ones for the same request, e.g. streaming updates)
	type ridText struct {
		requestID string
		texts     []string
	}
	seen := make(map[string]int) // requestID -> index in result
	var ordered []ridText

	for _, e := range entries {
		var blocks []contentBlock
		if json.Unmarshal(e.content, &blocks) != nil {
			continue
		}
		var texts []string
		for _, b := range blocks {
			if b.Type != "text" {
				continue
			}
			t := strings.TrimSpace(b.Text)
			if t != "" && t != "(no content)" {
				texts = append(texts, t)
			}
		}
		if len(texts) == 0 {
			continue
		}
		if idx, ok := seen[e.requestID]; ok {
			ordered[idx].texts = texts // overwrite with later entry
		} else {
			seen[e.requestID] = len(ordered)
			ordered = append(ordered, ridText{requestID: e.requestID, texts: texts})
		}
	}

	var result []assistantTextBlock
	for _, rt := range ordered {
		for _, t := range rt.texts {
			result = append(result, assistantTextBlock{requestID: rt.requestID, text: t})
		}
	}
	return result
}

// handleStopRetry is a background process spawned by stop hook.
// It retries transcript reading 3 times at 2-second intervals to catch
// messages that weren't flushed when the stop hook first fired.
func handleStopRetry(sessName string, topicID int64, transcriptPath string) error {
	config, err := loadConfig()
	if err != nil || config == nil {
		return nil
	}
	for i := 0; i < 3; i++ {
		time.Sleep(2 * time.Second)
		// Note: retry doesn't have access to claudeSessionID, pass empty string
		n := deliverUnsentTexts(config, sessName, topicID, transcriptPath, false, "")
		hookLog("stop-retry: %d/3 sent=%d session=%s", i+1, n, sessName)
	}
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
	// NOTE: Hook installation is now done per-project via installHooksForProject()
	// This global install function is kept for backward compatibility but does nothing
	fmt.Println("⚠️ Global hook installation is deprecated. Hooks are now installed per-project automatically.")
	fmt.Println("💡 Hooks will be automatically installed when you create or resume a session.")
	return nil
}

// installHooksForProject installs ccc hooks to a project's .claude/settings.local.json
func installHooksForProject(projectPath string) error {
	settingsLocalPath := filepath.Join(projectPath, ".claude", "settings.local.json")

	// Ensure .claude directory exists
	claudeDir := filepath.Dir(settingsLocalPath)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	// Install hooks
	if err := installHooksToPath(settingsLocalPath, true); err != nil {
		return fmt.Errorf("failed to install hooks to %s: %w", settingsLocalPath, err)
	}

	hookLog("install-hooks: installed to %s", settingsLocalPath)
	return nil
}

// verifyHooksForProject checks if ccc hooks are present in a project's .claude/settings.local.json
func verifyHooksForProject(projectPath string) bool {
	settingsLocalPath := filepath.Join(projectPath, ".claude", "settings.local.json")

	data, err := os.ReadFile(settingsLocalPath)
	if err != nil {
		hookLog("verify-hooks: no settings.local.json at %s", settingsLocalPath)
		return false
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		hookLog("verify-hooks: failed to parse settings.local.json: %v", err)
		return false
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hookLog("verify-hooks: no hooks in settings.local.json")
		return false
	}

	// Check if ccc hooks are present
	hasCccHooks := false
	requiredHooks := []string{"PreToolUse", "Stop", "UserPromptSubmit", "Notification"}

	for _, hookType := range requiredHooks {
		if hookEntries, exists := hooks[hookType].([]interface{}); exists {
			for _, entry := range hookEntries {
				if entryMap, ok := entry.(map[string]interface{}); ok {
					if cmd, ok := entryMap["command"].(string); ok {
						if strings.Contains(cmd, "ccc hook-") {
							hasCccHooks = true
							break
						}
					}
				}
			}
		}
	}

	hookLog("verify-hooks: hasCccHooks=%v for %s", hasCccHooks, projectPath)
	return hasCccHooks
}

func installHooksToPath(settingsPath string, isLocal bool) error {
	// Ensure directory exists
	settingsDir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Read existing settings or create new
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		// File doesn't exist, create empty settings
		settings = make(map[string]interface{})
	} else if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse settings: %w", err)
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
		"PostToolUse": {
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"command": cccPath + " hook-post-tool",
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
		"Notification": {
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"command": cccPath + " hook-notification",
						"type":    "command",
					},
				},
			},
		},
	}

	// For settings.local.json, we completely replace hooks (not merge)
	// This ensures only ccc hooks are in the project-local settings
	if isLocal {
		// Remove ALL existing ccc hooks from all hook types (clean slate)
		allHookTypes := []string{"Stop", "Notification", "PermissionRequest", "PostToolUse", "PreToolUse", "UserPromptSubmit"}
		for _, hookType := range allHookTypes {
			delete(hooks, hookType)
		}

		// Add only our hooks (no merging)
		for hookType, newHooks := range cccHooks {
			hooks[hookType] = newHooks
		}
	} else {
		// Legacy behavior for global settings: merge with existing hooks
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
	}

	settings["hooks"] = hooks

	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, newData, 0600); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	return nil
}

func uninstallHook() error {
	// NOTE: Per-project hooks are managed via settings.local.json in each project
	// This global uninstall function is kept for backward compatibility but does nothing
	fmt.Println("⚠️ Global hook uninstallation is deprecated.")
	fmt.Println("💡 To remove hooks from a project, delete the .claude/settings.local.json file in that project.")
	fmt.Println("💡 To cleanup old global hooks, use: ccc cleanup-hooks")
	return nil
}

// cleanupGlobalHooks removes ccc hooks from global config files
// This is used to clean up old installations that installed hooks to global settings
func cleanupGlobalHooks() error {
	home, _ := os.UserHomeDir()
	defaultSettingsPath := filepath.Join(home, ".claude", "settings.json")

	// Load config to get all provider config dirs
	config, err := loadConfig()
	cleanedCount := 0
	configDirs := make(map[string]bool)

	if err == nil && config.Providers != nil {
		// Collect all unique config dirs
		for _, provider := range config.Providers {
			if provider.ConfigDir != "" {
				// Expand ~
				configDir := provider.ConfigDir
				if strings.HasPrefix(configDir, "~/") {
					configDir = filepath.Join(home, configDir[2:])
				} else if configDir == "~" {
					configDir = home
				}
				configDirs[configDir] = true
			}
		}
	}

	// Cleanup hooks from each provider config dir
	for configDir := range configDirs {
		providerSettingsPath := filepath.Join(configDir, "settings.json")
		if _, err := os.Stat(providerSettingsPath); err == nil {
			if err := uninstallHooksFromPath(providerSettingsPath); err != nil {
				fmt.Printf("⚠️ Failed to cleanup hooks from %s: %v\n", configDir, err)
			} else {
				fmt.Printf("✅ Cleaned up hooks from %s\n", configDir)
				cleanedCount++
			}
		}
	}

	// Always cleanup from default ~/.claude
	if _, err := os.Stat(defaultSettingsPath); err == nil {
		if err := uninstallHooksFromPath(defaultSettingsPath); err != nil {
			fmt.Printf("⚠️ Failed to cleanup hooks from %s: %v\n", defaultSettingsPath, err)
		} else {
			fmt.Printf("✅ Cleaned up hooks from %s\n", defaultSettingsPath)
			cleanedCount++
		}
	}

	if cleanedCount == 0 {
		fmt.Println("✨ No global hooks found to cleanup")
		return nil
	}

	fmt.Printf("✅ Cleaned up ccc hooks from %d location(s)\n", cleanedCount)
	fmt.Println("💡 Hooks are now managed per-project in .claude/settings.local.json")
	return nil
}

// uninstallHooksFromProject removes ccc hooks from a specific project's settings.local.json
func uninstallHooksFromProject(projectPath string) error {
	settingsLocalPath := filepath.Join(projectPath, ".claude", "settings.local.json")

	// Check if file exists
	if _, err := os.Stat(settingsLocalPath); os.IsNotExist(err) {
		return nil // Nothing to uninstall
	}

	return uninstallHooksFromPath(settingsLocalPath)
}

// uninstallHooksFromPath removes ccc hooks from a specific settings.json file
func uninstallHooksFromPath(settingsPath string) error {
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
		return nil // No hooks to remove
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

// installHooksToCurrentDir installs ccc hooks to the current directory's .claude/settings.local.json
// This is used by the 'ccc install-hooks' command
func installHooksToCurrentDir() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check if hooks are already installed
	if verifyHooksForProject(cwd) {
		fmt.Printf("✅ Hooks already installed in %s\n", cwd)
		return nil
	}

	// Install hooks
	if err := installHooksForProject(cwd); err != nil {
		return fmt.Errorf("failed to install hooks: %w", err)
	}

	fmt.Printf("✅ Hooks installed to %s/.claude/settings.local.json\n", cwd)
	return nil
}

// ensureHooksForSession ensures ccc hooks are installed in the session's project directory
// This should be called when a session is created or resumed
func ensureHooksForSession(config *Config, sessionName string, sessionInfo *SessionInfo) error {
	if sessionInfo == nil {
		if config == nil || config.Sessions == nil {
			return nil
		}
		sessionInfo = config.Sessions[sessionName]
		if sessionInfo == nil {
			return nil
		}
	}

	// Get the project path for this session
	projectPath := getSessionWorkDir(config, sessionName, sessionInfo)
	if projectPath == "" {
		return fmt.Errorf("unable to determine project path for session '%s'", sessionName)
	}

	// Check if hooks are already installed
	if verifyHooksForProject(projectPath) {
		hookLog("ensure-hooks: hooks already present for %s", projectPath)
		return nil
	}

	// Install hooks to the project
	hookLog("ensure-hooks: installing hooks to %s", projectPath)
	if err := installHooksForProject(projectPath); err != nil {
		return fmt.Errorf("failed to install hooks for project %s: %w", projectPath, err)
	}

	hookLog("ensure-hooks: hooks installed successfully for %s", projectPath)
	return nil
}

// ========== Streaming Integration (API 9.5) ==========

// sendAssistantMessage sends an assistant text message with optional streaming
// If config.EnableStreaming is true, uses sendMessageDraft for real-time typing effect
// Otherwise, falls back to standard sendMessageGetID
func sendAssistantMessage(config *Config, chatID int64, threadID int64, text string) (int64, error) {
	return sendStreamingMessage(config, chatID, threadID, text, config.EnableStreaming)
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
	f, err := os.OpenFile(filepath.Join(cacheDir(), "hook-debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
