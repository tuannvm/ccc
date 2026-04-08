package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/ledger"
)

// TestLedgerAppendAndRead tests basic ledger operations
func TestLedgerAppendAndRead(t *testing.T) {
	// Use a unique session name with temp suffix so the ledger file doesn't collide
	session := "test-ledger-" + filepath.Base(t.TempDir())
	// Clean up after test
	defer os.Remove(filepath.Join(config.CacheDir(), "ledger-"+session+".jsonl"))

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
	if err := ledger.AppendMessage(rec); err != nil {
		t.Fatalf("appendMessage failed: %v", err)
	}

	// Read back
	records := ledger.ReadLedger(session)
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
	if err := ledger.UpdateDelivery(session, "test:1", "terminal_delivered", true); err != nil {
		t.Fatalf("updateDelivery failed: %v", err)
	}

	// Read again — should be merged
	records = ledger.ReadLedger(session)
	if len(records) != 1 {
		t.Fatalf("readLedger returned %d records after update, want 1", len(records))
	}
	if !records[0].TerminalDelivered {
		t.Error("TerminalDelivered should be true after update")
	}

	// Test isDelivered
	if !ledger.IsDelivered(session, "test:1", "terminal") {
		t.Error("ledger.IsDelivered(terminal) should be true")
	}
	if !ledger.IsDelivered(session, "test:1", "telegram") {
		t.Error("ledger.IsDelivered(telegram) should be true")
	}

	// Test findUndelivered
	ledger.AppendMessage(&MessageRecord{
		ID:                "test:2",
		Session:           session,
		Type:              "assistant_text",
		Text:              "response",
		Origin:            "claude",
		TerminalDelivered: true,
		TelegramDelivered: false,
	})

	undelivered := ledger.FindUndelivered(session, "telegram")
	if len(undelivered) != 1 {
		t.Fatalf("ledger.FindUndelivered(telegram) returned %d, want 1", len(undelivered))
	}
	if undelivered[0].ID != "test:2" {
		t.Errorf("undelivered ID = %q, want test:2", undelivered[0].ID)
	}
}

// TestLedgerDedup tests that contentHash produces consistent hashes
func TestLedgerDedup(t *testing.T) {
	h1 := ledger.ContentHash("hello world")
	h2 := ledger.ContentHash("hello world")
	h3 := ledger.ContentHash("different text")

	if h1 != h2 {
		t.Errorf("same content produced different hashes: %s vs %s", h1, h2)
	}
	if h1 == h3 {
		t.Error("different content produced same hash")
	}
}
