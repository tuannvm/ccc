package main

import (
	"path/filepath"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/ledger"
)

// ledgerPath returns the path to a session's ledger file
func ledgerPath(session string) string {
	return filepath.Join(config.CacheDir(), "ledger-"+session+".jsonl")
}

// appendMessage writes a new message record to the session's ledger
func appendMessage(rec *MessageRecord) error {
	return ledger.AppendMessage(rec)
}

// updateDelivery appends an update record to the ledger
func updateDelivery(session, msgID, field string, value any) error {
	return ledger.UpdateDelivery(session, msgID, field, value)
}

// readLedger reads all records from a session's ledger and merges updates
func readLedger(session string) []*MessageRecord {
	return ledger.ReadLedger(session)
}

// isDelivered checks if a message ID has already been delivered to the given target
func isDelivered(session, msgID, target string) bool {
	return ledger.IsDelivered(session, msgID, target)
}

// findUndelivered returns messages not yet delivered to the given target ("telegram" or "terminal")
func findUndelivered(session, target string) []*MessageRecord {
	return ledger.FindUndelivered(session, target)
}

// contentHash returns a short hash of content for dedup IDs
func contentHash(s string) string {
	return ledger.ContentHash(s)
}
