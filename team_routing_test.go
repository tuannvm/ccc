package main

import (
	"testing"

	"github.com/tuannvm/ccc/session"
)

// TestIsBuiltinCommand tests the built-in command detection
func TestIsBuiltinCommand(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "/stop command", text: "/stop", want: true},
		{name: "/delete command", text: "/delete", want: true},
		{name: "/resume command", text: "/resume", want: true},
		{name: "/providers command", text: "/providers", want: true},
		{name: "/provider command", text: "/provider", want: true},
		{name: "/new command", text: "/new", want: true},
		{name: "/worktree command", text: "/worktree", want: true},
		{name: "/team command", text: "/team", want: true},
		{name: "/cleanup command", text: "/cleanup", want: true},
		{name: "/c command", text: "/c", want: true},
		{name: "/stats command", text: "/stats", want: true},
		{name: "/update command", text: "/update", want: true},
		{name: "/version command", text: "/version", want: true},
		{name: "/auth command", text: "/auth", want: true},
		{name: "/restart command", text: "/restart", want: true},
		{name: "/continue command", text: "/continue", want: true},
		{name: "/stop with space", text: "/stop now", want: true},
		{name: "/team list subcommand", text: "/team list", want: true},
		{name: "/new project-name", text: "/new my-project", want: true},
		{name: "case insensitive /STOP", text: "/STOP", want: true},
		{name: "case insensitive /Team", text: "/Team list", want: true},
		{name: "user message with /planner prefix", text: "/planner create plan", want: false},
		{name: "user message with @executor mention", text: "@executor run tests", want: false},
		{name: "plain text message", text: "hello world", want: false},
		{name: "empty string", text: "", want: false},
		{name: "whitespace only", text: "   ", want: false},
		{name: "leading whitespace before command", text: "   /stop", want: true},
		{name: "unknown command", text: "/unknown", want: false},
		{name: "unknown with space", text: "/unknown command", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBuiltinCommand(tt.text)
			if got != tt.want {
				t.Errorf("isBuiltinCommand(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// TestGetTeamRoleTarget tests role-to-target mapping for team sessions
func TestGetTeamRoleTarget(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		role        session.PaneRole
		wantTarget  string
		wantErr     bool
	}{
		{
			name:        "planner role",
			sessionName: "test-session",
			role:        session.RolePlanner,
			wantTarget:  "ccc-team:test-session.1",
			wantErr:     false,
		},
		{
			name:        "executor role",
			sessionName: "test-session",
			role:        session.RoleExecutor,
			wantTarget:  "ccc-team:test-session.2",
			wantErr:     false,
		},
		{
			name:        "reviewer role",
			sessionName: "test-session",
			role:        session.RoleReviewer,
			wantTarget:  "ccc-team:test-session.3",
			wantErr:     false,
		},
		{
			name:        "unknown role",
			sessionName: "test-session",
			role:        "unknown",
			wantTarget:  "",
			wantErr:     true,
		},
		{
			name:        "empty session name",
			sessionName: "",
			role:        session.RolePlanner,
			wantTarget:  "",
			wantErr:     true,
		},
		{
			name:        "session name with dots gets sanitized",
			sessionName: "test.session.name",
			role:        session.RoleExecutor,
			wantTarget:  "ccc-team:test__session__name.2",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getTeamRoleTarget(tt.sessionName, tt.role)
			if (err != nil) != tt.wantErr {
				t.Errorf("getTeamRoleTarget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantTarget {
				t.Errorf("getTeamRoleTarget() = %q, want %q", got, tt.wantTarget)
			}
		})
	}
}

// TestGetSessionNameFromInfo tests session name extraction from SessionInfo
func TestGetSessionNameFromInfo(t *testing.T) {
	tests := []struct {
		name string
		info *SessionInfo
		want string
	}{
		{
			name: "SessionName field takes priority",
			info: &SessionInfo{
				SessionName: "my-session",
				Path:        "/home/user/other-path",
			},
			want: "my-session",
		},
		{
			name: "fallback to path basename when SessionName empty",
			info: &SessionInfo{
				SessionName: "",
				Path:        "/home/user/project-dir",
			},
			want: "project-dir",
		},
		{
			name: "whitespace SessionName is not trimmed",
			info: &SessionInfo{
				SessionName: "   ",
				Path:        "/home/user/project-dir",
			},
			want: "   ", // SessionName is not trimmed, whitespace is returned as-is
		},
		{
			name: "path with trailing slash",
			info: &SessionInfo{
				SessionName: "",
				Path:        "/home/user/project-dir/",
			},
			want: "", // basename of "/foo/" is ""
		},
		{
			name: "empty path",
			info: &SessionInfo{
				SessionName: "",
				Path:        "",
			},
			want: "",
		},
		{
			name: "nested path",
			info: &SessionInfo{
				SessionName: "",
				Path:        "/home/user/projects/my-app/backend",
			},
			want: "backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSessionNameFromInfo(tt.info)
			if got != tt.want {
				t.Errorf("getSessionNameFromInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHandleTeamSessionMessage_NonTeamSession tests that non-team sessions return false
func TestHandleTeamSessionMessage_NonTeamSession(t *testing.T) {
	config := &Config{
		TeamSessions: map[int64]*SessionInfo{},
		Sessions: map[string]*SessionInfo{
			"regular": {TopicID: 100, Path: "/home/user/regular"},
		},
	}

	// Non-team topic ID should return false
	handled := handleTeamSessionMessage(config, "hello", 100, 123, 0)
	if handled {
		t.Error("handleTeamSessionMessage() = true for non-team session, want false")
	}
}

// TestHandleTeamSessionMessage_NilSessionInfo tests handling of nil session info in map
func TestHandleTeamSessionMessage_NilSessionInfo(t *testing.T) {
	config := &Config{
		// Topic exists in map but points to nil
		TeamSessions: map[int64]*SessionInfo{
			999: nil,
		},
	}

	// Nil session info should return true (handled with error message)
	handled := handleTeamSessionMessage(config, "hello", 999, 123, 0)
	if !handled {
		t.Error("handleTeamSessionMessage() with nil session info should return true")
	}
}

// Note: Tests for handleTeamSessionMessage with actual team sessions are integration tests
// that require tmux to be running. The pure functions (isBuiltinCommand, getTeamRoleTarget,
// getSessionNameFromInfo) are tested above. Integration tests should be run separately
// in an environment with tmux available.
