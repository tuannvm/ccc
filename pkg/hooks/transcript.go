package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/session"
)

// AssistantTextBlock pairs extracted text with its requestId for dedup
type AssistantTextBlock struct {
	RequestID string
	Text      string
}

// ExtractRecentAssistantTexts reads the last N assistant entries from the
// transcript and returns their text blocks. The caller uses ledger dedup
// to avoid resending previously delivered messages.
func ExtractRecentAssistantTexts(transcriptPath string, tailCount int) []AssistantTextBlock {
	if transcriptPath == "" {
		HookLog("extract: empty transcript path")
		return nil
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		HookLog("extract: failed to open transcript %s: %v", transcriptPath, err)
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

	var result []AssistantTextBlock
	for _, rt := range ordered {
		for _, t := range rt.texts {
			result = append(result, AssistantTextBlock{RequestID: rt.requestID, Text: t})
		}
	}
	return result
}

// DeliverUnsentTextsConfig holds the configuration for delivering unsent texts
type DeliverUnsentTextsConfig struct {
	Config            *config.Config
	SessionName       string
	TopicID           int64
	TranscriptPath    string
	InsertIntoToolMsg bool
	ClaudeSessionID   string
	// Callbacks for root-level dependencies
	LoadToolState      func(sessionName string) *ToolState
	AddTextToToolState func(sessName string, text string, ts int64)
	SaveToolState      func(sessionName string, state *ToolState)
	FormatToolMessage  func(state *ToolState) string
	EditMessageHTML    func(cfg *config.Config, chatID int64, msgID int64, threadID int64, text string) error
	SendMessageHTML    func(cfg *config.Config, chatID int64, threadID int64, text string) (int64, error)
	SendMessageGetID   func(cfg *config.Config, chatID int64, threadID int64, text string) (int64, error)
	SendMessage        func(cfg *config.Config, chatID int64, threadID int64, text string) error
	IsDelivered        func(sessName, id, origin string) bool
	AppendMessage      func(msg *ledger.MessageRecord)
	ClearToolState     func(sessionName string)
	InferRoleFromTranscriptPath func(transcriptPath string) session.PaneRole
}

// DeliverUnsentTexts scans transcript tail and sends any assistant text
// blocks not yet delivered to Telegram (using ledger dedup).
// If insertIntoToolMsg is true and tool state has a message, texts are inserted
// into the tool blockquote (for text before/between tools in PreToolUse).
// If false, texts are sent as separate messages (for text after tools in Stop hook).
// claudeSessionID is used to look up the role for team sessions.
func DeliverUnsentTexts(cfg *DeliverUnsentTextsConfig) int {
	HookLog("deliver-unsent: sess=%s topic=%d transcript=%s", cfg.SessionName, cfg.TopicID, cfg.TranscriptPath)
	blocks := ExtractRecentAssistantTexts(cfg.TranscriptPath, 80)
	lastPreview := ""
	if len(blocks) > 0 {
		lastPreview = Truncate(blocks[len(blocks)-1].Text, 60)
	}
	HookLog("deliver-unsent: found %d blocks, last=%s", len(blocks), lastPreview)

	// Determine message prefix for team sessions
	// For team sessions, look up the role by matching Claude session ID to panes
	rolePrefix := ""
	HookLog("deliver-unsent: checking team session: topicID=%d, claudeSessionID=%s", cfg.TopicID, cfg.ClaudeSessionID)
	if cfg.Config.IsTeamSession(cfg.TopicID) {
		HookLog("deliver-unsent: is team session")
		// Try to find the role by matching Claude session ID to panes
		if sessInfo, exists := cfg.Config.GetTeamSession(cfg.TopicID); exists && sessInfo != nil && sessInfo.Panes != nil {
			HookLog("deliver-unsent: got sessInfo with %d panes", len(sessInfo.Panes))
			for role, pane := range sessInfo.Panes {
				if pane != nil {
					HookLog("deliver-unsent: pane role=%s, claudeSessionID=%s", role, pane.ClaudeSessionID)
					if pane.ClaudeSessionID == cfg.ClaudeSessionID {
						// Found the matching pane, get role prefix
						rolePrefixes := map[session.PaneRole]string{
							session.RolePlanner:  "[Planner] ",
							session.RoleExecutor: "[Executor] ",
							session.RoleReviewer: "[Reviewer] ",
						}
						if prefix, ok := rolePrefixes[role]; ok {
							rolePrefix = prefix
							HookLog("deliver-unsent: found role=%s for claude_session_id=%s", role, cfg.ClaudeSessionID)
						}
						break
					}
				}
			}
		}
		// Fallback 1: Try to infer role from transcript path (same logic as persistClaudeSessionID)
		if rolePrefix == "" && cfg.TranscriptPath != "" {
			HookLog("deliver-unsent: role not found via panes, trying transcript path inference")
			role := cfg.InferRoleFromTranscriptPath(cfg.TranscriptPath)
			if role != "" {
				rolePrefixes := map[session.PaneRole]string{
					session.RolePlanner:  "[Planner] ",
					session.RoleExecutor: "[Executor] ",
					session.RoleReviewer: "[Reviewer] ",
				}
				if prefix, ok := rolePrefixes[role]; ok {
					rolePrefix = prefix
					HookLog("deliver-unsent: inferred role=%s from transcript path", role)
				}
			}
		}
		// Fallback 2: check CCC_ROLE environment variable (may not work due to process isolation)
		if rolePrefix == "" {
			HookLog("deliver-unsent: role not found via transcript, trying CCC_ROLE env var")
			if cccRole := os.Getenv("CCC_ROLE"); cccRole != "" {
				rolePrefixes := map[string]string{
					"planner":  "[Planner] ",
					"executor": "[Executor] ",
					"reviewer": "[Reviewer] ",
				}
				if prefix, ok := rolePrefixes[cccRole]; ok {
					rolePrefix = prefix
					HookLog("deliver-unsent: using CCC_ROLE env var=%s", cccRole)
				}
			} else {
				HookLog("deliver-unsent: CCC_ROLE env var is empty")
			}
		}
	} else {
		HookLog("deliver-unsent: not a team session")
	}
	HookLog("deliver-unsent: final rolePrefix=%s", rolePrefix)

	sent := 0
	for _, block := range blocks {
		blockID := fmt.Sprintf("reply:%s:%s", block.RequestID, ledger.ContentHash(block.Text))
		if cfg.IsDelivered(cfg.SessionName, blockID, "telegram") {
			continue
		}
		HookLog("deliver-text: rid=%s len=%d insert=%v preview=%s", block.RequestID, len(block.Text), cfg.InsertIntoToolMsg, Truncate(block.Text, 80))

		state := cfg.LoadToolState(cfg.SessionName)
		if cfg.InsertIntoToolMsg && state.MsgID != 0 {
			// Insert into tool blockquote at correct time position
			cfg.AddTextToToolState(cfg.SessionName, block.Text, time.Now().UnixMilli())
			state = cfg.LoadToolState(cfg.SessionName)
			text := cfg.FormatToolMessage(state)
			cfg.EditMessageHTML(cfg.Config, cfg.Config.GroupID, state.MsgID, cfg.TopicID, text)
			cfg.AppendMessage(&ledger.MessageRecord{
				ID:                blockID,
				Session:           cfg.SessionName,
				Type:              "assistant_text",
				Text:              Truncate(block.Text, 500),
				Origin:            "claude",
				TerminalDelivered: true,
				TelegramDelivered: true,
				TelegramMsgID:     state.MsgID,
			})
		} else {
			// Send as separate message with role prefix for team sessions
			// Format: *topic-name:* [Role] message
			msg := fmt.Sprintf("*%s:* %s%s", cfg.SessionName, rolePrefix, block.Text)
			tgMsgID, err := cfg.SendMessageHTML(cfg.Config, cfg.Config.GroupID, cfg.TopicID, msg)
			if err != nil {
				// If thread not found, retry without thread_id
				if strings.Contains(err.Error(), "message thread not found") && cfg.TopicID != 0 {
					HookLog("deliver-text: thread not found, retrying without thread_id")
					time.Sleep(500 * time.Millisecond)
					tgMsgID, _ = cfg.SendMessageGetID(cfg.Config, cfg.Config.GroupID, 0, msg)
				} else {
					HookLog("deliver-text: send failed, retrying: %v", err)
					time.Sleep(500 * time.Millisecond)
					tgMsgID, _ = cfg.SendMessageGetID(cfg.Config, cfg.Config.GroupID, cfg.TopicID, msg)
				}
			}
			cfg.AppendMessage(&ledger.MessageRecord{
				ID:                blockID,
				Session:           cfg.SessionName,
				Type:              "assistant_text",
				Text:              Truncate(block.Text, 500),
				Origin:            "claude",
				TerminalDelivered: true,
				TelegramDelivered: tgMsgID > 0,
				TelegramMsgID:     tgMsgID,
			})
		}
		sent++
	}
	return sent
}

// Truncate shortens a string to n characters
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// HookLog writes debug log entries
func HookLog(format string, args ...any) {
	f, err := os.OpenFile(filepath.Join(config.CacheDir(), "hook-debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// SendAssistantMessage sends an assistant text message with optional streaming
// If config.EnableStreaming is true, uses telegram.SendDraftMessage for real-time typing effect
// Otherwise, falls back to standard telegram.SendMessage
func SendAssistantMessage(cfg *config.Config, chatID int64, threadID int64, text string) (int64, error) {
	return telegram.SendStreamingMessage(cfg, chatID, threadID, text, cfg.EnableStreaming)
}

// HandleStopRetryConfig holds configuration for stop retry handler
type HandleStopRetryConfig struct {
	SessionName    string
	TopicID        int64
	TranscriptPath string
	LoadConfig     func() (*config.Config, error)
	// DeliverUnsentTexts callback for retrying message delivery
	DeliverUnsentTexts func(config *config.Config, sessName string, topicID int64, transcriptPath string, insertIntoToolMsg bool, claudeSessionID string) int
}

// HandleStopRetry is a background process spawned by stop hook.
// It retries transcript reading 3 times at 2-second intervals to catch
// messages that weren't flushed when the stop hook first fired.
func HandleStopRetry(cfg *HandleStopRetryConfig) error {
	config, err := cfg.LoadConfig()
	if err != nil || config == nil {
		return nil
	}
	for i := range 3 {
		time.Sleep(2 * time.Second)
		// Note: retry doesn't have access to claudeSessionID, pass empty string
		n := cfg.DeliverUnsentTexts(config, cfg.SessionName, cfg.TopicID, cfg.TranscriptPath, false, "")
		HookLog("stop-retry: %d/3 sent=%d session=%s", i+1, n, cfg.SessionName)
	}
	return nil
}

// HandleStopRetryFromArgs parses CLI args for the hook-stop-retry command.
func HandleStopRetryFromArgs(args []string, handleRetry func(string, int64, string) error) {
	if len(args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: ccc hook-stop-retry <session> <topicID> <transcript>\n")
		os.Exit(1)
	}
	var tid int64
	if _, err := fmt.Sscan(args[1], &tid); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid topicID %q: %v\n", args[1], err)
		os.Exit(1)
	}
	if err := handleRetry(args[0], tid, args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "Stop retry failed: %v\n", err)
		os.Exit(1)
	}
}

// ToolState tracks tool calls and the Telegram message ID for live updates
type ToolState struct {
	MsgID int64     `json:"msg_id"`
	Tools []ToolCall `json:"tools"`
}

// ToolCall represents a single tool call or text block
type ToolCall struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	IsText bool   `json:"is_text,omitempty"` // true for assistant text
	Time   int64  `json:"time,omitempty"`    // unix ms for ordering
}
