package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
