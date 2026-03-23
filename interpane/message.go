// Package interpane handles inter-pane communication for team sessions.
package interpane

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tuannvm/ccc/session"
)

const (
	// MaxHops is the maximum number of inter-pane message hops to prevent infinite loops
	MaxHops = 5
	// HopCountHeader is the header prefix used to track hop count
	HopCountHeader = "CCC-Hops:"
)

// errQueued is a special error indicating a message was queued (not delivered)
var errQueued = fmt.Errorf("message queued")

// IsQueued checks if an error indicates a message was queued
func IsQueued(err error) bool {
	return err == errQueued
}

// Mention represents an @mention extracted from a response
type Mention struct {
	RequestID string           // Request ID for deduplication
	Role      session.PaneRole // Target role (planner, executor, reviewer)
	Message   string           // Message content after the @mention
	Context   string           // Surrounding context for disambiguation
	HopCount  int              // Hop count from incoming message (for propagation)
}

// MentionKey returns a unique key for this mention (for deduplication)
// Uses a simple hash of message content to distinguish multiple mentions to the same role
func (m *Mention) MentionKey() string {
	// Use a simple but effective hash of the message content
	// DJB2 hash algorithm
	content := m.Message
	var hash uint32
	for i := 0; i < len(content); i++ {
		hash = ((hash << 5) + hash) + uint32(content[i])
	}
	return fmt.Sprintf("%s:%s:%x", m.RequestID, m.Role, hash)
}

// InterPaneMessage represents an inter-pane message with delivery tracking
type InterPaneMessage struct {
	ID        string            // Unique message ID
	FromRole  session.PaneRole  // Source role
	ToRole    session.PaneRole  // Target role
	Content   string            // Message content
	HopCount  int               // Number of hops this message has made
	Timestamp time.Time         // When the message was created
}

// Router handles inter-pane message routing
type Router struct {
	tmuxPath    string
	sessionDir  string           // Session directory for persistence
	routedState *RoutedState    // Persisted routed request tracking
	messageQueue *MessageQueue  // Persisted message queue
	currentRequestID string      // Current request ID being routed
	mu          sync.RWMutex     // Protects sessionDir and currentRequestID
}

// NewRouter creates a new inter-pane router with persistence
func NewRouter() *Router {
	// Find tmux binary
	tmuxPath := "tmux"
	if path, err := exec.LookPath("tmux"); err == nil {
		tmuxPath = path
	}
	return &Router{
		tmuxPath: tmuxPath,
	}
}

// InitSession initializes the router for a specific session directory
func (r *Router) InitSession(sessionDir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create session directory if it doesn't exist
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	r.sessionDir = sessionDir

	// Load existing state
	routedState, err := LoadRoutedState(sessionDir)
	if err != nil {
		// Non-fatal, start with empty state
		r.routedState = &RoutedState{
			Requests:          make(map[string]time.Time),
			DeliveredMentions: make(map[string]time.Time),
		}
	} else {
		r.routedState = routedState
	}

	// Load existing queue
	messageQueue, err := LoadMessageQueue(sessionDir)
	if err != nil {
		// Non-fatal, start with empty queue
		r.messageQueue = &MessageQueue{Messages: []QueuedMessage{}}
	} else {
		r.messageQueue = messageQueue
	}

	return nil
}

// saveState persists the current state to disk
func (r *Router) saveState() error {
	if r.sessionDir == "" {
		return nil
	}

	if r.routedState != nil {
		if err := SaveRoutedState(r.sessionDir, r.routedState); err != nil {
			return err
		}
	}

	if r.messageQueue != nil {
		if err := SaveMessageQueue(r.sessionDir, r.messageQueue); err != nil {
			return err
		}
	}

	return nil
}

// IsAlreadyRouted checks if a request ID was already routed
func (r *Router) IsAlreadyRouted(requestID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.routedState == nil {
		return false
	}
	return r.routedState.IsRouted(requestID)
}

// MarkAsRouted marks a request ID as already routed and persists
func (r *Router) MarkAsRouted(requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.routedState == nil {
		return nil
	}
	r.routedState.MarkRouted(requestID)
	return r.saveState()
}

// ClearRoutedHistory clears the routing history (for testing or session reset)
func (r *Router) ClearRoutedHistory() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.routedState == nil {
		return nil
	}
	r.routedState.Requests = make(map[string]time.Time)
	return r.saveState()
}

// IsRequestQueued checks if a request ID is already in the message queue
func (r *Router) IsRequestQueued(requestID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.messageQueue == nil {
		return false
	}
	// Check if any message in the queue matches this request ID
	for _, msg := range r.messageQueue.Messages {
		if msg.RequestID == requestID {
			return true
		}
	}
	return false
}

// IsMentionQueued checks if a specific mention (requestID + toRole + content) is already in the queue
func (r *Router) IsMentionQueued(requestID string, toRole session.PaneRole, content string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.messageQueue == nil {
		return false
	}
	// Check if any message in the queue matches this request ID, role, AND content
	// This allows multiple mentions to the same role with different content
	for _, msg := range r.messageQueue.Messages {
		if msg.RequestID == requestID && msg.ToRole == string(toRole) && msg.Content == content {
			return true
		}
	}
	return false
}

// IsMentionDelivered checks if a specific mention (by mention key) was already delivered
func (r *Router) IsMentionDelivered(mentionKey string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.routedState == nil {
		return false
	}
	_, exists := r.routedState.DeliveredMentions[mentionKey]
	return exists
}

// MarkMentionDelivered marks a specific mention (by mention key) as delivered
func (r *Router) MarkMentionDelivered(mentionKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.routedState == nil {
		return nil
	}
	r.routedState.DeliveredMentions[mentionKey] = time.Now()
	return r.saveState()
}

// ClearDeliveredMentionsForRequest clears all delivered mentions for a specific request ID
// This is called when the entire request is marked as routed
func (r *Router) ClearDeliveredMentionsForRequest(requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.routedState == nil {
		return nil
	}
	// Remove all delivered mentions for this request ID
	for key := range r.routedState.DeliveredMentions {
		if strings.HasPrefix(key, requestID+":") {
			delete(r.routedState.DeliveredMentions, key)
		}
	}
	return r.saveState()
}

// SetCurrentRequestID sets the current request ID being routed
func (r *Router) SetCurrentRequestID(requestID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentRequestID = requestID
}

// GetCurrentRequestID gets the current request ID being routed
func (r *Router) GetCurrentRequestID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentRequestID
}

// ClearCurrentRequestID clears the current request ID
func (r *Router) ClearCurrentRequestID() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentRequestID = ""
}

// ParseMentions extracts @mentions from text content
// Returns mentions for @planner, @executor, @reviewer
// Mentions must be at the start of a line (after optional whitespace)
// requestID is used for deduplication - all mentions from this response share this ID
// incomingHopCount is the hop count from the incoming message (0 if from user directly)
func (r *Router) ParseMentions(text string, requestID string, incomingHopCount int) []Mention {
	var mentions []Mention

	// Split text by lines and process each line
	// Pattern: @role at start of line (with optional whitespace), case-insensitive
	// Groups: (1) role name
	linePattern := regexp.MustCompile(`(?i)^\s*@(planner|executor|reviewer)\b`)

	lines := strings.Split(text, "\n")
	var currentMention *Mention
	var mentionLines []string

	for _, line := range lines {
		matches := linePattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			// Save any existing mention
			if currentMention != nil {
				currentMention.Message = strings.TrimSpace(strings.Join(mentionLines, "\n"))
				// Extract context from full text
				currentMention.Context = r.extractContext(text, currentMention.Message)
				mentions = append(mentions, *currentMention)
			}

			// Start new mention
			roleStr := strings.ToLower(matches[1]) // Normalize to lowercase
			var role session.PaneRole
			switch roleStr {
			case "planner":
				role = session.RolePlanner
			case "executor":
				role = session.RoleExecutor
			case "reviewer":
				role = session.RoleReviewer
			default:
				continue
			}

			currentMention = &Mention{
				RequestID: requestID,
				Role:      role,
				HopCount:  incomingHopCount + 1, // Increment hop count for outgoing
				Message:   "",
			}

			// Get message content after the @role
			afterMention := linePattern.ReplaceAllString(line, "")
			mentionLines = []string{strings.TrimSpace(afterMention)}
		} else if currentMention != nil {
			// Continue current mention (line didn't match pattern)
			mentionLines = append(mentionLines, line)
		}
	}

	// Don't forget the last mention
	if currentMention != nil {
		currentMention.Message = strings.TrimSpace(strings.Join(mentionLines, "\n"))
		currentMention.Context = r.extractContext(text, currentMention.Message)
		mentions = append(mentions, *currentMention)
	}

	return mentions
}

// extractContext extracts surrounding lines for context
func (r *Router) extractContext(fullText, mention string) string {
	// Find the mention position
	idx := strings.Index(fullText, mention)
	if idx < 0 {
		return mention
	}

	// Get up to 200 characters before and after for context
	start := idx - 200
	if start < 0 {
		start = 0
	}
	end := idx + len(mention) + 200
	if end > len(fullText) {
		end = len(fullText)
	}

	context := fullText[start:end]
	lines := strings.Split(context, "\n")

	// Return up to 3 lines before and after, plus the mention line
	// Format as "...\nline\nline\n@mention...\nline\nline\n..."
	resultLines := []string{}
	if len(lines) > 6 {
		resultLines = append(resultLines, "...")
	}
	startLine := 0
	if len(lines) > 3 {
		startLine = len(lines) - 3
	}
	for i := startLine; i < len(lines) && i < startLine+6; i++ {
		resultLines = append(resultLines, lines[i])
	}
	if len(lines) > 6 {
		resultLines = append(resultLines, "...")
	}

	return strings.Join(resultLines, "\n")
}

// RouteToPane sends a message to a specific pane in a team session
// paneInfo should contain the tmux pane ID for the target role
// toRole is the target role (planner, executor, reviewer)
// Returns error if routing fails
// Returns special error errQueued if message was queued (pane busy)
func (r *Router) RouteToPane(sessionName string, paneID string, toRole session.PaneRole, message string, hopCount int) error {
	// Check hop count to prevent loops
	if hopCount >= MaxHops {
		return fmt.Errorf("max hops (%d) exceeded, dropping message", MaxHops)
	}

	if paneID == "" {
		return fmt.Errorf("empty pane ID for session: %s", sessionName)
	}

	// Check if target pane is ready (has active Claude prompt)
	if !r.paneHasActivePrompt(paneID) {
		// Pane is busy, queue the message
		if err := r.queueMessage(sessionName, paneID, toRole, message, hopCount); err != nil {
			return err
		}
		return errQueued
	}

	// Prepend hop count header to message (for next hop propagation)
	messageWithHeader := fmt.Sprintf("%s %d\n\n%s", HopCountHeader, hopCount, message)

	// Send message via tmux buffer (safer than send-keys for complex content)
	if err := r.sendViaBuffer(paneID, messageWithHeader); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Log the delivery
	r.logDelivery(sessionName, toRole, message, hopCount, "delivered")

	return nil
}

// inferRoleFromPaneID attempts to infer role from pane ID
// This is a best-effort inference for logging purposes
func (r *Router) inferRoleFromPaneID(paneID string) session.PaneRole {
	// Parse pane index from pane ID (format: %0, %1, %2, etc.)
	var index int
	if _, err := fmt.Sscanf(strings.TrimPrefix(paneID, "%"), "%d", &index); err == nil {
		switch index {
		case 0:
			return session.RolePlanner
		case 1:
			return session.RoleExecutor
		case 2:
			return session.RoleReviewer
		}
	}
	return session.RoleStandard
}

// paneHasActivePrompt checks if a pane has an active Claude prompt
func (r *Router) paneHasActivePrompt(paneTarget string) bool {
	cmd := exec.Command(r.tmuxPath, "capture-pane", "-t", paneTarget, "-p", "-e", "-J", "-S", "-15")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	content := string(out)
	lines := strings.Split(strings.TrimSpace(content), "\n")

	// Find the last non-empty line
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			// Check for Claude's prompt (❯ indicates ready for input)
			// But must avoid false positives from shell prompts and tool output
			if strings.Contains(line, "❯") {
				// Exclude obvious shell prompts ending with $, %, #, >
				trimmed := strings.TrimRight(line, " ")
				if len(trimmed) > 0 {
					lastChar := trimmed[len(trimmed)-1]
					if lastChar == '$' || lastChar == '%' || lastChar == '#' ||
					   lastChar == '>' || lastChar == ';' {
						return false
					}
				}
				// Also exclude lines that look like shell command output
				// (e.g., "user@host:~ ❯" from powerlevel10k)
				if strings.Contains(trimmed, "@") && strings.Contains(trimmed, ":") {
					return false
				}
				// Has ❯ and doesn't look like a shell prompt → Claude is ready
				return true
			}
			break
		}
	}

	return false
}

// sendViaBuffer sends a message using tmux buffer (safer than send-keys)
func (r *Router) sendViaBuffer(paneTarget, message string) error {
	// Generate unique buffer name
	bufferName := fmt.Sprintf("ccc-msg-%d", time.Now().UnixNano())

	// Load message into tmux buffer
	cmd := exec.Command(r.tmuxPath, "load-buffer", "-b", bufferName, "-")
	cmd.Stdin = strings.NewReader(message)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Paste buffer into target pane
	if err := exec.Command(r.tmuxPath, "paste-buffer", "-b", bufferName, "-t", paneTarget, "-d").Run(); err != nil {
		return err
	}

	// Send Enter to submit
	time.Sleep(50 * time.Millisecond)
	return exec.Command(r.tmuxPath, "send-keys", "-t", paneTarget, "Enter").Run()
}

// queueMessage queues a message for later delivery when pane becomes ready
func (r *Router) queueMessage(sessionName string, paneID string, toRole session.PaneRole, message string, hopCount int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.messageQueue == nil {
		return fmt.Errorf("message queue not initialized")
	}

	// Create queued message with current request ID
	qmsg := QueuedMessage{
		RequestID: r.currentRequestID,
		Session:   sessionName,
		ToPaneID:  paneID,
		ToRole:    string(toRole),
		Content:   message,
		HopCount:  hopCount,
	}

	// Enqueue the message
	if err := r.messageQueue.Enqueue(qmsg); err != nil {
		r.logDelivery(sessionName, toRole, message, hopCount, "queue-failed")
		return fmt.Errorf("failed to enqueue message: %w", err)
	}

	// Persist to disk
	if err := r.saveState(); err != nil {
		r.logDelivery(sessionName, toRole, message, hopCount, "queue-persist-failed")
		return fmt.Errorf("failed to persist queue: %w", err)
	}

	r.logDelivery(sessionName, toRole, message, hopCount, "queued")
	return nil
}

// ProcessQueue attempts to deliver queued messages that are ready for retry
// Returns the number of messages successfully delivered
func (r *Router) ProcessQueue(sessionName string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.messageQueue == nil {
		return 0, nil
	}

	// Get messages ready for retry
	ready := r.messageQueue.Dequeue()
	delivered := 0

	for _, msg := range ready {
		// Check if pane is now ready
		if !r.paneHasActivePrompt(msg.ToPaneID) {
			// Still busy, requeue with updated retry time
			msg.Retries++
			if err := r.messageQueue.Enqueue(msg); err != nil {
				// Log error but continue - message will be dropped if queue is full
				r.logDelivery(msg.Session, session.PaneRole(msg.ToRole), msg.Content, msg.HopCount, "requeue-failed")
			}
			continue
		}

		// Prepend hop count header to message (for next hop propagation)
		messageWithHeader := fmt.Sprintf("%s %d\n\n%s", HopCountHeader, msg.HopCount, msg.Content)

		// Try to deliver the message
		if err := r.sendViaBuffer(msg.ToPaneID, messageWithHeader); err != nil {
			// Failed to send, requeue with updated retry time
			msg.Retries++
			if enqueueErr := r.messageQueue.Enqueue(msg); enqueueErr != nil {
				// Log error but continue - message will be dropped if queue is full
				r.logDelivery(msg.Session, session.PaneRole(msg.ToRole), msg.Content, msg.HopCount, "requeue-failed")
			} else {
				r.logDelivery(msg.Session, session.PaneRole(msg.ToRole), msg.Content, msg.HopCount, "retry-failed")
			}
			continue
		}

		// Successfully delivered, mark as delivered (remove from queue)
		r.messageQueue.MarkDelivered(msg.ID)
		delivered++
		r.logDelivery(msg.Session, session.PaneRole(msg.ToRole), msg.Content, msg.HopCount, "delivered-from-queue")

		// Mark this mention as delivered (for retry deduplication)
		// Generate mention key from the message's stored data
		if r.routedState != nil && msg.RequestID != "" {
			// Create a temporary Mention to generate the key
			tempMention := Mention{
				RequestID: msg.RequestID,
				Role:      session.PaneRole(msg.ToRole),
				Message:   msg.Content,
			}
			mentionKey := tempMention.MentionKey()
			r.routedState.DeliveredMentions[mentionKey] = time.Now()
		}
	}

	// Persist updated queue state
	if err := r.saveState(); err != nil {
		return delivered, fmt.Errorf("failed to persist queue after processing: %w", err)
	}

	return delivered, nil
}

// logDelivery logs a message delivery attempt
func (r *Router) logDelivery(sessionName string, role session.PaneRole, message string, hopCount int, status string) {
	logDir := filepath.Join(os.TempDir(), "ccc-interpane")
	os.MkdirAll(logDir, 0755)

	logPath := filepath.Join(logDir, "delivery.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("15:04:05")
	truncatedMsg := message
	if len(truncatedMsg) > 100 {
		truncatedMsg = truncatedMsg[:100] + "..."
	}

	fmt.Fprintf(f, "[%s] %s -> %s: hops=%d status=%s msg=%q\n",
		timestamp, sessionName, role, hopCount, status, truncatedMsg)
}
