package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tuannvm/ccc/pkg/hooks"
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
			blocks := hooks.ExtractRecentAssistantTexts(filePath, tailCount)
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
	result := hooks.ExtractRecentAssistantTexts("/nonexistent/path/file.jsonl", 80)
	if result != nil {
		t.Errorf("non-existent file = %v, want nil", result)
	}
}

// TestExtractRecentEmptyPath tests with empty path
func TestExtractRecentEmptyPath(t *testing.T) {
	result := hooks.ExtractRecentAssistantTexts("", 80)
	if result != nil {
		t.Errorf("empty path = %v, want nil", result)
	}
}

// TestHookDataJSON tests HookData JSON parsing
func TestHookDataJSON(t *testing.T) {
	jsonStr := `{"cwd":"/Users/test/project","transcript_path":"/tmp/transcript.jsonl","session_id":"abc123"}`

	var hookData HookData
	if err := json.Unmarshal([]byte(jsonStr), &hookData); err != nil {
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

// TestTelegramMessageJSON tests TelegramMessage JSON parsing
func TestTelegramMessageJSON(t *testing.T) {
	jsonStr := `{
		"message_id": 123,
		"message_thread_id": 456,
		"chat": {"id": 789, "type": "supergroup"},
		"from": {"id": 111, "username": "testuser"},
		"text": "Hello world"
	}`

	var msg TelegramMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if msg.MessageID != 123 {
		t.Errorf("MessageID = %d, want 123", msg.MessageID)
	}
	if msg.MessageThreadID != 456 {
		t.Errorf("MessageThreadID = %d, want 456", msg.MessageThreadID)
	}
	if msg.Chat.ID != 789 {
		t.Errorf("Chat.ID = %d, want 789", msg.Chat.ID)
	}
	if msg.Chat.Type != "supergroup" {
		t.Errorf("Chat.Type = %q, want supergroup", msg.Chat.Type)
	}
	if msg.From.Username != "testuser" {
		t.Errorf("From.Username = %q, want testuser", msg.From.Username)
	}
	if msg.Text != "Hello world" {
		t.Errorf("Text = %q, want 'Hello world'", msg.Text)
	}
}

// TestTopicResultJSON tests TopicResult JSON parsing
func TestTopicResultJSON(t *testing.T) {
	jsonStr := `{"message_thread_id": 12345, "name": "test-topic"}`

	var topic TopicResult
	if err := json.Unmarshal([]byte(jsonStr), &topic); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if topic.MessageThreadID != 12345 {
		t.Errorf("MessageThreadID = %d, want 12345", topic.MessageThreadID)
	}
	if topic.Name != "test-topic" {
		t.Errorf("Name = %q, want test-topic", topic.Name)
	}
}

// TestTelegramResponseJSON tests TelegramResponse JSON parsing
func TestTelegramResponseJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantOK  bool
		wantErr string
	}{
		{
			name:   "success response",
			json:   `{"ok": true, "result": {}}`,
			wantOK: true,
		},
		{
			name:    "error response",
			json:    `{"ok": false, "description": "Bad Request"}`,
			wantOK:  false,
			wantErr: "Bad Request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp TelegramResponse
			if err := json.Unmarshal([]byte(tt.json), &resp); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if resp.OK != tt.wantOK {
				t.Errorf("OK = %v, want %v", resp.OK, tt.wantOK)
			}
			if resp.Description != tt.wantErr {
				t.Errorf("Description = %q, want %q", resp.Description, tt.wantErr)
			}
		})
	}
}

// TestReplyToMessage tests nested message parsing
func TestReplyToMessage(t *testing.T) {
	jsonStr := `{
		"message_id": 100,
		"text": "Reply text",
		"chat": {"id": 123, "type": "private"},
		"from": {"id": 456, "username": "user"},
		"reply_to_message": {
			"message_id": 99,
			"text": "Original text",
			"chat": {"id": 123, "type": "private"},
			"from": {"id": 456, "username": "user"}
		}
	}`

	var msg TelegramMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if msg.ReplyToMessage == nil {
		t.Fatal("ReplyToMessage should not be nil")
	}
	if msg.ReplyToMessage.MessageID != 99 {
		t.Errorf("ReplyToMessage.MessageID = %d, want 99", msg.ReplyToMessage.MessageID)
	}
	if msg.ReplyToMessage.Text != "Original text" {
		t.Errorf("ReplyToMessage.Text = %q, want 'Original text'", msg.ReplyToMessage.Text)
	}
}

// TestMessageTruncation tests that long messages are truncated
func TestMessageTruncation(t *testing.T) {
	// The sendMessage function truncates at 4000 chars
	// We test the truncation logic directly
	const maxLen = 4000

	tests := []struct {
		name       string
		inputLen   int
		shouldTrim bool
	}{
		{"short message", 100, false},
		{"exactly max", maxLen, false},
		{"over max", maxLen + 100, true},
		{"way over max", 10000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create message of specified length
			text := make([]byte, tt.inputLen)
			for i := range text {
				text[i] = 'a'
			}
			msg := string(text)

			// Apply same truncation logic as sendMessage
			if len(msg) > maxLen {
				msg = msg[:maxLen] + "\n... (truncated)"
			}

			if tt.shouldTrim {
				if len(msg) <= tt.inputLen {
					// Should have been truncated
					if len(msg) != maxLen+len("\n... (truncated)") {
						t.Errorf("truncated length = %d, want %d", len(msg), maxLen+len("\n... (truncated)"))
					}
				}
			} else {
				if len(msg) != tt.inputLen {
					t.Errorf("message was unexpectedly modified")
				}
			}
		})
	}
}

// TestDetectConsentDialog tests the consent dialog pattern detection
func TestDetectConsentDialog(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "workspace trust dialog with numbered options",
			content:  "Do you trust the files in this folder?\n\n1. Yes, I trust this folder\n2. No, exit",
			expected: true,
		},
		{
			name:     "consent dialog with bracket format",
			content:  "Safety check required\n[1] Allow access\n[2] Decline and exit",
			expected: true,
		},
		{
			name:     "consent dialog with close paren format",
			content:  "Confirm you want to proceed\n1) Continue\n2) Cancel",
			expected: true,
		},
		{
			name:     "mixed format options",
			content:  "Allow this project?\n1. Yes\n2) Cancel",
			expected: true,
		},
		{
			name:     "dialog with trust keyword and exit option",
			content:  "Do you trust this workspace you created?\n1. Trust\n2. Exit",
			expected: true,
		},
		{
			name:     "no numbered options - not a dialog",
			content:  "Do you trust this folder?\nYes or No?",
			expected: false,
		},
		{
			name:     "no trust keywords - not a consent dialog",
			content:  "Choose an option:\n1. Option A\n2. Option B",
			expected: false,
		},
		{
			name:     "no exit keywords - not a consent dialog",
			content:  "Do you trust this folder?\n1. Yes, I trust\n2. Also trust",
			expected: false,
		},
		{
			name:     "active Claude session with prompt - not dialog",
			content:  "How can I help?\n❯ ",
			expected: false,
		},
		{
			name:     "Claude tool use output - not dialog",
			content:  "Bash: running command\nresult: success\n❯ ",
			expected: false,
		},
		{
			name:     "dialog without Claude context but no prompt",
			content:  "Do you trust this folder?\n1. Yes\n2. Exit",
			expected: true,
		},
		{
			name:     "single option - not enough to be dialog",
			content:  "Do you trust this folder?\n1. Yes",
			expected: false,
		},
		{
			name:     "safety check with decline option",
			content:  "Safety check: confirm you want to proceed\n1. Allow\n2. Decline",
			expected: true,
		},
		{
			name:     "project you created phrase with exit",
			content:  "Is this a project you created?\n1. Yes\n2. Exit",
			expected: true,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "only Claude context",
			content:  "How can I help you today?\nI'm ready to assist.",
			expected: false,
		},
		{
			name:     "numbered options with allow keyword",
			content:  "Allow this application?\n1. Allow\n2. Skip",
			expected: true,
		},
		{
			name:     "dialog with proceed keyword",
			content:  "Safety check\n1. Proceed\n2. Cancel",
			expected: true,
		},
		{
			name:     "dialog with confirm keyword",
			content:  "Confirm access\n1. Confirm\n2. Deny",
			expected: true,
		},
		{
			name:     "dialog with you trust phrase and exit",
			content:  "Are you sure you trust?\n1. Yes\n2. Decline",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectConsentDialog(tt.content)
			if result != tt.expected {
				t.Errorf("detectConsentDialog() = %v, want %v\nContent: %q", result, tt.expected, tt.content)
			}
		})
	}
}
