package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusDeleted    TaskStatus = "deleted"
)

// Task represents a single task in the task list
type Task struct {
	TaskID      string     `json:"task_id"`
	Subject     string     `json:"subject"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	Owner       string     `json:"owner,omitempty"`
	ActiveForm  string     `json:"active_form,omitempty"`
	CreatedAt   int64      `json:"created_at"`
	UpdatedAt   int64      `json:"updated_at"`
	Metadata    any        `json:"metadata,omitempty"`
}

// TaskState tracks all tasks for a session and the Telegram message ID
type TaskState struct {
	MsgID  int64            `json:"msg_id"`
	Tasks  map[string]*Task `json:"tasks"` // task_id -> Task
	Order  []string         `json:"order"` // ordered task IDs
	mu     sync.RWMutex
}

var taskStateMu sync.Mutex

// taskStatePath returns the path for the task state file
func taskStatePath(sessionName string) string {
	return filepath.Join(cacheDir(), "tasks-"+sessionName+".json")
}

// loadTaskState loads the task state from disk
func loadTaskState(sessionName string) *TaskState {
	taskStateMu.Lock()
	defer taskStateMu.Unlock()

	path := taskStatePath(sessionName)
	data, err := os.ReadFile(path)
	if err != nil {
		// Return empty state if file doesn't exist
		return &TaskState{
			Tasks: make(map[string]*Task),
			Order: make([]string, 0),
		}
	}

	var state TaskState
	if err := json.Unmarshal(data, &state); err != nil {
		hookLog("load-task-state: failed to parse %s: %v", path, err)
		return &TaskState{
			Tasks: make(map[string]*Task),
			Order: make([]string, 0),
		}
	}

	if state.Tasks == nil {
		state.Tasks = make(map[string]*Task)
	}
	if state.Order == nil {
		state.Order = make([]string, 0)
	}

	return &state
}

// saveTaskState saves the task state to disk
func saveTaskState(sessionName string, state *TaskState) {
	taskStateMu.Lock()
	defer taskStateMu.Unlock()

	path := taskStatePath(sessionName)
	data, err := json.Marshal(state)
	if err != nil {
		hookLog("save-task-state: failed to marshal state for %s: %v", sessionName, err)
		return
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		hookLog("save-task-state: failed to write %s: %v", path, err)
	}
}

// addTask adds a new task to the state
func addTask(sessionName string, task *Task) {
	state := loadTaskState(sessionName)
	state.mu.Lock()

	// Set timestamps
	now := time.Now().Unix()
	task.CreatedAt = now
	task.UpdatedAt = now

	// Add to tasks map
	state.Tasks[task.TaskID] = task

	// Add to order if not already present
	found := false
	for _, id := range state.Order {
		if id == task.TaskID {
			found = true
			break
		}
	}
	if !found {
		state.Order = append(state.Order, task.TaskID)
	}

	state.mu.Unlock()

	// Save after releasing lock to avoid blocking
	saveTaskState(sessionName, state)
	hookLog("add-task: session=%s task_id=%s subject=%q", sessionName, task.TaskID, task.Subject)
}

// updateTask updates an existing task
func updateTask(sessionName string, taskID string, updates map[string]interface{}) {
	state := loadTaskState(sessionName)
	state.mu.Lock()

	task, exists := state.Tasks[taskID]
	if !exists {
		state.mu.Unlock()
		hookLog("update-task: task not found: %s", taskID)
		return
	}

	// Apply updates
	task.UpdatedAt = time.Now().Unix()

	if subject, ok := updates["subject"].(string); ok && subject != "" {
		task.Subject = subject
	}
	if description, ok := updates["description"].(string); ok && description != "" {
		task.Description = description
	}
	if status, ok := updates["status"].(string); ok && status != "" {
		task.Status = TaskStatus(status)
	}
	if owner, ok := updates["owner"].(string); ok && owner != "" {
		task.Owner = owner
	}
	if activeForm, ok := updates["active_form"].(string); ok && activeForm != "" {
		task.ActiveForm = activeForm
	}
	if metadata, ok := updates["metadata"]; ok {
		task.Metadata = metadata
	}

	state.mu.Unlock()

	// Save after releasing lock to avoid blocking
	saveTaskState(sessionName, state)
	hookLog("update-task: session=%s task_id=%s status=%s", sessionName, taskID, task.Status)
}

// removeTask marks a task as deleted (doesn't actually remove to preserve history)
func removeTask(sessionName string, taskID string) {
	updateTask(sessionName, taskID, map[string]interface{}{
		"status": TaskStatusDeleted,
	})
	// Periodically cleanup deleted tasks to prevent file bloat
	cleanupDeletedTasks(sessionName)
}

// getActiveTasks returns all non-deleted tasks in order
func getActiveTasks(state *TaskState) []*Task {
	state.mu.RLock()
	defer state.mu.RUnlock()

	var tasks []*Task
	for _, id := range state.Order {
		task, exists := state.Tasks[id]
		if !exists {
			continue
		}
		if task.Status == TaskStatusDeleted {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks
}

// statusIcon returns the appropriate icon for a task status
func statusIcon(status TaskStatus) string {
	switch status {
	case TaskStatusPending:
		return "⏳"
	case TaskStatusInProgress:
		return "🔄"
	case TaskStatusCompleted:
		return "✅"
	default:
		return "❓"
	}
}

// formatTaskListHTML formats the task list as HTML for Telegram
func formatTaskListHTML(sessionName string, state *TaskState) string {
	tasks := getActiveTasks(state)

	if len(tasks) == 0 {
		return fmt.Sprintf("📋 Tasks - %s\n\nNo active tasks", htmlEscape(sessionName))
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("📋 Tasks - %s (%d tasks)", htmlEscape(sessionName), len(tasks)))
	lines = append(lines, "")

	// Sort tasks: pending/in-progress first, then completed
	sort.Slice(tasks, func(i, j int) bool {
		// Both pending/in-progress: sort by updated at (newest first)
		if (tasks[i].Status == TaskStatusPending || tasks[i].Status == TaskStatusInProgress) &&
			(tasks[j].Status == TaskStatusPending || tasks[j].Status == TaskStatusInProgress) {
			return tasks[i].UpdatedAt > tasks[j].UpdatedAt
		}
		// i is pending/in-progress, j is completed: i comes first
		if tasks[i].Status == TaskStatusPending || tasks[i].Status == TaskStatusInProgress {
			return tasks[j].Status == TaskStatusCompleted
		}
		// i is completed, j is pending/in-progress: j comes first
		if tasks[j].Status == TaskStatusPending || tasks[j].Status == TaskStatusInProgress {
			return false
		}
		// Both completed: sort by updated at (newest first)
		return tasks[i].UpdatedAt > tasks[j].UpdatedAt
	})

	for _, task := range tasks {
		icon := statusIcon(task.Status)
		subject := htmlEscape(task.Subject)
		lines = append(lines, fmt.Sprintf("%s %s", icon, subject))
	}

	return strings.Join(lines, "\n")
}

// setTaskListMsgID sets the Telegram message ID for the task list
func setTaskListMsgID(sessionName string, msgID int64) {
	state := loadTaskState(sessionName)
	state.mu.Lock()
	state.MsgID = msgID
	state.mu.Unlock()

	saveTaskState(sessionName, state)
	hookLog("set-task-msgid: session=%s msg_id=%d", sessionName, msgID)
}

// getTaskListMsgID gets the Telegram message ID for the task list
func getTaskListMsgID(sessionName string) int64 {
	state := loadTaskState(sessionName)
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.MsgID
}

// clearTaskState removes all task state for a session
func clearTaskState(sessionName string) {
	path := taskStatePath(sessionName)
	os.Remove(path)
	hookLog("clear-task-state: session=%s", sessionName)
}

// cleanupDeletedTasks removes deleted tasks from the state to prevent file bloat
// Call this periodically or when the number of deleted tasks exceeds a threshold
func cleanupDeletedTasks(sessionName string) {
	state := loadTaskState(sessionName)
	state.mu.Lock()

	// Count deleted tasks and collect their IDs
	var deletedIDs []string
	var activeIDs []string
	for _, id := range state.Order {
		task, exists := state.Tasks[id]
		if !exists {
			continue
		}
		if task.Status == TaskStatusDeleted {
			deletedIDs = append(deletedIDs, id)
		} else {
			activeIDs = append(activeIDs, id)
		}
	}

	// Only cleanup if there are more than 10 deleted tasks
	if len(deletedIDs) <= 10 {
		state.mu.Unlock()
		return
	}

	// Remove deleted tasks from map
	for _, id := range deletedIDs {
		delete(state.Tasks, id)
	}

	// Update order to only include active tasks
	state.Order = activeIDs

	state.mu.Unlock()

	saveTaskState(sessionName, state)
	hookLog("cleanup-deleted-tasks: session=%s removed=%d tasks", sessionName, len(deletedIDs))
}
