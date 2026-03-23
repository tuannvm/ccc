package session

import (
	"fmt"
	"sync"
	"testing"
)

// mockSessionForRuntime is a minimal Session implementation for testing TeamRuntime
type mockSessionForRuntime struct {
	name string
}

func (m *mockSessionForRuntime) GetName() string {
	return m.name
}

func (m *mockSessionForRuntime) GetPath() string {
	return "/mock/path"
}

func (m *mockSessionForRuntime) GetTopicID() int64 {
	return 123
}

func (m *mockSessionForRuntime) GetProviderName() string {
	return "test"
}

func (m *mockSessionForRuntime) GetType() SessionKind {
	return SessionKindTeam
}

func (m *mockSessionForRuntime) GetLayoutName() string {
	return "team-3pane"
}

func (m *mockSessionForRuntime) GetPanes() map[PaneRole]*PaneInfo {
	return map[PaneRole]*PaneInfo{
		RolePlanner:  {Role: RolePlanner},
		RoleExecutor: {Role: RoleExecutor},
		RoleReviewer: {Role: RoleReviewer},
	}
}

// TestGetRoleTarget tests role to pane index mapping
func TestGetRoleTarget(t *testing.T) {
	runtime := &TeamRuntime{}
	sess := &mockSessionForRuntime{name: "test-session"}

	tests := []struct {
		name        string
		role        PaneRole
		wantTarget  string
		wantErr     bool
	}{
		{
			name:       "planner role",
			role:       RolePlanner,
			wantTarget: "ccc-team:test-session.0",
			wantErr:    false,
		},
		{
			name:       "executor role",
			role:       RoleExecutor,
			wantTarget: "ccc-team:test-session.1",
			wantErr:    false,
		},
		{
			name:       "reviewer role",
			role:       RoleReviewer,
			wantTarget: "ccc-team:test-session.2",
			wantErr:    false,
		},
		{
			name:       "standard role (not in 3-pane layout)",
			role:       RoleStandard,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "unknown role",
			role:       "unknown",
			wantTarget: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := runtime.GetRoleTarget(sess, tt.role)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRoleTarget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if target != tt.wantTarget {
				t.Errorf("GetRoleTarget() target = %q, want %q", target, tt.wantTarget)
			}
		})
	}
}

// TestGetDefaultTarget tests that executor is the default target
func TestGetDefaultTarget(t *testing.T) {
	runtime := &TeamRuntime{}
	sess := &mockSessionForRuntime{name: "test-session"}

	target, err := runtime.GetDefaultTarget(sess)
	if err != nil {
		t.Errorf("GetDefaultTarget() error = %v", err)
		return
	}

	wantTarget := "ccc-team:test-session.1"
	if target != wantTarget {
		t.Errorf("GetDefaultTarget() target = %q, want %q", target, wantTarget)
	}
}

// TestGetSessionName tests name sanitization for tmux
func TestGetSessionName(t *testing.T) {
	runtime := &TeamRuntime{}

	tests := []struct {
		name         string
		sessionName  string
		wantSanitized string
	}{
		{
			name:         "simple name",
			sessionName:  "myproject",
			wantSanitized: "myproject",
		},
		{
			name:         "with dots",
			sessionName:  "my.project",
			wantSanitized: "my__project",
		},
		{
			name:         "multiple dots",
			sessionName:  "my.project.name",
			wantSanitized: "my__project__name",
		},
		{
			name:         "with dashes (unchanged)",
			sessionName:  "my-project",
			wantSanitized: "my-project",
		},
		{
			name:         "mixed dots and dashes",
			sessionName:  "my.project-name",
			wantSanitized: "my__project-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := &mockSessionForRuntime{name: tt.sessionName}
			result := runtime.getSessionName(sess)
			if result != tt.wantSanitized {
				t.Errorf("getSessionName() = %q, want %q", result, tt.wantSanitized)
			}
		})
	}
}

// TestGetOrCreateTeamWindow tests team window creation and retrieval
func TestGetOrCreateTeamWindow(t *testing.T) {
	// Clear any existing state
	teamSessionsMutex.Lock()
	originalSessions := make(map[string]*TeamWindowState)
	for k, v := range ActiveTeamSessions {
		originalSessions[k] = v
		delete(ActiveTeamSessions, k)
	}
	teamSessionsMutex.Unlock()

	// Restore after test
	defer func() {
		teamSessionsMutex.Lock()
		// Clean up any test-created sessions before restoring originals
		for k := range ActiveTeamSessions {
			if _, ok := originalSessions[k]; !ok {
				delete(ActiveTeamSessions, k)
			}
		}
		// Restore original sessions
		for k, v := range originalSessions {
			ActiveTeamSessions[k] = v
		}
		teamSessionsMutex.Unlock()
	}()

	tests := []struct {
		name        string
		sessionName string
		wantPanes   int
	}{
		{
			name:        "create new team window",
			sessionName: "test-team-1",
			wantPanes:   3,
		},
		{
			name:        "get existing team window",
			sessionName: "test-team-1",
			wantPanes:   3,
		},
		{
			name:        "create another team window",
			sessionName: "test-team-2",
			wantPanes:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := GetOrCreateTeamWindow(tt.sessionName)
			if err != nil {
				t.Errorf("GetOrCreateTeamWindow() error = %v", err)
				return
			}
			if state.SessionName != tt.sessionName {
				t.Errorf("GetOrCreateTeamWindow() SessionName = %q, want %q", state.SessionName, tt.sessionName)
			}
			if len(state.Panes) != tt.wantPanes {
				t.Errorf("GetOrCreateTeamWindow() Panes length = %d, want %d", len(state.Panes), tt.wantPanes)
			}

			// Verify pane roles are correct
			if state.Panes[0].Role != RolePlanner {
				t.Errorf("Pane 0 role = %v, want Planner", state.Panes[0].Role)
			}
			if state.Panes[1].Role != RoleExecutor {
				t.Errorf("Pane 1 role = %v, want Executor", state.Panes[1].Role)
			}
			if state.Panes[2].Role != RoleReviewer {
				t.Errorf("Pane 2 role = %v, want Reviewer", state.Panes[2].Role)
			}
		})
	}
}

// TestDeleteTeamWindow tests team window deletion
func TestDeleteTeamWindow(t *testing.T) {
	// Create a team window
	sessionName := "test-delete-window"
	_, err := GetOrCreateTeamWindow(sessionName)
	if err != nil {
		t.Fatalf("GetOrCreateTeamWindow() failed: %v", err)
	}

	// Verify it exists
	if _, exists := ActiveTeamSessions[sessionName]; !exists {
		t.Fatal("Team session was not created")
	}

	// Delete it
	DeleteTeamWindow(sessionName)

	// Verify it's gone
	if _, exists := ActiveTeamSessions[sessionName]; exists {
		t.Error("Team session was not deleted")
	}

	// Deleting non-existent should not panic
	DeleteTeamWindow("non-existent")
}

// TestFindPaneByRole tests finding pane by role in a team session
func TestFindPaneByRole(t *testing.T) {
	runtime := &TeamRuntime{}
	sessionName := "test-find-pane"

	// Create a team window
	state, err := GetOrCreateTeamWindow(sessionName)
	if err != nil {
		t.Fatalf("GetOrCreateTeamWindow() failed: %v", err)
	}

	// Set some pane IDs for testing
	state.Panes[0].PaneID = "%0"
	state.Panes[1].PaneID = "%1"
	state.Panes[2].PaneID = "%2"

	tests := []struct {
		name        string
		role        PaneRole
		wantPaneID  string
		wantErr     bool
	}{
		{
			name:       "planner role",
			role:       RolePlanner,
			wantPaneID: "%0",
			wantErr:    false,
		},
		{
			name:       "executor role",
			role:       RoleExecutor,
			wantPaneID: "%1",
			wantErr:    false,
		},
		{
			name:       "reviewer role",
			role:       RoleReviewer,
			wantPaneID: "%2",
			wantErr:    false,
		},
		{
			name:       "unknown role",
			role:       "unknown",
			wantPaneID: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paneID, err := runtime.FindPaneByRole(sessionName, tt.role)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindPaneByRole() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if paneID != tt.wantPaneID {
				t.Errorf("FindPaneByRole() paneID = %q, want %q", paneID, tt.wantPaneID)
			}
		})
	}

	// Test non-existent session
	_, err = runtime.FindPaneByRole("non-existent", RoleExecutor)
	if err == nil {
		t.Error("FindPaneByRole() with non-existent session should return error")
	}

	// Clean up
	DeleteTeamWindow(sessionName)
}

// TestActiveTeamSessionsConcurrency tests concurrent access to ActiveTeamSessions
func TestActiveTeamSessionsConcurrency(t *testing.T) {
	// Save and restore original state
	teamSessionsMutex.Lock()
	originalSessions := make(map[string]*TeamWindowState)
	for k, v := range ActiveTeamSessions {
		originalSessions[k] = v
		delete(ActiveTeamSessions, k)
	}
	teamSessionsMutex.Unlock()

	defer func() {
		teamSessionsMutex.Lock()
		for k, v := range originalSessions {
			ActiveTeamSessions[k] = v
		}
		for k := range ActiveTeamSessions {
			if _, ok := originalSessions[k]; !ok {
				delete(ActiveTeamSessions, k)
			}
		}
		teamSessionsMutex.Unlock()
	}()

	const numGoroutines = 100
	const numIterations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // 3 types of operations

	// Goroutines that create/get sessions
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				sessionName := "concurrent-test-" + string(rune('A'+id%26))
				GetOrCreateTeamWindow(sessionName)
			}
		}(i)
	}

	// Goroutines that read sessions
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				sessionName := "concurrent-test-" + string(rune('A'+id%26))
				if state, err := GetOrCreateTeamWindow(sessionName); err == nil {
					// Read from state
					_ = state.SessionName
					_ = len(state.Panes)
				}
			}
		}(i)
	}

	// Goroutines that delete sessions
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations/2; j++ { // Fewer deletions
				sessionName := "concurrent-test-" + string(rune('A'+id%26))
				DeleteTeamWindow(sessionName)
			}
		}(i)
	}

	wg.Wait()

	// Verify no data corruption - check that all remaining sessions are valid
	teamSessionsMutex.RLock()
	for name, state := range ActiveTeamSessions {
		if state == nil {
			t.Errorf("Session %q has nil state", name)
			continue
		}
		if state.SessionName != name {
			t.Errorf("Session %q has mismatched name: %q", name, state.SessionName)
		}
		if len(state.Panes) != 3 {
			t.Errorf("Session %q has %d panes, want 3", name, len(state.Panes))
		}
	}
	teamSessionsMutex.RUnlock()
}

// TestTeamWindowState tests TeamWindowState structure
func TestTeamWindowState(t *testing.T) {
	state := &TeamWindowState{
		SessionName: "test-session",
		WindowName:  "test-window",
		Panes: [3]*TeamPaneInfo{
			{PaneID: "%0", Role: RolePlanner},
			{PaneID: "%1", Role: RoleExecutor},
			{PaneID: "%2", Role: RoleReviewer},
		},
		CreateTime: 1234567890,
	}

	if state.SessionName != "test-session" {
		t.Errorf("SessionName = %q, want 'test-session'", state.SessionName)
	}
	if state.WindowName != "test-window" {
		t.Errorf("WindowName = %q, want 'test-window'", state.WindowName)
	}
	if len(state.Panes) != 3 {
		t.Errorf("Panes length = %d, want 3", len(state.Panes))
	}
	if state.Panes[0].Role != RolePlanner {
		t.Errorf("Pane 0 role = %v, want Planner", state.Panes[0].Role)
	}
	if state.CreateTime != 1234567890 {
		t.Errorf("CreateTime = %d, want 1234567890", state.CreateTime)
	}
}

// TestTeamPaneInfo tests TeamPaneInfo structure
func TestTeamPaneInfo(t *testing.T) {
	info := &TeamPaneInfo{
		PaneID:     "%1",
		Role:       RoleExecutor,
		ClaudePID:  12345,
	}

	if info.PaneID != "%1" {
		t.Errorf("PaneID = %q, want '%%1'", info.PaneID)
	}
	if info.Role != RoleExecutor {
		t.Errorf("Role = %v, want Executor", info.Role)
	}
	if info.ClaudePID != 12345 {
		t.Errorf("ClaudePID = %d, want 12345", info.ClaudePID)
	}
}

// TestRoleToIndexMapping tests the role to pane index mapping
func TestRoleToIndexMapping(t *testing.T) {
	runtime := &TeamRuntime{}
	sess := &mockSessionForRuntime{name: "test"}

	tests := []struct {
		role         PaneRole
		wantPaneNum  int // tmux 0-based pane number
	}{
		{RolePlanner, 0},
		{RoleExecutor, 1},
		{RoleReviewer, 2},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			target, err := runtime.GetRoleTarget(sess, tt.role)
			if err != nil {
				t.Errorf("GetRoleTarget(%v) error = %v", tt.role, err)
				return
			}

			// Verify target format: "ccc-team:session.{paneNum}"
			wantTarget := fmt.Sprintf("ccc-team:test.%d", tt.wantPaneNum)
			if target != wantTarget {
				t.Errorf("GetRoleTarget(%v) = %q, want %q", tt.role, target, wantTarget)
			}
		})
	}
}

// TestEmptySessionName tests handling of empty session names
func TestEmptySessionName(t *testing.T) {
	runtime := &TeamRuntime{}
	sess := &mockSessionForRuntime{name: ""}

	target, err := runtime.GetRoleTarget(sess, RoleExecutor)
	if err != nil {
		t.Errorf("GetRoleTarget() with empty session name should not error, got %v", err)
		return
	}

	// Should still produce a valid target even with empty name
	wantTarget := "ccc-team:.1"
	if target != wantTarget {
		t.Errorf("GetRoleTarget() with empty name = %q, want %q", target, wantTarget)
	}
}
