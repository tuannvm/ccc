package interpane

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadRoutedState(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Test loading non-existent state
	state, err := LoadRoutedState(tmpDir)
	if err != nil {
		t.Fatalf("LoadRoutedState() error = %v", err)
	}
	if state == nil {
		t.Fatal("LoadRoutedState() returned nil state")
	}
	if len(state.Requests) != 0 {
		t.Errorf("LoadRoutedState() returned %d requests, want 0", len(state.Requests))
	}
}

func TestSaveAndLoadRoutedState(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and save state
	state := &RoutedState{}
	state.Requests = make(map[string]time.Time)
	state.Requests["req1"] = time.Now()
	state.Requests["req2"] = time.Now().Add(-2 * time.Hour) // Old entry

	err := SaveRoutedState(tmpDir, state)
	if err != nil {
		t.Fatalf("SaveRoutedState() error = %v", err)
	}

	// Load state
	loaded, err := LoadRoutedState(tmpDir)
	if err != nil {
		t.Fatalf("LoadRoutedState() error = %v", err)
	}

	// Old entry should be cleaned up
	if len(loaded.Requests) != 1 {
		t.Errorf("LoadRoutedState() returned %d requests, want 1 (old entries cleaned up)", len(loaded.Requests))
	}
	if _, exists := loaded.Requests["req1"]; !exists {
		t.Error("LoadRoutedState() did not contain req1")
	}
	if _, exists := loaded.Requests["req2"]; exists {
		t.Error("LoadRoutedState() contained req2 (should have been cleaned up)")
	}
}

func TestRoutedStateIsRouted(t *testing.T) {
	state := &RoutedState{}
	state.Requests = make(map[string]time.Time)

	// Initially not routed
	if state.IsRouted("test-req") {
		t.Error("IsRouted() returned true for non-existent request")
	}

	// Mark as routed
	state.MarkRouted("test-req")
	if !state.IsRouted("test-req") {
		t.Error("IsRouted() returned false after MarkRouted()")
	}
}

func TestLoadMessageQueue(t *testing.T) {
	tmpDir := t.TempDir()

	// Test loading non-existent queue
	queue, err := LoadMessageQueue(tmpDir)
	if err != nil {
		t.Fatalf("LoadMessageQueue() error = %v", err)
	}
	if queue == nil {
		t.Fatal("LoadMessageQueue() returned nil queue")
	}
	if len(queue.Messages) != 0 {
		t.Errorf("LoadMessageQueue() returned %d messages, want 0", len(queue.Messages))
	}
}

func TestSaveAndLoadMessageQueue(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and save queue
	queue := &MessageQueue{
		Messages: []QueuedMessage{
			{
				Session:    "test-session",
				ToPaneID:   "%0",
				ToRole:     "planner",
				Content:    "test message",
				HopCount:   1,
				Timestamp:  time.Now(),
				RetryAfter: time.Now().Add(10 * time.Second),
				Retries:    0,
			},
		},
	}

	err := SaveMessageQueue(tmpDir, queue)
	if err != nil {
		t.Fatalf("SaveMessageQueue() error = %v", err)
	}

	// Load queue
	loaded, err := LoadMessageQueue(tmpDir)
	if err != nil {
		t.Fatalf("LoadMessageQueue() error = %v", err)
	}

	if len(loaded.Messages) != 1 {
		t.Fatalf("LoadMessageQueue() returned %d messages, want 1", len(loaded.Messages))
	}

	msg := loaded.Messages[0]
	if msg.Session != "test-session" {
		t.Errorf("LoadMessageQueue() Session = %q, want %q", msg.Session, "test-session")
	}
	if msg.ToPaneID != "%0" {
		t.Errorf("LoadMessageQueue() ToPaneID = %q, want %q", msg.ToPaneID, "%0")
	}
	if msg.Content != "test message" {
		t.Errorf("LoadMessageQueue() Content = %q, want %q", msg.Content, "test message")
	}
}

func TestMessageQueueEnqueue(t *testing.T) {
	queue := &MessageQueue{
		Messages: []QueuedMessage{},
	}

	msg := QueuedMessage{
		Session:  "test-session",
		ToPaneID: "%0",
		ToRole:   "planner",
		Content:  "test message",
		HopCount: 1,
	}

	err := queue.Enqueue(msg)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if len(queue.Messages) != 1 {
		t.Errorf("Enqueue() resulted in %d messages, want 1", len(queue.Messages))
	}

	// Check that ID and timestamp were set
	if queue.Messages[0].ID == "" {
		t.Error("Enqueue() did not set ID")
	}
	if queue.Messages[0].Timestamp.IsZero() {
		t.Error("Enqueue() did not set Timestamp")
	}
	if queue.Messages[0].Retries != 0 {
		t.Errorf("Enqueue() Retries = %d, want 0", queue.Messages[0].Retries)
	}
}

func TestMessageQueueMaxSize(t *testing.T) {
	queue := &MessageQueue{
		Messages: []QueuedMessage{},
	}

	// Fill one role to its per-role limit
	for i := 0; i < maxQueueSizePerRole; i++ {
		msg := QueuedMessage{
			Session:  "test-session",
			ToPaneID: "%0",
			ToRole:   "planner",
			Content:  "test message",
			HopCount: 1,
		}
		err := queue.Enqueue(msg)
		if err != nil {
			t.Fatalf("Enqueue() error at %d for planner: %v", i, err)
		}
	}

	// Try to add one more to the same role - should fail
	msg := QueuedMessage{
		Session:  "test-session",
		ToPaneID: "%0",
		ToRole:   "planner",
		Content:  "overflow",
		HopCount: 1,
	}
	err := queue.Enqueue(msg)
	if err == nil {
		t.Error("Enqueue() did not return error when role queue is full")
	}

	// But we should still be able to add to a different role
	msg2 := QueuedMessage{
		Session:  "test-session",
		ToPaneID: "%1",
		ToRole:   "executor",
		Content:  "different role",
		HopCount: 1,
	}
	err = queue.Enqueue(msg2)
	if err != nil {
		t.Errorf("Enqueue() returned error for different role: %v", err)
	}

	// Fill executor to its limit
	for i := 1; i < maxQueueSizePerRole; i++ {
		msg := QueuedMessage{
			Session:  "test-session",
			ToPaneID: "%1",
			ToRole:   "executor",
			Content:  "test message",
			HopCount: 1,
		}
		err := queue.Enqueue(msg)
		if err != nil {
			t.Fatalf("Enqueue() error at %d for executor: %v", i, err)
		}
	}

	// Fill reviewer to its limit
	for i := 0; i < maxQueueSizePerRole; i++ {
		msg := QueuedMessage{
			Session:  "test-session",
			ToPaneID: "%2",
			ToRole:   "reviewer",
			Content:  "test message",
			HopCount: 1,
		}
		err := queue.Enqueue(msg)
		if err != nil {
			t.Fatalf("Enqueue() error at %d for reviewer: %v", i, err)
		}
	}

	// Now the queue should be at global limit (3 * maxQueueSizePerRole)
	if len(queue.Messages) != maxQueueSize {
		t.Errorf("Queue size = %d, want %d", len(queue.Messages), maxQueueSize)
	}

	// Any more messages should fail due to global limit
	msg3 := QueuedMessage{
		Session:  "test-session",
		ToPaneID: "%0",
		ToRole:   "planner",
		Content:  "global overflow",
		HopCount: 1,
	}
	err = queue.Enqueue(msg3)
	if err == nil {
		t.Error("Enqueue() did not return error when global queue is full")
	}
}

func TestMessageQueueDequeue(t *testing.T) {
	now := time.Now()
	queue := &MessageQueue{
		Messages: []QueuedMessage{
			{
				ID:         "msg1",
				Session:    "test-session",
				ToPaneID:   "%0",
				RetryAfter: now.Add(-1 * time.Hour), // Ready
				Content:    "ready message",
			},
			{
				ID:         "msg2",
				Session:    "test-session",
				ToPaneID:   "%1",
				RetryAfter: now.Add(1 * time.Hour), // Not ready
				Content:    "future message",
			},
		},
	}

	ready := queue.Dequeue()

	if len(ready) != 1 {
		t.Errorf("Dequeue() returned %d messages, want 1", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != "msg1" {
		t.Errorf("Dequeue() returned ID %q, want msg1", ready[0].ID)
	}

	// Queue should still have the non-ready message
	if len(queue.Messages) != 1 {
		t.Errorf("Dequeue() left %d messages in queue, want 1", len(queue.Messages))
	}
}

func TestMessageQueueMarkDelivered(t *testing.T) {
	queue := &MessageQueue{
		Messages: []QueuedMessage{
			{ID: "msg1", Content: "message 1"},
			{ID: "msg2", Content: "message 2"},
			{ID: "msg3", Content: "message 3"},
		},
	}

	queue.MarkDelivered("msg2")

	if len(queue.Messages) != 2 {
		t.Errorf("MarkDelivered() left %d messages, want 2", len(queue.Messages))
	}

	for _, msg := range queue.Messages {
		if msg.ID == "msg2" {
			t.Error("MarkDelivered() did not remove msg2")
		}
	}
}

func TestCleanupSessionState(t *testing.T) {
	tmpDir := t.TempDir()

	// Create state files
	routedPath := filepath.Join(tmpDir, routedStateFile)
	queuePath := filepath.Join(tmpDir, messageQueueFile)

	// Write test files
	os.WriteFile(routedPath, []byte("{}"), 0600)
	os.WriteFile(queuePath, []byte("[]"), 0600)

	// Verify files exist
	if _, err := os.Stat(routedPath); os.IsNotExist(err) {
		t.Fatal("Test setup failed: routed state file not created")
	}
	if _, err := os.Stat(queuePath); os.IsNotExist(err) {
		t.Fatal("Test setup failed: queue file not created")
	}

	// Cleanup
	err := CleanupSessionState(tmpDir)
	if err != nil {
		t.Fatalf("CleanupSessionState() error = %v", err)
	}

	// Verify files are gone
	if _, err := os.Stat(routedPath); !os.IsNotExist(err) {
		t.Error("CleanupSessionState() did not remove routed state file")
	}
	if _, err := os.Stat(queuePath); !os.IsNotExist(err) {
		t.Error("CleanupSessionState() did not remove queue file")
	}
}

func TestCleanupSessionStateNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	// Cleanup on non-existent files should not error
	err := CleanupSessionState(tmpDir)
	if err != nil {
		t.Fatalf("CleanupSessionState() error = %v", err)
	}
}
