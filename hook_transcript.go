package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tuannvm/ccc/session"
)

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
	for i := range 3 {
		time.Sleep(2 * time.Second)
		// Note: retry doesn't have access to claudeSessionID, pass empty string
		n := deliverUnsentTexts(config, sessName, topicID, transcriptPath, false, "")
		hookLog("stop-retry: %d/3 sent=%d session=%s", i+1, n, sessName)
	}
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
func hookLog(format string, args ...any) {
	f, err := os.OpenFile(filepath.Join(cacheDir(), "hook-debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
