package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ledgerMu sync.Mutex

func ledgerPath(session string) string {
	return filepath.Join(cacheDir(), "ledger-"+session+".jsonl")
}

// appendMessage writes a new message record to the session's ledger
func appendMessage(rec *MessageRecord) error {
	if rec.Timestamp == 0 {
		rec.Timestamp = time.Now().Unix()
	}
	ledgerMu.Lock()
	defer ledgerMu.Unlock()

	f, err := os.OpenFile(ledgerPath(rec.Session), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// updateDelivery appends an update record to the ledger
func updateDelivery(session, msgID, field string, value any) error {
	rec := &MessageRecord{
		Update:      msgID,
		Session:     session,
		UpdateField: field,
		UpdateValue: value,
		Timestamp:   time.Now().Unix(),
	}
	ledgerMu.Lock()
	defer ledgerMu.Unlock()

	f, err := os.OpenFile(ledgerPath(session), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// readLedger reads all records from a session's ledger and merges updates
func readLedger(session string) []*MessageRecord {
	ledgerMu.Lock()
	defer ledgerMu.Unlock()

	f, err := os.Open(ledgerPath(session))
	if err != nil {
		return nil
	}
	defer f.Close()

	byID := make(map[string]*MessageRecord)
	var order []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec MessageRecord
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		// Update record: apply to existing
		if rec.Update != "" {
			if orig, ok := byID[rec.Update]; ok {
				applyUpdate(orig, rec.UpdateField, rec.UpdateValue)
			}
			continue
		}
		if rec.ID == "" {
			continue
		}
		if _, exists := byID[rec.ID]; !exists {
			order = append(order, rec.ID)
		}
		r := rec // copy
		byID[rec.ID] = &r
	}

	var result []*MessageRecord
	for _, id := range order {
		result = append(result, byID[id])
	}
	return result
}

func applyUpdate(rec *MessageRecord, field string, value any) {
	switch field {
	case "terminal_delivered":
		if v, ok := value.(bool); ok {
			rec.TerminalDelivered = v
		}
	case "telegram_delivered":
		if v, ok := value.(bool); ok {
			rec.TelegramDelivered = v
		}
	case "telegram_msg_id":
		switch v := value.(type) {
		case float64:
			rec.TelegramMsgID = int64(v)
		case int64:
			rec.TelegramMsgID = v
		}
	}
}

// isDelivered checks if a message ID has already been delivered to the given target
func isDelivered(session, msgID, target string) bool {
	records := readLedger(session)
	for _, r := range records {
		if r.ID == msgID {
			switch target {
			case "telegram":
				return r.TelegramDelivered
			case "terminal":
				return r.TerminalDelivered
			}
		}
	}
	return false
}

// findUndelivered returns messages not yet delivered to the given target ("telegram" or "terminal")
func findUndelivered(session, target string) []*MessageRecord {
	records := readLedger(session)
	var result []*MessageRecord
	for _, r := range records {
		switch target {
		case "telegram":
			if !r.TelegramDelivered {
				result = append(result, r)
			}
		case "terminal":
			if !r.TerminalDelivered {
				result = append(result, r)
			}
		}
	}
	return result
}

// contentHash returns a short hash of content for dedup IDs
func contentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:4])
}
