// Package interpane provides persistence for inter-pane communication state.
package interpane

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RoutedState tracks which request IDs have already been routed
type RoutedState struct {
	Requests map[string]time.Time `json:"requests"` // requestID -> timestamp
	// DeliveredMentions tracks mentions that were successfully delivered
	// Key format: "requestID:role:hash" where hash is derived from message content
	// This allows multiple mentions to the same role with different content
	DeliveredMentions map[string]time.Time `json:"delivered_mentions"` // mention key -> timestamp
	mu       sync.RWMutex       `json:"-"`
}

// QueuedMessage represents a message waiting to be delivered to a busy pane
type QueuedMessage struct {
	ID         string    `json:"id"`         // Unique message ID
	RequestID  string    `json:"request_id"` // Request ID for deduplication
	Session    string    `json:"session"`    // Team session name
	ToPaneID   string    `json:"to_pane_id"` // Target tmux pane ID
	ToRole     string    `json:"to_role"`    // Target role name
	Content    string    `json:"content"`    // Message content
	HopCount   int       `json:"hop_count"`  // Current hop count
	Timestamp  time.Time `json:"timestamp"`  // When queued
	RetryAfter time.Time `json:"retry_after"` // When to retry
	Retries    int       `json:"retries"`     // Number of retry attempts
}

// MessageQueue manages queued messages for busy panes
type MessageQueue struct {
	Messages []QueuedMessage `json:"messages"`
	mu       sync.RWMutex    `json:"-"`
}

const (
	// State filenames
	routedStateFile    = "routed-mentions.json"
	messageQueueFile   = "queued-messages.jsonl"
	maxQueueSizePerRole = 25 // Per-role queue limit (planner/executor/reviewer)
	maxQueueSize       = 75  // Global queue limit (3 roles × 25)
	maxRetries         = 5
	baseRetryDelay     = 10 * time.Second
	maxRetryDelay      = 5 * time.Minute
)

// LoadRoutedState loads the routed request state from disk
func LoadRoutedState(sessionDir string) (*RoutedState, error) {
	state := &RoutedState{
		Requests:          make(map[string]time.Time),
		DeliveredMentions: make(map[string]time.Time),
	}

	path := filepath.Join(sessionDir, routedStateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing state, return empty
			return state, nil
		}
		return nil, fmt.Errorf("failed to read routed state: %w", err)
	}

	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("failed to parse routed state: %w", err)
	}

	// Initialize DeliveredMentions if nil (backwards compatibility)
	if state.DeliveredMentions == nil {
		state.DeliveredMentions = make(map[string]time.Time)
	}

	// Clean up old entries (older than 1 hour)
	state.cleanupOldEntries()

	return state, nil
}

// SaveRoutedState persists the routed request state to disk
func SaveRoutedState(sessionDir string, state *RoutedState) error {
	state.mu.RLock()
	defer state.mu.RUnlock()

	path := filepath.Join(sessionDir, routedStateFile)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal routed state: %w", err)
	}

	// Write to temp file first, then rename (atomic write)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write routed state: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to commit routed state: %w", err)
	}

	return nil
}

// IsRouted checks if a request ID has already been routed
func (s *RoutedState) IsRouted(requestID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.Requests[requestID]
	return exists
}

// MarkRouted marks a request ID as routed
func (s *RoutedState) MarkRouted(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests[requestID] = time.Now()
}

// IsMentionDelivered checks if a specific mention (requestID + role) was already delivered
// cleanupOldEntries removes entries older than 1 hour
func (s *RoutedState) cleanupOldEntries() {
	cutoff := time.Now().Add(-time.Hour)
	for id, ts := range s.Requests {
		if ts.Before(cutoff) {
			delete(s.Requests, id)
		}
	}
	for id, ts := range s.DeliveredMentions {
		if ts.Before(cutoff) {
			delete(s.DeliveredMentions, id)
		}
	}
}

// LoadMessageQueue loads the message queue from disk
func LoadMessageQueue(sessionDir string) (*MessageQueue, error) {
	queue := &MessageQueue{
		Messages: []QueuedMessage{},
	}

	path := filepath.Join(sessionDir, messageQueueFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing queue, return empty
			return queue, nil
		}
		return nil, fmt.Errorf("failed to read message queue: %w", err)
	}

	if err := json.Unmarshal(data, &queue.Messages); err != nil {
		return nil, fmt.Errorf("failed to parse message queue: %w", err)
	}

	// Clean up old or delivered messages
	queue.cleanup()

	return queue, nil
}

// SaveMessageQueue persists the message queue to disk
func SaveMessageQueue(sessionDir string, queue *MessageQueue) error {
	queue.mu.RLock()
	defer queue.mu.RUnlock()

	path := filepath.Join(sessionDir, messageQueueFile)
	data, err := json.MarshalIndent(queue.Messages, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message queue: %w", err)
	}

	// Write to temp file first, then rename (atomic write)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write message queue: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to commit message queue: %w", err)
	}

	return nil
}

// Enqueue adds a message to the queue
func (q *MessageQueue) Enqueue(msg QueuedMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check per-role limit (primary constraint)
	roleCount := 0
	for _, m := range q.Messages {
		if m.ToRole == msg.ToRole {
			roleCount++
		}
	}
	if roleCount >= maxQueueSizePerRole {
		return fmt.Errorf("message queue full for role %s (max %d per role)", msg.ToRole, maxQueueSizePerRole)
	}

	// Check global limit as a failsafe
	if len(q.Messages) >= maxQueueSize {
		return fmt.Errorf("message queue full (max %d globally)", maxQueueSize)
	}

	// For new messages (ID is empty), initialize fields
	// For requeues (ID is set), preserve retry state
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("%s-%d", msg.Session, time.Now().UnixNano())
		msg.Timestamp = time.Now()
		msg.Retries = 0
	}

	// Calculate retry delay with exponential backoff
	// delay = base * 2^retries, capped at maxRetryDelay
	delay := time.Duration(float64(baseRetryDelay) * math.Pow(2, float64(msg.Retries)))
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	msg.RetryAfter = time.Now().Add(delay)

	q.Messages = append(q.Messages, msg)
	return nil
}

// Dequeue retrieves messages ready for retry
func (q *MessageQueue) Dequeue() []QueuedMessage {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var ready []QueuedMessage
	var remaining []QueuedMessage

	for _, msg := range q.Messages {
		if msg.RetryAfter.Before(now) {
			ready = append(ready, msg)
		} else {
			remaining = append(remaining, msg)
		}
	}

	q.Messages = remaining
	return ready
}

// MarkDelivered removes a message from the queue after successful delivery
func (q *MessageQueue) MarkDelivered(messageID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var remaining []QueuedMessage
	for _, msg := range q.Messages {
		if msg.ID != messageID {
			remaining = append(remaining, msg)
		}
	}
	q.Messages = remaining
}

// cleanup removes old or invalid messages
func (q *MessageQueue) cleanup() {
	var valid []QueuedMessage
	now := time.Now()

	for _, msg := range q.Messages {
		// Remove messages older than 1 hour
		if now.Sub(msg.Timestamp) > time.Hour {
			continue
		}
		// Remove messages with excessive retries
		if msg.Retries >= maxRetries {
			continue
		}
		// Increment retries for messages that are being kept
		if msg.RetryAfter.Before(now) {
			msg.Retries++
			// Calculate next retry with exponential backoff
			delay := time.Duration(msg.Retries) * baseRetryDelay
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
			msg.RetryAfter = now.Add(delay)
		}
		valid = append(valid, msg)
	}

	q.Messages = valid
}

// CleanupSessionState removes all inter-pane state files for a session
func CleanupSessionState(sessionDir string) error {
	// Remove routed state file
	routedPath := filepath.Join(sessionDir, routedStateFile)
	if err := os.Remove(routedPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove routed state: %w", err)
	}

	// Remove message queue file
	queuePath := filepath.Join(sessionDir, messageQueueFile)
	if err := os.Remove(queuePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove message queue: %w", err)
	}

	return nil
}
