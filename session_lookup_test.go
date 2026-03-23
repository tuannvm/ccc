package main

import (
	"testing"

	"github.com/tuannvm/ccc/session"
)

// TestFindSessionByWindowNameExactMatch tests exact window name matching
func TestFindSessionByWindowNameExactMatch(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"project1": {TopicID: 100, Path: "/home/user/project1"},
			"project2": {TopicID: 200, Path: "/home/user/project2"},
		},
		TeamSessions: map[int64]*SessionInfo{
			101: {SessionName: "team1", Path: "/home/user/team1"},
		},
	}

	tests := []struct {
		name         string
		windowName   string
		wantName     string
		wantTopicID  int64
	}{
		{
			name:        "exact match in regular sessions",
			windowName:  "project1",
			wantName:    "project1",
			wantTopicID: 100,
		},
		{
			name:        "exact match in team sessions",
			windowName:  "team1",
			wantName:    "team1",
			wantTopicID: 101,
		},
		{
			name:        "no match returns empty",
			windowName:  "nonexistent",
			wantName:    "",
			wantTopicID: 0,
		},
		{
			name:        "empty window name returns empty",
			windowName:  "",
			wantName:    "",
			wantTopicID: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, topicID := findSessionByWindowName(config, tt.windowName)
			if name != tt.wantName {
				t.Errorf("findSessionByWindowName() name = %q, want %q", name, tt.wantName)
			}
			if topicID != tt.wantTopicID {
				t.Errorf("findSessionByWindowName() topicID = %d, want %d", topicID, tt.wantTopicID)
			}
		})
	}
}

// TestFindSessionByWindowNameSanitizedMatch tests sanitized (tmuxSafeName) matching
func TestFindSessionByWindowNameSanitizedMatch(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"my.project":    {TopicID: 100, Path: "/home/user/my.project"},
			"another.name":  {TopicID: 200, Path: "/home/user/another.name"},
			"normal-name":   {TopicID: 300, Path: "/home/user/normal-name"},
		},
		TeamSessions: map[int64]*SessionInfo{
			101: {SessionName: "team.project", Path: "/home/user/team.project"},
		},
	}

	tests := []struct {
		name         string
		windowName   string
		wantName     string
		wantTopicID  int64
	}{
		{
			name:        "sanitized match in regular sessions (dots to __)",
			windowName:  "my__project",
			wantName:    "my.project",
			wantTopicID: 100,
		},
		{
			name:        "sanitized match in team sessions",
			windowName:  "team__project",
			wantName:    "team.project",
			wantTopicID: 101,
		},
		{
			name:        "exact match takes priority over sanitized",
			windowName:  "normal-name",
			wantName:    "normal-name",
			wantTopicID: 300,
		},
		{
			name:        "no sanitized match returns empty",
			windowName:  "nonexistent__name",
			wantName:    "",
			wantTopicID: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, topicID := findSessionByWindowName(config, tt.windowName)
			if name != tt.wantName {
				t.Errorf("findSessionByWindowName() name = %q, want %q", name, tt.wantName)
			}
			if topicID != tt.wantTopicID {
				t.Errorf("findSessionByWindowName() topicID = %d, want %d", topicID, tt.wantTopicID)
			}
		})
	}
}

// TestFindSessionByWindowNameAmbiguous tests ambiguous match detection
func TestFindSessionByWindowNameAmbiguous(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			// "my.project" and "my.project" would be the same key, so we need a real ambiguous scenario
			// With tmuxSafeName replacing "." with "__", "a.b.c" and "ab.c" produce different results
			// So to test ambiguous matches, we use sessions that could theoretically collide
			// In practice, this tests the code path that checks for duplicates
			"test.one": {TopicID: 100, Path: "/home/user/test.one"},
			"test.two": {TopicID: 200, Path: "/home/user/test.two"},
		},
	}

	// For a sanitized match to be ambiguous, multiple sessions would need to sanitize to the same name
	// With the current tmuxSafeName implementation ("." -> "__"), this is difficult to create
	// So we test a scenario that doesn't trigger ambiguity
	t.Run("non_ambiguous_sanitized_match", func(t *testing.T) {
		// "test.one" -> "test__one"
		name, topicID := findSessionByWindowName(config, "test__one")
		if name != "test.one" {
			t.Errorf("findSessionByWindowName() name = %q, want 'test.one'", name)
		}
		if topicID != 100 {
			t.Errorf("findSessionByWindowName() topicID = %d, want 100", topicID)
		}
	})

	// Test that exact match is never ambiguous
	t.Run("exact_match_is_never_ambiguous", func(t *testing.T) {
		name, topicID := findSessionByWindowName(config, "test.one")
		if name != "test.one" {
			t.Errorf("findSessionByWindowName() name = %q, want 'test.one'", name)
		}
		if topicID != 100 {
			t.Errorf("findSessionByWindowName() topicID = %d, want 100", topicID)
		}
	})
}

// TestFindSessionByClaudeID tests finding sessions by Claude session ID
func TestFindSessionByClaudeID(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"project1": {
				TopicID:         100,
				Path:            "/home/user/project1",
				ClaudeSessionID: "claude-1",
			},
			"project2": {
				TopicID:         200,
				Path:            "/home/user/project2",
				ClaudeSessionID: "claude-2",
			},
		},
		TeamSessions: map[int64]*SessionInfo{
			101: {
				SessionName: "team1",
				Path:        "/home/user/team1",
				Panes: map[session.PaneRole]*PaneInfo{
					session.RolePlanner: {ClaudeSessionID: "claude-3"},
				},
			},
		},
	}

	tests := []struct {
		name           string
		claudeSessionID string
		wantName       string
		wantTopicID    int64
	}{
		{
			name:           "match in regular sessions",
			claudeSessionID: "claude-1",
			wantName:       "project1",
			wantTopicID:    100,
		},
		{
			name:           "match in team sessions (pane)",
			claudeSessionID: "claude-3",
			wantName:       "team1",
			wantTopicID:    101,
		},
		{
			name:           "no match returns empty",
			claudeSessionID: "nonexistent",
			wantName:       "",
			wantTopicID:    0,
		},
		{
			name:           "empty session ID returns empty",
			claudeSessionID: "",
			wantName:       "",
			wantTopicID:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, topicID := findSessionByClaudeID(config, tt.claudeSessionID)
			if name != tt.wantName {
				t.Errorf("findSessionByClaudeID() name = %q, want %q", name, tt.wantName)
			}
			if topicID != tt.wantTopicID {
				t.Errorf("findSessionByClaudeID() topicID = %d, want %d", topicID, tt.wantTopicID)
			}
		})
	}
}

// TestGetSessionByTopicInLookup tests finding sessions by topic ID
// Note: Renamed from TestGetSessionByTopic to avoid duplicate with main_test.go
func TestGetSessionByTopicInLookup(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"project1": {TopicID: 100, Path: "/home/user/project1"},
			"project2": {TopicID: 200, Path: "/home/user/project2"},
		},
		TeamSessions: map[int64]*SessionInfo{
			101: {SessionName: "team1", Path: "/home/user/team1"},
		},
	}

	tests := []struct {
		name        string
		topicID     int64
		wantName    string
	}{
		{
			name:     "match in regular sessions",
			topicID:  100,
			wantName: "project1",
		},
		{
			name:     "match in team sessions",
			topicID:  101,
			wantName: "team1",
		},
		{
			name:     "no match returns empty",
			topicID:  999,
			wantName: "",
		},
		{
			name:     "zero topic ID returns empty",
			topicID:  0,
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := getSessionByTopic(config, tt.topicID)
			if name != tt.wantName {
				t.Errorf("getSessionByTopic() name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

// TestGetSessionByTopicNilMaps tests with nil maps
func TestGetSessionByTopicNilMapsInLookup(t *testing.T) {
	config := &Config{
		Sessions:     nil,
		TeamSessions: nil,
	}

	name := getSessionByTopic(config, 100)
	if name != "" {
		t.Errorf("getSessionByTopic() with nil maps = %q, want empty", name)
	}
}

// TestFindSessionByCwd tests finding sessions by current working directory
func TestFindSessionByCwd(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"project1": {TopicID: 100, Path: "/home/user/project1"},
			"project2": {TopicID: 200, Path: "/home/user/projects/project2"},
		},
		TeamSessions: map[int64]*SessionInfo{
			101: {SessionName: "team1", Path: "/home/user/team1"},
		},
	}

	tests := []struct {
		name        string
		cwd         string
		wantName    string
		wantTopicID int64
	}{
		{
			name:        "exact path match in regular sessions",
			cwd:         "/home/user/project1",
			wantName:    "project1",
			wantTopicID: 100,
		},
		{
			name:        "exact path match in team sessions",
			cwd:         "/home/user/team1",
			wantName:    "team1",
			wantTopicID: 101,
		},
		{
			name:        "subdirectory path match",
			cwd:         "/home/user/project1/subdir",
			wantName:    "project1",
			wantTopicID: 100,
		},
		{
			name:        "no match returns empty",
			cwd:         "/nonexistent/path",
			wantName:    "",
			wantTopicID: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, topicID := findSessionByCwd(config, tt.cwd)
			if name != tt.wantName {
				t.Errorf("findSessionByCwd() name = %q, want %q", name, tt.wantName)
			}
			if topicID != tt.wantTopicID {
				t.Errorf("findSessionByCwd() topicID = %d, want %d", topicID, tt.wantTopicID)
			}
		})
	}
}

// TestFindSessionNilChecks tests nil map handling
func TestFindSessionNilChecks(t *testing.T) {
	config := &Config{
		Sessions:     nil,
		TeamSessions: nil,
	}

	// These should not panic
	name, topicID := findSession(config, "/some/path", "")
	if name != "" {
		t.Errorf("findSession() with nil maps = %q, want empty", name)
	}
	if topicID != 0 {
		t.Errorf("findSession() topicID = %d, want 0", topicID)
	}
}

// TestFindSessionByWindowNameNilSessionsInTeamMap tests nil session info in team sessions map
func TestFindSessionByWindowNameNilSessionsInTeamMap(t *testing.T) {
	config := &Config{
		TeamSessions: map[int64]*SessionInfo{
			101: nil, // nil entry
		},
	}

	name, topicID := findSessionByWindowName(config, "team1")
	if name != "" {
		t.Errorf("findSessionByWindowName() with nil session info = %q, want empty", name)
	}
	if topicID != 0 {
		t.Errorf("findSessionByWindowName() topicID = %d, want 0", topicID)
	}
}

// TestFindSessionPriorityOrder tests the priority order: claudeID > window name > cwd
func TestFindSessionPriorityOrder(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
			"project1": {
				TopicID:         100,
				Path:            "/home/user/project1",
				ClaudeSessionID: "claude-1",
			},
		},
	}

	// When claudeSessionID is provided, it should be used (highest priority)
	name, topicID := findSession(config, "/home/user/project1", "claude-1")
	if name != "project1" {
		t.Errorf("findSession() with claudeID should use claudeID, got %q", name)
	}
	if topicID != 100 {
		t.Errorf("findSession() topicID = %d, want 100", topicID)
	}

	// When claudeSessionID is empty, should fall back to window name/cwd
	name, topicID = findSession(config, "/home/user/project1", "")
	if name != "project1" {
		t.Errorf("findSession() without claudeID should fall back to path matching, got %q", name)
	}
	if topicID != 100 {
		t.Errorf("findSession() topicID = %d, want 100", topicID)
	}
}

// TestFindSessionByWindowNameWithNilConfig tests nil config handling
func TestFindSessionByWindowNameWithNilConfig(t *testing.T) {
	// The implementation does not handle nil config and will panic
	// This test documents that behavior
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("findSessionByWindowName() with nil config should panic, but didn't")
		}
	}()
	findSessionByWindowName(nil, "test")
}

// TestFindSessionByCwdWithNilConfig tests nil config handling
func TestFindSessionByCwdWithNilConfig(t *testing.T) {
	// The implementation does not handle nil config and will panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("findSessionByCwd() with nil config should panic, but didn't")
		}
	}()
	findSessionByCwd(nil, "/some/path")
}

// TestFindSessionByClaudeIDWithNilConfig tests nil config handling
func TestFindSessionByClaudeIDWithNilConfig(t *testing.T) {
	// The implementation does not handle nil config and will panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("findSessionByClaudeID() with nil config should panic, but didn't")
		}
	}()
	findSessionByClaudeID(nil, "claude-id")
}

// TestGetSessionByTopicWithNilConfig tests nil config handling
func TestGetSessionByTopicWithNilConfigInLookup(t *testing.T) {
	// The implementation does not handle nil config and will panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("getSessionByTopic() with nil config should panic, but didn't")
		}
	}()
	getSessionByTopic(nil, 100)
}
