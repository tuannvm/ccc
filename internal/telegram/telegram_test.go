package telegram

import (
	"encoding/json"
	"testing"
)

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
			json:   `{"ok": true, "result": []}`,
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
