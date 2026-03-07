package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kidandcat/ccc/internal/config"
)

// TestGetSessionByTopic tests the GetSessionByTopic function
func TestGetSessionByTopic(t *testing.T) {
	config := &config.Config{
		Sessions: map[string]*config.SessionInfo{
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
			result := GetSessionByTopic(config, tt.topicID)
			if result != tt.expected {
				t.Errorf("GetSessionByTopic(config, %d) = %q, want %q", tt.topicID, result, tt.expected)
			}
		})
	}
}

// TestGetSessionByTopicNilSessions tests with nil sessions map
func TestGetSessionByTopicNilSessions(t *testing.T) {
	config := &config.Config{
		Sessions: nil,
	}
	result := GetSessionByTopic(config, 100)
	if result != "" {
		t.Errorf("GetSessionByTopic with nil sessions = %q, want empty string", result)
	}
}

// TestEmptySessionsMap tests behavior with empty sessions
func TestEmptySessionsMap(t *testing.T) {
	config := &config.Config{
		Sessions: make(map[string]*config.SessionInfo),
	}

	result := GetSessionByTopic(config, 100)
	if result != "" {
		t.Errorf("GetSessionByTopic with empty sessions = %q, want empty", result)
	}
}

// TestLedgerAppendAndRead tests basic ledger operations
func TestLedgerAppendAndRead(t *testing.T) {
	// Use a unique session name with temp suffix so the ledger file doesn't collide
	session := "test-ledger-" + filepath.Base(t.TempDir())
	// Clean up after test
	defer removeLedgerFile(session)

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
	if err := AppendMessage(rec); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
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
	if err := UpdateDelivery(session, "test:1", "terminal_delivered", true); err != nil {
		t.Fatalf("UpdateDelivery failed: %v", err)
	}

	// Read again — should be merged
	records = readLedger(session)
	if len(records) != 1 {
		t.Fatalf("readLedger returned %d records after update, want 1", len(records))
	}
	if !records[0].TerminalDelivered {
		t.Error("TerminalDelivered should be true after update")
	}

	// Test IsDelivered
	if !IsDelivered(session, "test:1", "terminal") {
		t.Error("IsDelivered(terminal) should be true")
	}
	if !IsDelivered(session, "test:1", "telegram") {
		t.Error("IsDelivered(telegram) should be true")
	}

	// Test FindUndelivered
	AppendMessage(&MessageRecord{
		ID:                "test:2",
		Session:           session,
		Type:              "assistant_text",
		Text:              "response",
		Origin:            "claude",
		TerminalDelivered: true,
		TelegramDelivered: false,
	})

	undelivered := FindUndelivered(session, "telegram")
	if len(undelivered) != 1 {
		t.Fatalf("FindUndelivered(telegram) returned %d, want 1", len(undelivered))
	}
	if undelivered[0].ID != "test:2" {
		t.Errorf("undelivered ID = %q, want test:2", undelivered[0].ID)
	}
}

// TestLedgerDedup tests that ContentHash produces consistent hashes
func TestLedgerDedup(t *testing.T) {
	h1 := ContentHash("hello world")
	h2 := ContentHash("hello world")
	h3 := ContentHash("different text")

	if h1 != h2 {
		t.Errorf("same content produced different hashes: %s vs %s", h1, h2)
	}
	if h1 == h3 {
		t.Error("different content produced same hash")
	}
}

// Helper function for tests
func removeLedgerFile(session string) {
	// Ledger files are stored in cache directory, clean up if needed
	// This is handled by t.TempDir() for test isolation
	os.RemoveAll(session)
}
