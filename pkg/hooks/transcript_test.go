package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtractRecentAssistantTexts tests parsing transcript JSONL files
func TestExtractRecentAssistantTexts(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name     string
		content  string
		expected []string // expected texts in order
	}{
		{
			name:     "simple response with one text block",
			content:  `{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"Hello! How can I help?"}]}}`,
			expected: []string{"Hello! How can I help?"},
		},
		{
			name:     "multiple text blocks in one entry",
			content:  `{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"First part"},{"type":"text","text":"Second part"}]}}`,
			expected: []string{"First part", "Second part"},
		},
		{
			name:     "filters thinking and tool_use",
			content:  `{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"thinking","thinking":"let me think..."},{"type":"text","text":"Here is my answer"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
			expected: []string{"Here is my answer"},
		},
		{
			name: "streaming dedup same requestId keeps last",
			content: `{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"partial response..."}]}}
		{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"complete response with more detail"}]}}`,
			expected: []string{"complete response with more detail"},
		},
		{
			name: "returns ALL turns (not just last)",
			content: `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"first question"}]}}
		{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"first answer"}]}}
		{"type":"user","message":{"role":"user","content":[{"type":"text","text":"second question"}]}}
		{"type":"assistant","requestId":"req_4","message":{"role":"assistant","content":[{"type":"text","text":"second answer"}]}}`,
			expected: []string{"first answer", "second answer"},
		},
		{
			name:     "empty file returns nil",
			content:  "",
			expected: nil,
		},
		{
			name:     "no assistant messages returns nil",
			content:  `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
			expected: nil,
		},
		{
			name:     "filters no content",
			content:  `{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"(no content)"},{"type":"text","text":"real content"}]}}`,
			expected: []string{"real content"},
		},
		{
			name:     "codex response item output text",
			content:  `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Codex answer"}],"phase":"final_answer"}}`,
			expected: []string{"Codex answer"},
		},
		{
			name: "skips error entries without requestId",
			content: `{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"good"}]}}
		{"type":"assistant","isApiErrorMessage":true,"message":{"role":"assistant","content":[{"type":"text","text":"No response requested."}]}}`,
			expected: []string{"good"},
		},
		{
			name: "multiple requestIds all returned",
			content: `{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"running tool"}]}}
		{"type":"assistant","requestId":"req_4","message":{"role":"assistant","content":[{"type":"text","text":"tool completed"}]}}`,
			expected: []string{"running tool", "tool completed"},
		},
		{
			name: "tail count limits results",
			content: `{"type":"assistant","requestId":"req_1","message":{"role":"assistant","content":[{"type":"text","text":"old message"}]}}
		{"type":"assistant","requestId":"req_2","message":{"role":"assistant","content":[{"type":"text","text":"recent message"}]}}`,
			expected: []string{"recent message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, tt.name+".jsonl")
			if err := os.WriteFile(filePath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			tailCount := 80
			if tt.name == "tail count limits results" {
				tailCount = 1 // only keep last entry
			}
			blocks := ExtractRecentAssistantTexts(filePath, tailCount)
			var result []string
			for _, b := range blocks {
				result = append(result, b.Text)
			}
			if tt.expected == nil {
				if result != nil {
					t.Errorf("got %v, want nil", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("returned %d blocks, want %d: %v", len(result), len(tt.expected), result)
				return
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("block %d = %q, want %q", i, result[i], exp)
				}
			}
		})
	}
}

// TestExtractRecentNonExistent tests with non-existent file
func TestExtractRecentNonExistent(t *testing.T) {
	result := ExtractRecentAssistantTexts("/nonexistent/path/file.jsonl", 80)
	if result != nil {
		t.Errorf("non-existent file = %v, want nil", result)
	}
}

// TestExtractRecentEmptyPath tests with empty path
func TestExtractRecentEmptyPath(t *testing.T) {
	result := ExtractRecentAssistantTexts("", 80)
	if result != nil {
		t.Errorf("empty path = %v, want nil", result)
	}
}

// TestHookDataJSON tests HookData JSON parsing
func TestHookDataJSON(t *testing.T) {
	jsonStr := `{"cwd":"/Users/test/project","transcript_path":"/tmp/transcript.jsonl","session_id":"abc123"}`

	var hookData HookData
	if err := ParseHookDataTo(&hookData, []byte(jsonStr)); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if hookData.Cwd != "/Users/test/project" {
		t.Errorf("Cwd = %q, want %q", hookData.Cwd, "/Users/test/project")
	}
	if hookData.TranscriptPath != "/tmp/transcript.jsonl" {
		t.Errorf("TranscriptPath = %q, want %q", hookData.TranscriptPath, "/tmp/transcript.jsonl")
	}
	if hookData.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want %q", hookData.SessionID, "abc123")
	}
}

func TestHookDataCodexCamelCaseJSON(t *testing.T) {
	jsonStr := `{"cwd":"/Users/test/project","transcriptPath":"/tmp/codex.jsonl","sessionId":"codex123","hookEventName":"Stop","toolName":"Shell"}`

	parsed, err := ParseHookData([]byte(jsonStr))
	if err != nil {
		t.Fatalf("ParseHookData failed: %v", err)
	}
	if parsed.TranscriptPath != "/tmp/codex.jsonl" {
		t.Errorf("TranscriptPath = %q, want /tmp/codex.jsonl", parsed.TranscriptPath)
	}
	if parsed.SessionID != "codex123" {
		t.Errorf("SessionID = %q, want codex123", parsed.SessionID)
	}
	if parsed.HookEventName != "Stop" {
		t.Errorf("HookEventName = %q, want Stop", parsed.HookEventName)
	}
	if parsed.ToolName != "Shell" {
		t.Errorf("ToolName = %q, want Shell", parsed.ToolName)
	}
}

// ParseHookDataTo is a test helper that uses ParseHookData
func ParseHookDataTo(hd *HookData, data []byte) error {
	parsed, err := ParseHookData(data)
	if err != nil {
		return err
	}
	*hd = parsed
	return nil
}

// TestTruncate tests the Truncate function
func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length unchanged",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long string truncated",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}
