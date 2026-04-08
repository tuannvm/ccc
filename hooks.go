package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
)

// ToolState is an alias for hooks.ToolState
type ToolState = hooks.ToolState

// ToolCall is an alias for hooks.ToolCall
type ToolCall = hooks.ToolCall

// telegramActiveFlag returns the path of the flag file that indicates
// a Telegram message is being processed by a tmux session.
func telegramActiveFlag(tmuxName string) string {
	return filepath.Join(configpkg.CacheDir(), "telegram-active-"+tmuxName)
}

// thinkingFlag returns the path of the flag file that indicates
// Claude is actively processing in a session (for typing indicator).
func thinkingFlag(sessionName string) string {
	return filepath.Join(configpkg.CacheDir(), "thinking-"+sessionName)
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
	return filepath.Join(configpkg.CacheDir(), "prompt-ack-"+sessionName)
}

func writePromptAck(sessionName string) {
	os.WriteFile(promptAckPath(sessionName), []byte("1"), 0600)
}

// toolStatePath returns the path for tool call display state
func toolStatePath(sessionName string) string {
	return filepath.Join(configpkg.CacheDir(), "tools-"+sessionName+".json")
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
