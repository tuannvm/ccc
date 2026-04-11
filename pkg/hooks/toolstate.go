package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/config"
)

// HtmlEscape escapes special HTML characters
func HtmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// FormatToolLines builds tool lines without blockquote wrapper
func FormatToolLines(state *ToolState) string {
	var lines []string
	for _, t := range state.Tools {
		if t.IsText {
			lines = append(lines, fmt.Sprintf("💬 %s", HtmlEscape(t.Input)))
		} else if t.Name == "" {
			lines = append(lines, fmt.Sprintf("⚙️ %s", HtmlEscape(t.Input)))
		} else if t.Input != "" {
			lines = append(lines, fmt.Sprintf("⚙️ %s: %s", HtmlEscape(t.Name), HtmlEscape(t.Input)))
		} else {
			lines = append(lines, fmt.Sprintf("⚙️ %s", HtmlEscape(t.Name)))
		}
	}
	return strings.Join(lines, "\n")
}

// FormatToolMessage builds blockquote (expanded during tool calls)
func FormatToolMessage(state *ToolState) string {
	return "<blockquote>" + FormatToolLines(state) + "</blockquote>"
}

// ToolInputSummary extracts a short description from tool input
func ToolInputSummary(hookData HookData) string {
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

// ToolStatePath returns the path for tool call display state
func ToolStatePath(sessionName string) string {
	return config.CacheDir() + "/tools-" + sessionName + ".json"
}

// LoadToolState loads tool state from disk
func LoadToolState(sessionName string) *ToolState {
	data, err := os.ReadFile(ToolStatePath(sessionName))
	if err != nil {
		return &ToolState{}
	}
	var state ToolState
	if json.Unmarshal(data, &state) != nil {
		return &ToolState{}
	}
	return &state
}

// SaveToolState saves tool state to disk
func SaveToolState(sessionName string, state *ToolState) {
	data, _ := json.Marshal(state)
	os.WriteFile(ToolStatePath(sessionName), data, 0600)
}

// ClearToolState removes tool state from disk
func ClearToolState(sessionName string) {
	os.Remove(ToolStatePath(sessionName))
}

// AddTextToToolState adds an assistant text block to the tool state, ordered by timestamp
func AddTextToToolState(sessName string, text string, ts int64) {
	state := LoadToolState(sessName)
	if state.MsgID == 0 {
		return
	}
	state.Tools = append(state.Tools, ToolCall{IsText: true, Input: text, Time: ts})
	// Sort all entries by timestamp
	sort.Slice(state.Tools, func(i, j int) bool {
		return state.Tools[i].Time < state.Tools[j].Time
	})
	SaveToolState(sessName, state)
}

// ReadHookStdin reads stdin JSON with a timeout
func ReadHookStdin() ([]byte, error) {
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
