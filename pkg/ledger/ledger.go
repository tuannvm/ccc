package ledger

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tuannvm/ccc/pkg/config"
)

var ledgerMu sync.Mutex

func ledgerPath(session string) string {
	return filepath.Join(config.CacheDir(), "ledger-"+session+".jsonl")
}

// MessageRecord tracks the delivery state of a single message
type MessageRecord struct {
	ID                string `json:"id"`                        // unique: "{requestId}:{hash}" or "tg:{update_id}"
	Session           string `json:"session"`                   // session name
	Type              string `json:"type"`                      // user_prompt / tool_call / assistant_text / notification
	Text              string `json:"text"`                      // message content
	Origin            string `json:"origin"`                    // terminal / telegram / claude
	TerminalDelivered bool   `json:"terminal_delivered"`        // whether terminal received it
	TelegramDelivered bool   `json:"telegram_delivered"`        // whether Telegram received it
	TelegramMsgID     int64  `json:"telegram_msg_id,omitempty"` // Telegram message ID (for editing)
	Timestamp         int64  `json:"timestamp"`                 // unix timestamp
	Update            string `json:"update,omitempty"`          // if set, this is an update record for the given ID
	UpdateField       string `json:"update_field,omitempty"`    // field name to update
	UpdateValue       any    `json:"update_value,omitempty"`    // new value
}

// appendMessage writes a new message record to the session's ledger
func AppendMessage(rec *MessageRecord) error {
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

// AppendMessageIfAbsent writes rec only when msgID has not appeared in the
// session ledger yet. It is intentionally based on record existence, not final
// delivery state, so concurrent hook processes reserve a reply before sending it.
func AppendMessageIfAbsent(rec *MessageRecord) (bool, error) {
	if rec.Timestamp == 0 {
		rec.Timestamp = time.Now().Unix()
	}
	ledgerMu.Lock()
	defer ledgerMu.Unlock()

	path := ledgerPath(rec.Session)
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			var existing MessageRecord
			if json.Unmarshal(scanner.Bytes(), &existing) != nil {
				continue
			}
			if existing.ID == rec.ID {
				f.Close()
				return false, nil
			}
		}
		f.Close()
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, err
	}
	defer f.Close()

	data, err := json.Marshal(rec)
	if err != nil {
		return false, err
	}
	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		return false, err
	}
	return true, nil
}

// updateDelivery appends an update record to the ledger
func UpdateDelivery(session, msgID, field string, value any) error {
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

// ReadLedger reads all records from a session's ledger and merges updates
func ReadLedger(session string) []*MessageRecord {
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

// IsDelivered checks if a message ID has already been delivered to the given target
func IsDelivered(session, msgID, target string) bool {
	records := ReadLedger(session)
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

// FindUndelivered returns messages not yet delivered to the given target ("telegram" or "terminal")
func FindUndelivered(session, target string) []*MessageRecord {
	records := ReadLedger(session)
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

// ContentHash returns a short hash of content for dedup IDs
func ContentHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:4])
}
