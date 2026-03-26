package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestTmuxSafeName tests the tmuxSafeName function
func TestTmuxSafeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple name", "myproject", "myproject"},
		{"with dash", "my-project", "my-project"},
		{"with dot", "my.project", "my__project"},
		{"empty", "", ""},
		{"with spaces", "my project", "my project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tmuxSafeName(tt.input)
			if result != tt.expected {
				t.Errorf("tmuxSafeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGetSessionByTopic tests the getSessionByTopic function
func TestGetSessionByTopic(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"project1":   {TopicID: 100, Path: "/home/user/project1"},
			"project2":   {TopicID: 200, Path: "/home/user/project2"},
			"money/shop": {TopicID: 300, Path: "/home/user/money/shop"},
		},
	}

	tests := []struct {
		name     string
		topicID  int64
		expected string
	}{
		{"existing topic", 100, "project1"},
		{"another existing", 200, "project2"},
		{"nested path", 300, "money/shop"},
		{"non-existent", 999, ""},
		{"zero", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSessionByTopic(config, tt.topicID)
			if result != tt.expected {
				t.Errorf("getSessionByTopic(config, %d) = %q, want %q", tt.topicID, result, tt.expected)
			}
		})
	}
}

// TestGetSessionByTopicNilSessions tests with nil sessions map
func TestGetSessionByTopicNilSessions(t *testing.T) {
	config := &Config{
		Sessions: nil,
	}
	result := getSessionByTopic(config, 100)
	if result != "" {
		t.Errorf("getSessionByTopic with nil sessions = %q, want empty string", result)
	}
}

// TestConfigSaveLoad tests saving and loading config
func TestConfigSaveLoad(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override config path for test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test config
	config := &Config{
		BotToken: "test-token-123",
		ChatID:   12345,
		GroupID:  -67890,
		Sessions: map[string]*SessionInfo{
			"project1":   {TopicID: 100, Path: "/home/user/project1"},
			"money/shop": {TopicID: 200, Path: "/home/user/money/shop"},
		},
		Away: true,
	}

	// Save config
	if err := saveConfig(config); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load config
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Verify loaded config matches
	if loaded.BotToken != config.BotToken {
		t.Errorf("BotToken = %q, want %q", loaded.BotToken, config.BotToken)
	}
	if loaded.ChatID != config.ChatID {
		t.Errorf("ChatID = %d, want %d", loaded.ChatID, config.ChatID)
	}
	if loaded.GroupID != config.GroupID {
		t.Errorf("GroupID = %d, want %d", loaded.GroupID, config.GroupID)
	}
	if loaded.Away != config.Away {
		t.Errorf("Away = %v, want %v", loaded.Away, config.Away)
	}
	if len(loaded.Sessions) != len(config.Sessions) {
		t.Errorf("Sessions length = %d, want %d", len(loaded.Sessions), len(config.Sessions))
	}
	for name, info := range config.Sessions {
		loadedInfo := loaded.Sessions[name]
		if loadedInfo == nil || loadedInfo.TopicID != info.TopicID {
			t.Errorf("Sessions[%q].TopicID mismatch", name)
		}
	}
}

// TestConfigLoadNonExistent tests loading non-existent config
func TestConfigLoadNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	_, err = loadConfig()
	if err == nil {
		t.Error("loadConfig should fail for non-existent file")
	}
}

// TestConfigSessionsInitialized tests that Sessions map is initialized on load
func TestConfigSessionsInitialized(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Write config without sessions field
	configPath := filepath.Join(tmpDir, ".ccc.json")
	data := []byte(`{"bot_token": "test", "chat_id": 123}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if loaded.Sessions == nil {
		t.Error("Sessions should be initialized to non-nil map")
	}
}

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
			blocks := extractRecentAssistantTexts(filePath, tailCount)
			var result []string
			for _, b := range blocks {
				result = append(result, b.text)
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
	result := extractRecentAssistantTexts("/nonexistent/path/file.jsonl", 80)
	if result != nil {
		t.Errorf("non-existent file = %v, want nil", result)
	}
}

// TestExtractRecentEmptyPath tests with empty path
func TestExtractRecentEmptyPath(t *testing.T) {
	result := extractRecentAssistantTexts("", 80)
	if result != nil {
		t.Errorf("empty path = %v, want nil", result)
	}
}

// TestExecuteCommand tests the executeCommand function
func TestExecuteCommand(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		wantContain string
		wantErr     bool
	}{
		{"echo", "echo hello", "hello", false},
		{"pwd", "pwd", "/", false},
		{"invalid command", "nonexistentcommand123", "", true},
		{"exit code", "exit 1", "", true},
		{"stderr output", "echo error >&2", "error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("executeCommand(%q) error = %v, wantErr %v", tt.cmd, err, tt.wantErr)
			}
			if tt.wantContain != "" && !contains(output, tt.wantContain) {
				t.Errorf("executeCommand(%q) output = %q, want to contain %q", tt.cmd, output, tt.wantContain)
			}
		})
	}
}

// TestConfigJSON tests JSON marshaling/unmarshaling
func TestConfigJSON(t *testing.T) {
	config := &Config{
		BotToken: "token123",
		ChatID:   12345,
		GroupID:  -67890,
		Sessions: map[string]*SessionInfo{
			"test": {TopicID: 100, Path: "/home/user/test"},
		},
		Away: true,
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if loaded.BotToken != config.BotToken {
		t.Errorf("BotToken mismatch")
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

// TestWindowNameFromTarget tests extracting window name from tmux target
func TestWindowNameFromTarget(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		expected string
	}{
		{"session:window", "ccc:myproject", "myproject"},
		{"no colon", "myproject", "myproject"},
		{"multiple colons", "sess:win:extra", "extra"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := windowNameFromTarget(tt.target)
			if result != tt.expected {
				t.Errorf("windowNameFromTarget(%q) = %q, want %q", tt.target, result, tt.expected)
			}
		})
	}
}

// TestConfigFilePermissions tests that config is saved with correct permissions
func TestConfigFilePermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		BotToken: "secret-token",
		ChatID:   12345,
		Sessions: make(map[string]*SessionInfo),
	}

	if err := saveConfig(config); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}

	// Check permissions are 0600 (owner read/write only)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Config file permissions = %o, want 0600", perm)
	}
}

// TestEmptySessionsMap tests behavior with empty sessions
func TestEmptySessionsMap(t *testing.T) {
	config := &Config{
		Sessions: make(map[string]*SessionInfo),
	}

	result := getSessionByTopic(config, 100)
	if result != "" {
		t.Errorf("getSessionByTopic with empty sessions = %q, want empty", result)
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

// TestLedgerAppendAndRead tests basic ledger operations
func TestLedgerAppendAndRead(t *testing.T) {
	// Use a unique session name with temp suffix so the ledger file doesn't collide
	session := "test-ledger-" + filepath.Base(t.TempDir())
	// Clean up after test
	defer os.Remove(ledgerPath(session))

	// Append a message
	rec := &MessageRecord{
		ID:                "test:1",
		Session:           session,
		Type:              "user_prompt",
		Text:              "hello world",
		Origin:            "telegram",
		TerminalDelivered: false,
		TelegramDelivered: true,
	}
	if err := appendMessage(rec); err != nil {
		t.Fatalf("appendMessage failed: %v", err)
	}

	// Read back
	records := readLedger(session)
	if len(records) != 1 {
		t.Fatalf("readLedger returned %d records, want 1", len(records))
	}
	if records[0].ID != "test:1" {
		t.Errorf("ID = %q, want test:1", records[0].ID)
	}
	if records[0].TerminalDelivered {
		t.Error("TerminalDelivered should be false")
	}

	// Update delivery
	if err := updateDelivery(session, "test:1", "terminal_delivered", true); err != nil {
		t.Fatalf("updateDelivery failed: %v", err)
	}

	// Read again — should be merged
	records = readLedger(session)
	if len(records) != 1 {
		t.Fatalf("readLedger returned %d records after update, want 1", len(records))
	}
	if !records[0].TerminalDelivered {
		t.Error("TerminalDelivered should be true after update")
	}

	// Test isDelivered
	if !isDelivered(session, "test:1", "terminal") {
		t.Error("isDelivered(terminal) should be true")
	}
	if !isDelivered(session, "test:1", "telegram") {
		t.Error("isDelivered(telegram) should be true")
	}

	// Test findUndelivered
	appendMessage(&MessageRecord{
		ID:                "test:2",
		Session:           session,
		Type:              "assistant_text",
		Text:              "response",
		Origin:            "claude",
		TerminalDelivered: true,
		TelegramDelivered: false,
	})

	undelivered := findUndelivered(session, "telegram")
	if len(undelivered) != 1 {
		t.Fatalf("findUndelivered(telegram) returned %d, want 1", len(undelivered))
	}
	if undelivered[0].ID != "test:2" {
		t.Errorf("undelivered ID = %q, want test:2", undelivered[0].ID)
	}
}

// TestLedgerDedup tests that contentHash produces consistent hashes
func TestLedgerDedup(t *testing.T) {
	h1 := contentHash("hello world")
	h2 := contentHash("hello world")
	h3 := contentHash("different text")

	if h1 != h2 {
		t.Errorf("same content produced different hashes: %s vs %s", h1, h2)
	}
	if h1 == h3 {
		t.Error("different content produced same hash")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestAtomicSaveConfigConcurrent tests that concurrent writes don't corrupt config
func TestAtomicSaveConfigConcurrent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-concurrent-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")

	// Create initial config
	config := &Config{
		BotToken: "test-token",
		ChatID:   12345,
		Sessions: make(map[string]*SessionInfo),
	}
	if err := saveConfig(config); err != nil {
		t.Fatalf("Initial save failed: %v", err)
	}

	// Spawn 100 goroutines, each writing different data
	const numGoroutines = 100
	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(sessionNum int) {
			defer wg.Done()
			cfg := &Config{
				BotToken: "test-token",
				ChatID:   12345,
				Sessions: map[string]*SessionInfo{
					fmt.Sprintf("session-%d", sessionNum): {
						TopicID: int64(sessionNum),
						Path:    fmt.Sprintf("/path/session-%d", sessionNum),
					},
				},
			}
			if err := saveConfig(cfg); err != nil {
				errCh <- fmt.Errorf("goroutine %d: %w", sessionNum, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		t.Fatalf("Concurrent writes had %d errors: %v", len(errors), errors[0])
	}

	// Verify config is valid JSON
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read final config: %v", err)
	}

	var finalConfig Config
	if err := json.Unmarshal(data, &finalConfig); err != nil {
		t.Fatalf("Final config is not valid JSON: %v", err)
	}

	// All sessions should be present (or one of the last writes won)
	if finalConfig.Sessions == nil {
		t.Error("Sessions map is nil after concurrent writes")
	}

	// File permissions should be 0600
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("Config permissions = %o, want 0600", perm)
	}

	// Verify no temp files left
	tmpFiles, _ := filepath.Glob(filepath.Join(tmpDir, ".config", "ccc", "config-*.tmp"))
	if len(tmpFiles) > 0 {
		t.Errorf("Found %d temp files left behind: %v", len(tmpFiles), tmpFiles)
	}
}

// TestAtomicSaveConfigPreservesOriginal tests that original config is preserved on failure
func TestAtomicSaveConfigPreservesOriginal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-preserve-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")

	// Write initial valid config
	initialConfig := &Config{
		BotToken: "original-token",
		ChatID:   11111,
		GroupID:  22222,
		Sessions: map[string]*SessionInfo{
			"original": {TopicID: 100, Path: "/original/path"},
		},
	}
	if err := saveConfig(initialConfig); err != nil {
		t.Fatalf("Initial save failed: %v", err)
	}

	// Read the initial content for comparison
	originalData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read initial config: %v", err)
	}

	// Make config directory read-only to simulate write failure
	configDir := filepath.Dir(configPath)
	if err := os.Chmod(configDir, 0555); err != nil {
		t.Fatalf("Failed to make config dir read-only: %v", err)
	}
	defer os.Chmod(configDir, 0755) // Restore permissions

	// Try to save (should fail)
	newConfig := &Config{
		BotToken: "new-token",
		ChatID:   99999,
	}
	if err := saveConfig(newConfig); err == nil {
		t.Error("Expected save to fail on read-only directory, but it succeeded")
	}

	// Verify original config is unchanged
	currentData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config after failed save: %v", err)
	}

	if string(currentData) != string(originalData) {
		t.Errorf("Original config was modified!\nExpected: %s\nGot: %s", string(originalData), string(currentData))
	}

	// Verify it's still valid JSON
	var config Config
	if err := json.Unmarshal(currentData, &config); err != nil {
		t.Fatalf("Original config is no longer valid JSON: %v", err)
	}

	if config.BotToken != "original-token" {
		t.Errorf("BotToken changed from 'original-token' to '%s'", config.BotToken)
	}
	if config.ChatID != 11111 {
		t.Errorf("ChatID changed from 11111 to %d", config.ChatID)
	}
}

// TestAtomicSaveConfigPermissions verifies config file has correct permissions
func TestAtomicSaveConfigPermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-perms-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		BotToken: "secret-token-12345",
		ChatID:   12345,
		Sessions: map[string]*SessionInfo{
			"test": {TopicID: 100, Path: "/test/path"},
		},
	}

	if err := saveConfig(config); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Config permissions = %o, want 0600 (owner read/write only)", perm)
	}
}

// TestAtomicSaveConfigTempCleanup verifies temp files are cleaned up on error
func TestAtomicSaveConfigTempCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-cleanup-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")

	// Write initial config
	initialConfig := &Config{BotToken: "initial"}
	if err := saveConfig(initialConfig); err != nil {
		t.Fatalf("Initial save failed: %v", err)
	}

	// Count temp files before
	configDir := filepath.Dir(configPath)
	beforeFiles, _ := os.ReadDir(configDir)

	// Make the parent directory read-only to prevent rename
	// This will cause os.Rename to fail
	if err := os.Chmod(configDir, 0555); err != nil {
		t.Fatalf("Failed to make config dir read-only: %v", err)
	}

	// Try to save (should fail during rename)
	newConfig := &Config{BotToken: "new-token"}
	err = saveConfig(newConfig)

	// Restore permissions before any assertions
	os.Chmod(configDir, 0755)

	if err == nil {
		t.Error("Expected save to fail on read-only directory, but it succeeded")
	}

	// Count temp files after
	afterFiles, _ := os.ReadDir(configDir)

	// Filter for .tmp files
	countTmp := func(files []os.DirEntry) int {
		count := 0
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".tmp") {
				count++
			}
		}
		return count
	}

	beforeTmp := countTmp(beforeFiles)
	afterTmp := countTmp(afterFiles)

	if afterTmp > beforeTmp {
		t.Errorf("Temp files leaked: before=%d, after=%d", beforeTmp, afterTmp)
	}
}

// ============================================================================
// BASELINE TESTS - Testing existing behavior BEFORE refactoring
// These tests ensure refactoring doesn't break existing functionality
// ============================================================================

// TestBaselineConfigLoadSave tests that config load/save works correctly
func TestBaselineConfigLoadSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test 1: Create a config and save it
	config := &Config{
		BotToken:          "test-token-123",
		ChatID:            12345,
		GroupID:           67890,
		ProjectsDir:       "~/Projects",
		TranscriptionLang: "en",
		RelayURL:          "https://relay.example.com",
		Away:              true,
		OAuthToken:        "oauth-token",
		OTPSecret:         "otp-secret",
		ActiveProvider:    "test-provider",
		Providers: map[string]*ProviderConfig{
			"test-provider": {
				AuthToken:  "provider-token",
				AuthEnvVar: "TEST_API_KEY",
				BaseURL:     "https://api.example.com",
				ApiTimeout: 30000,
				OpusModel:   "claude-3-opus-20250114",
				SonnetModel: "claude-3-5-20241022",
			},
		},
		Sessions: map[string]*SessionInfo{
			"test-project": {
				TopicID:      1001,
				Path:         "/home/user/test-project",
				ProviderName: "test-provider",
			},
		},
	}

	// Save config
	if err := saveConfig(config); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	// Load config back
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Verify all fields match
	if loaded.BotToken != config.BotToken {
		t.Errorf("BotToken: got %q, want %q", loaded.BotToken, config.BotToken)
	}
	if loaded.ChatID != config.ChatID {
		t.Errorf("ChatID: got %d, want %d", loaded.ChatID, config.ChatID)
	}
	if loaded.GroupID != config.GroupID {
		t.Errorf("GroupID: got %d, want %d", loaded.GroupID, config.GroupID)
	}
	if loaded.ProjectsDir != config.ProjectsDir {
		t.Errorf("ProjectsDir: got %q, want %q", loaded.ProjectsDir, config.ProjectsDir)
	}
	if loaded.TranscriptionLang != config.TranscriptionLang {
		t.Errorf("TranscriptionLang: got %q, want %q", loaded.TranscriptionLang, config.TranscriptionLang)
	}
	if loaded.RelayURL != config.RelayURL {
		t.Errorf("RelayURL: got %q, want %q", loaded.RelayURL, config.RelayURL)
	}
	if loaded.Away != config.Away {
		t.Errorf("Away: got %v, want %v", loaded.Away, config.Away)
	}
	if loaded.OAuthToken != config.OAuthToken {
		t.Errorf("OAuthToken: got %q, want %q", loaded.OAuthToken, config.OAuthToken)
	}
	if loaded.OTPSecret != config.OTPSecret {
		t.Errorf("OTPSecret: got %q, want %q", loaded.OTPSecret, config.OTPSecret)
	}
	if loaded.ActiveProvider != config.ActiveProvider {
		t.Errorf("ActiveProvider: got %q, want %q", loaded.ActiveProvider, config.ActiveProvider)
	}

	// Verify providers
	if len(loaded.Providers) != 1 {
		t.Errorf("Providers count: got %d, want 1", len(loaded.Providers))
	}
	if p, ok := loaded.Providers["test-provider"]; !ok {
		t.Error("test-provider not found in loaded config")
	} else {
		if p.AuthToken != config.Providers["test-provider"].AuthToken {
			t.Errorf("Provider AuthToken mismatch")
		}
		if p.ApiTimeout != config.Providers["test-provider"].ApiTimeout {
			t.Errorf("Provider ApiTimeout mismatch")
		}
	}

	// Verify sessions
	if len(loaded.Sessions) != 1 {
		t.Errorf("Sessions count: got %d, want 1", len(loaded.Sessions))
	}
	if s, ok := loaded.Sessions["test-project"]; !ok {
		t.Error("test-project session not found")
	} else {
		if s.TopicID != 1001 {
			t.Errorf("Session TopicID: got %d, want 1001", s.TopicID)
		}
	}
}

// TestBaselineProviderResolution tests provider lookup functions
func TestBaselineProviderResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		ActiveProvider: "custom-provider",
		Providers: map[string]*ProviderConfig{
			"anthropic": {
				BaseURL:     "https://api.anthropic.com",
				AuthEnvVar:  "ANTHROPIC_API_KEY",
			},
			"custom-provider": {
				BaseURL:     "https://custom.ai",
				AuthToken:  "custom-key",
				SonnetModel: "custom-sonnet",
			},
		},
	}

	// Test getActiveProvider - it returns *ProviderConfig, not Provider interface
	active := getActiveProvider(config)
	if active == nil {
		t.Error("getActiveProvider returned nil")
	} else if active.BaseURL != "https://custom.ai" {
		t.Errorf("getActiveProvider().BaseURL: got %q, want 'https://custom.ai'", active.BaseURL)
	}

	// Test getProvider with specific name
	provider := getProvider(config, "anthropic")
	if provider == nil {
		t.Error("getProvider('anthropic') returned nil")
	} else if provider.Name() != "anthropic" {
		t.Errorf("getProvider('anthropic').Name(): got %q, want 'anthropic'", provider.Name())
	}

	// Test getProvider with empty string (should return active)
	provider = getProvider(config, "")
	if provider == nil {
		t.Error("getProvider('') returned nil")
	} else if provider.Name() != "custom-provider" {
		t.Errorf("getProvider('').Name(): got %q, want 'custom-provider'", provider.Name())
	}

	// Test getProviderNames
	names := getProviderNames(config)
	if len(names) != 2 {
		t.Errorf("getProviderNames length: got %d, want 2", len(names))
	}
	// Check anthropic is always included
	hasAnthropic := false
	for _, name := range names {
		if name == "anthropic" {
			hasAnthropic = true
			break
		}
	}
	if !hasAnthropic {
		t.Error("'anthropic' not in provider names (should always be included)")
	}
}

// TestBaselineSessionLookup tests session lookup functions
func TestBaselineSessionLookup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		GroupID: 12345,
		Sessions: map[string]*SessionInfo{
			"project1": {
				TopicID:         1001,
				Path:            "/home/user/project1",
				ClaudeSessionID: "claude-session-1",
				ProviderName:    "provider1",
			},
			"project2": {
				TopicID:         1002,
				Path:            "/home/user/project2",
				ClaudeSessionID: "claude-session-2",
			},
		},
	}

	// Test getSessionByTopic
	sessName := getSessionByTopic(config, 1001)
	if sessName != "project1" {
		t.Errorf("getSessionByTopic(1001): got %q, want 'project1'", sessName)
	}

	sessName = getSessionByTopic(config, 9999)
	if sessName != "" {
		t.Errorf("getSessionByTopic(9999): got %q, want empty", sessName)
	}

	// Test findSessionByClaudeID
	sessName, topicID := findSessionByClaudeID(config, "claude-session-1")
	if sessName != "project1" {
		t.Errorf("findSessionByClaudeID('claude-session-1'): got %q, want 'project1'", sessName)
	}
	if topicID != 1001 {
		t.Errorf("findSessionByClaudeID topicID: got %d, want 1001", topicID)
	}

	// Test findSessionByCwd
	sessName, topicID = findSessionByCwd(config, "/home/user/project1")
	if sessName != "project1" {
		t.Errorf("findSessionByCwd('/home/user/project1'): got %q, want 'project1'", sessName)
	}

	// Test non-existent path
	sessName, topicID = findSessionByCwd(config, "/nonexistent")
	if sessName != "" {
		t.Errorf("findSessionByCwd('/nonexistent'): got %q, want empty", sessName)
	}

	// Test findSession (combined lookup)
	sessName, topicID = findSession(config, "/home/user/project1", "")
	if sessName != "project1" {
		t.Errorf("findSession with claudeID='': got %q, want 'project1'", sessName)
	}

	sessName, topicID = findSession(config, "", "claude-session-2")
	if sessName != "project2" {
		t.Errorf("findSession with cwd='': got %q, want 'project2'", sessName)
	}
}

// TestBaselinePathUtilities tests path resolution functions
func TestBaselinePathUtilities(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		ProjectsDir: "~/CustomProjects",
	}

	// Test getProjectsDir
	projectsDir := getProjectsDir(config)
	expected := filepath.Join(tmpDir, "CustomProjects")
	if projectsDir != expected {
		t.Errorf("getProjectsDir: got %q, want %q", projectsDir, expected)
	}

	// Test resolveProjectPath with absolute path
	path := resolveProjectPath(config, "/absolute/path")
	if path != "/absolute/path" {
		t.Errorf("resolveProjectPath(absolute): got %q, want '/absolute/path'", path)
	}

	// Test resolveProjectPath with home-relative path
	path = resolveProjectPath(config, "~/from/home")
	expected = filepath.Join(tmpDir, "from/home")
	if path != expected {
		t.Errorf("resolveProjectPath(~/from/home): got %q, want %q", path, expected)
	}

	// Test resolveProjectPath with relative path
	path = resolveProjectPath(config, "relative/path")
	expected = filepath.Join(tmpDir, "CustomProjects", "relative/path")
	if path != expected {
		t.Errorf("resolveProjectPath(relative): got %q, want %q", path, expected)
	}

	// Test expandPath
	path = expandPath("~/test/path")
	expected = filepath.Join(tmpDir, "test/path")
	if path != expected {
		t.Errorf("expandPath(~/test/path): got %q, want %q", path, expected)
	}

	// Test expandPath with non-tilde path
	path = expandPath("/absolute/path")
	if path != "/absolute/path" {
		t.Errorf("expandPath(absolute): got %q, want '/absolute/path'", path)
	}
}

// TestCWDFallbackPathPrefixCollision tests that CWD matching doesn't match
// /repo to /repo-copy or /repo2 (path boundary protection)
func TestCWDFallbackPathPrefixCollision(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"repo": {
				Path:    "/home/user/repo",
				TopicID: 100,
			},
			"repo-copy": {
				Path:    "/home/user/repo-copy",
				TopicID: 200,
			},
		},
	}

	// Test exact match to /repo
	sessName, topicID := findSessionByCwd(config, "/home/user/repo")
	if sessName != "repo" {
		t.Errorf("CWD('/home/user/repo'): got %q, want 'repo'", sessName)
	}
	if topicID != 100 {
		t.Errorf("topicID: got %d, want 100", topicID)
	}

	// Test exact match to /repo-copy
	sessName, topicID = findSessionByCwd(config, "/home/user/repo-copy")
	if sessName != "repo-copy" {
		t.Errorf("CWD('/home/user/repo-copy'): got %q, want 'repo-copy'", sessName)
	}
	if topicID != 200 {
		t.Errorf("topicID: got %d, want 200", topicID)
	}

	// Test subdirectory of /repo (should match repo, not repo-copy)
	sessName, topicID = findSessionByCwd(config, "/home/user/repo/subdir")
	if sessName != "repo" {
		t.Errorf("CWD('/home/user/repo/subdir'): got %q, want 'repo'", sessName)
	}
	if topicID != 100 {
		t.Errorf("topicID: got %d, want 100", topicID)
	}

	// Test subdirectory of /repo-copy (should match repo-copy)
	sessName, topicID = findSessionByCwd(config, "/home/user/repo-copy/subdir")
	if sessName != "repo-copy" {
		t.Errorf("CWD('/home/user/repo-copy/subdir'): got %q, want 'repo-copy'", sessName)
	}
	if topicID != 200 {
		t.Errorf("topicID: got %d, want 200", topicID)
	}
}

// TestCWDFallbackZeroTopicID documents behavior with zero-topic sessions
// Note: findSessionByCwd() matches zero-topic sessions, but the Stop hook
// logic filters them out (see hooks.go lines 354-358)
func TestCWDFallbackZeroTopicID(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"private-session": {
				Path:    "/home/user/private",
				TopicID: 0, // Private chat, no topic
			},
			"topic-session": {
				Path:    "/home/user/topic",
				TopicID: 100,
			},
		},
	}

	// findSessionByCwd() DOES match zero-topic sessions (returns them)
	// The Stop hook logic is responsible for filtering these out
	sessName, topicID := findSessionByCwd(config, "/home/user/private")
	if sessName != "private-session" {
		t.Errorf("CWD('/home/user/private') with TopicID=0: got %q, want 'private-session' (findSessionByCwd matches all)", sessName)
	}
	if topicID != 0 {
		t.Errorf("topicID: got %d, want 0", topicID)
	}

	// Normal topic session should match
	sessName, topicID = findSessionByCwd(config, "/home/user/topic")
	if sessName != "topic-session" {
		t.Errorf("CWD('/home/user/topic'): got %q, want 'topic-session'", sessName)
	}
	if topicID != 100 {
		t.Errorf("topicID: got %d, want 100", topicID)
	}

	// Document: Stop hook logic filters TopicID == 0 before using CWD fallback
	// See hooks.go: "if info.TopicID == 0 { continue }"
}

// TestCWDFallbackTeamSessions tests team session CWD matching and tie-breaking
func TestCWDFallbackTeamSessions(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"regular": {
				Path:    "/home/user/project",
				TopicID: 100,
			},
		},
		TeamSessions: map[int64]*SessionInfo{
			200: {
				SessionName: "team-session",
				Path:        "/home/user/project",
			},
			300: {
				SessionName: "other-team",
				Path:        "/home/user/other-project",
			},
		},
	}

	// When path lengths are equal, regular session should win (tie-breaking)
	sessName, topicID := findSessionByCwd(config, "/home/user/project")
	if sessName != "regular" {
		t.Errorf("CWD('/home/user/project') tie-break: got %q, want 'regular'", sessName)
	}
	if topicID != 100 {
		t.Errorf("topicID: got %d, want 100", topicID)
	}

	// Team session with longer path should win
	sessName, topicID = findSessionByCwd(config, "/home/user/other-project/subdir")
	if sessName != "other-team" {
		t.Errorf("CWD('/home/user/other-project/subdir'): got %q, want 'other-team'", sessName)
	}
	if topicID != 300 {
		t.Errorf("topicID: got %d, want 300", topicID)
	}
}

// TestCWDFallbackNilSessions tests that nil session entries don't cause panics
func TestCWDFallbackNilSessions(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"valid": {
				Path:    "/home/user/valid",
				TopicID: 100,
			},
			"nil-entry": nil, // Simulates a corrupted config
		},
	}

	// Should not panic and should find the valid session
	sessName, topicID := findSessionByCwd(config, "/home/user/valid")
	if sessName != "valid" {
		t.Errorf("CWD with nil entry in map: got %q, want 'valid'", sessName)
	}
	if topicID != 100 {
		t.Errorf("topicID: got %d, want 100", topicID)
	}

	// Non-existent path should return empty
	sessName, topicID = findSessionByCwd(config, "/nonexistent")
	if sessName != "" {
		t.Errorf("CWD('/nonexistent'): got %q, want empty", sessName)
	}
}
