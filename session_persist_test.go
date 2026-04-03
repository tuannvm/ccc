package main

import (
	"os"
	"testing"

	"github.com/tuannvm/ccc/session"
)

// TestInferRoleFromTranscriptPath tests role extraction from transcript file paths
func TestInferRoleFromTranscriptPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantRole session.PaneRole
	}{
		{
			name:     "planner with dash separator",
			path:     "/path/to/session-planner.jsonl",
			wantRole: session.RolePlanner,
		},
		{
			name:     "executor with dash separator",
			path:     "/home/user/.ccc/transcripts/my-project-executor.jsonl",
			wantRole: session.RoleExecutor,
		},
		{
			name:     "reviewer with dash separator",
			path:     "/var/data/team-session-reviewer.jsonl",
			wantRole: session.RoleReviewer,
		},
		{
			name:     "planner with underscore separator",
			path:     "/path/to/session_planner.jsonl",
			wantRole: session.RolePlanner,
		},
		{
			name:     "planner with dot separator",
			path:     "/path/to/session.planner.jsonl",
			wantRole: session.RolePlanner,
		},
		{
			name:     "plain role name as filename",
			path:     "/path/to/planner.jsonl",
			wantRole: session.RolePlanner,
		},
		{
			name:     "plain role name as filename with .json extension",
			path:     "/path/to/executor.json",
			wantRole: session.RoleExecutor,
		},
		{
			name:     "double extension .json.jsonl",
			path:     "/path/to/session-reviewer.json.jsonl",
			wantRole: session.RoleReviewer,
		},
		{
			name:     "no role in filename",
			path:     "/path/to/session.jsonl",
			wantRole: "",
		},
		{
			name:     "empty path",
			path:     "",
			wantRole: "",
		},
		{
			name:     "role in directory name (not matched)",
			path:     "/planner/sessions/transcript.jsonl",
			wantRole: "",
		},
		{
			name:     "case insensitive - uppercase",
			path:     "/path/to/session-PLANNER.jsonl",
			wantRole: session.RolePlanner,
		},
		{
			name:     "case insensitive - mixed case",
			path:     "/path/to/session-ExEcUtOr.jsonl",
			wantRole: session.RoleExecutor,
		},
		{
			name:     "short alias 'plan' not matched",
			path:     "/path/to/session-plan.jsonl",
			wantRole: "",
		},
		{
			name:     "short alias 'exec' not matched",
			path:     "/path/to/session-exec.jsonl",
			wantRole: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferRoleFromTranscriptPath(tt.path)
			if got != tt.wantRole {
				t.Errorf("inferRoleFromTranscriptPath(%q) = %q, want %q", tt.path, got, tt.wantRole)
			}
		})
	}
}

// TestPersistClaudeSessionID_SingleSession tests persisting Claude session ID for single sessions
func TestPersistClaudeSessionID_SingleSession(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-persist-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		Sessions: map[string]*SessionInfo{
			"test-session": {TopicID: 100, Path: "/home/user/test", ClaudeSessionID: ""},
		},
	}

	// Persist a new Claude session ID
	persistClaudeSessionID(config, "test-session", "claude-abc123", "/path/to/session.jsonl")

	if config.Sessions["test-session"].ClaudeSessionID != "claude-abc123" {
		t.Errorf("ClaudeSessionID = %q, want %q", config.Sessions["test-session"].ClaudeSessionID, "claude-abc123")
	}
}

// TestPersistClaudeSessionID_EmptyInputs tests that empty inputs are ignored
func TestPersistClaudeSessionID_EmptyInputs(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"test-session": {TopicID: 100, ClaudeSessionID: "existing-id"},
		},
	}

	tests := []struct {
		name            string
		sessName        string
		claudeSessionID string
	}{
		{name: "empty session name", sessName: "", claudeSessionID: "new-id"},
		{name: "empty claude session ID", sessName: "test-session", claudeSessionID: ""},
		{name: "both empty", sessName: "", claudeSessionID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			persistClaudeSessionID(config, tt.sessName, tt.claudeSessionID, "/path/to/session.jsonl")
			// Should not change existing value
			if config.Sessions["test-session"].ClaudeSessionID != "existing-id" {
				t.Errorf("ClaudeSessionID changed unexpectedly = %q", config.Sessions["test-session"].ClaudeSessionID)
			}
		})
	}
}

// TestPersistClaudeSessionID_SingleClearsDuplicate tests that duplicates are cleared in other sessions
func TestPersistClaudeSessionID_SingleClearsDuplicate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-persist-dup-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		Sessions: map[string]*SessionInfo{
			"session-a": {TopicID: 100, ClaudeSessionID: "claude-shared-id"},
			"session-b": {TopicID: 200, ClaudeSessionID: ""},
		},
	}

	// Persist the same ID to session-b — should clear it from session-a
	persistClaudeSessionID(config, "session-b", "claude-shared-id", "/path/to/session.jsonl")

	if config.Sessions["session-b"].ClaudeSessionID != "claude-shared-id" {
		t.Errorf("session-b ClaudeSessionID = %q, want %q", config.Sessions["session-b"].ClaudeSessionID, "claude-shared-id")
	}
	if config.Sessions["session-a"].ClaudeSessionID != "" {
		t.Errorf("session-a ClaudeSessionID = %q, want empty (duplicate cleared)", config.Sessions["session-a"].ClaudeSessionID)
	}
}

// TestPersistClaudeSessionID_TeamSession tests persisting Claude session ID for team sessions
func TestPersistClaudeSessionID_TeamSession(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-persist-team-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		TeamSessions: map[int64]*SessionInfo{
			300: {
				SessionName: "team-test",
				TopicID:     300,
				Type:        session.SessionKindTeam,
				Panes: map[session.PaneRole]*PaneInfo{
					session.RolePlanner:  {Role: session.RolePlanner, ClaudeSessionID: ""},
					session.RoleExecutor: {Role: session.RoleExecutor, ClaudeSessionID: ""},
					session.RoleReviewer: {Role: session.RoleReviewer, ClaudeSessionID: ""},
				},
			},
		},
	}

	// Persist for planner role via transcript path
	persistClaudeSessionID(config, "team-test", "claude-planner-123", "/path/to/session-planner.jsonl")

	if config.TeamSessions[300].Panes[session.RolePlanner].ClaudeSessionID != "claude-planner-123" {
		t.Errorf("planner ClaudeSessionID = %q, want %q",
			config.TeamSessions[300].Panes[session.RolePlanner].ClaudeSessionID, "claude-planner-123")
	}
	// Other panes should be unchanged
	if config.TeamSessions[300].Panes[session.RoleExecutor].ClaudeSessionID != "" {
		t.Errorf("executor ClaudeSessionID should be empty, got %q",
			config.TeamSessions[300].Panes[session.RoleExecutor].ClaudeSessionID)
	}
}

// TestPersistClaudeSessionID_TeamClearsSiblingPane tests that duplicates are cleared from sibling panes
func TestPersistClaudeSessionID_TeamClearsSiblingPane(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-persist-sibling-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		TeamSessions: map[int64]*SessionInfo{
			400: {
				SessionName: "team-sibling",
				TopicID:     400,
				Type:        session.SessionKindTeam,
				Panes: map[session.PaneRole]*PaneInfo{
					session.RolePlanner:  {Role: session.RolePlanner, ClaudeSessionID: "claude-shared"},
					session.RoleExecutor: {Role: session.RoleExecutor, ClaudeSessionID: ""},
					session.RoleReviewer: {Role: session.RoleReviewer, ClaudeSessionID: ""},
				},
			},
		},
	}

	// Persist the same ID to executor — should clear from planner
	persistClaudeSessionID(config, "team-sibling", "claude-shared", "/path/to/session-executor.jsonl")

	if config.TeamSessions[400].Panes[session.RoleExecutor].ClaudeSessionID != "claude-shared" {
		t.Errorf("executor ClaudeSessionID = %q, want %q",
			config.TeamSessions[400].Panes[session.RoleExecutor].ClaudeSessionID, "claude-shared")
	}
	if config.TeamSessions[400].Panes[session.RolePlanner].ClaudeSessionID != "" {
		t.Errorf("planner ClaudeSessionID = %q, want empty (sibling cleared)",
			config.TeamSessions[400].Panes[session.RolePlanner].ClaudeSessionID)
	}
}

// TestPersistClaudeSessionID_SessionNotFound tests that unknown session names are safely ignored
func TestPersistClaudeSessionID_SessionNotFound(t *testing.T) {
	config := &Config{
		Sessions:     map[string]*SessionInfo{},
		TeamSessions: map[int64]*SessionInfo{},
	}

	// Should not panic
	persistClaudeSessionID(config, "nonexistent", "claude-123", "/path/to/session.jsonl")
}

// TestPersistClaudeSessionID_Idempotent tests that persisting the same ID is idempotent
func TestPersistClaudeSessionID_Idempotent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-persist-idem-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		Sessions: map[string]*SessionInfo{
			"test": {TopicID: 100, ClaudeSessionID: "claude-abc"},
		},
	}

	// Persist the same ID again
	persistClaudeSessionID(config, "test", "claude-abc", "/path/to/session.jsonl")

	if config.Sessions["test"].ClaudeSessionID != "claude-abc" {
		t.Errorf("ClaudeSessionID = %q, want %q", config.Sessions["test"].ClaudeSessionID, "claude-abc")
	}
}
