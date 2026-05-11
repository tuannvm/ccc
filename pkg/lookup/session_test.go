package lookup

import (
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// TestGetSessionByTopic tests the GetSessionByTopic function
func TestGetSessionByTopic(t *testing.T) {
	config := &configpkg.Config{
		Sessions: map[string]*configpkg.SessionInfo{
			"project1":   {TopicID: 100, Path: "/home/user/project1"},
			"project2":   {TopicID: 200, Path: "/home/user/project2"},
			"money/shop": {TopicID: 300, Path: "/home/user/money/shop"},
		},
	}

	tests := []struct {
		name     string
		topicID  int64
		expected string
	}{
		{"existing topic", 100, "project1"},
		{"another existing", 200, "project2"},
		{"nested path", 300, "money/shop"},
		{"non-existent", 999, ""},
		{"zero", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetSessionByTopic(config, tt.topicID)
			if result != tt.expected {
				t.Errorf("GetSessionByTopic(config, %d) = %q, want %q", tt.topicID, result, tt.expected)
			}
		})
	}
}

// TestGetSessionByTopicNilSessions tests with nil sessions map
func TestGetSessionByTopicNilSessions(t *testing.T) {
	config := &configpkg.Config{
		Sessions: nil,
	}
	result := GetSessionByTopic(config, 100)
	if result != "" {
		t.Errorf("GetSessionByTopic with nil sessions = %q, want empty string", result)
	}
}

// TestEmptySessionsMap tests behavior with empty sessions
func TestEmptySessionsMap(t *testing.T) {
	config := &configpkg.Config{
		Sessions: make(map[string]*configpkg.SessionInfo),
	}

	result := GetSessionByTopic(config, 100)
	if result != "" {
		t.Errorf("GetSessionByTopic with empty sessions = %q, want empty", result)
	}
}

// TestFindSession tests combined session lookup
func TestFindSession(t *testing.T) {
	config := &configpkg.Config{
		GroupID: 12345,
		Sessions: map[string]*configpkg.SessionInfo{
			"project1": {
				TopicID:         1001,
				Path:            "/home/user/project1",
				ClaudeSessionID: "claude-session-1",
				ProviderName:    "provider1",
			},
			"project2": {
				TopicID:         1002,
				Path:            "/home/user/project2",
				ClaudeSessionID: "claude-session-2",
			},
		},
	}

	// Test getSessionByTopic
	sessName := GetSessionByTopic(config, 1001)
	if sessName != "project1" {
		t.Errorf("GetSessionByTopic(1001): got %q, want 'project1'", sessName)
	}

	sessName = GetSessionByTopic(config, 9999)
	if sessName != "" {
		t.Errorf("GetSessionByTopic(9999): got %q, want empty", sessName)
	}

	// Test findSessionByClaudeID
	sessName, topicID := FindSessionByClaudeID(config, "claude-session-1")
	if sessName != "project1" {
		t.Errorf("FindSessionByClaudeID('claude-session-1'): got %q, want 'project1'", sessName)
	}
	if topicID != 1001 {
		t.Errorf("FindSessionByClaudeID topicID: got %d, want 1001", topicID)
	}

	// Test findSessionByCwd
	sessName, topicID = FindSessionByCwd(config, "/home/user/project1")
	if sessName != "project1" {
		t.Errorf("FindSessionByCwd('/home/user/project1'): got %q, want 'project1'", sessName)
	}

	// Test non-existent path
	sessName, topicID = FindSessionByCwd(config, "/nonexistent")
	if sessName != "" {
		t.Errorf("FindSessionByCwd('/nonexistent'): got %q, want empty", sessName)
	}

	// Test findSession (combined lookup)
	sessName, topicID = FindSession(config, "/home/user/project1", "")
	if sessName != "project1" {
		t.Errorf("FindSession with claudeID='': got %q, want 'project1'", sessName)
	}

	sessName, topicID = FindSession(config, "", "claude-session-2")
	if sessName != "project2" {
		t.Errorf("FindSession with cwd='': got %q, want 'project2'", sessName)
	}
}

func TestFindSessionPrefersCwdBeforeSelectedWindowFallback(t *testing.T) {
	config := &configpkg.Config{
		Sessions: map[string]*configpkg.SessionInfo{
			"selected-window": {
				TopicID: 1001,
				Path:    "/home/user/selected-window",
			},
			"hook-cwd": {
				TopicID: 1002,
				Path:    "/home/user/hook-cwd",
			},
		},
	}

	sessName, topicID := FindSession(config, "/home/user/hook-cwd", "new-codex-session-id")
	if sessName != "hook-cwd" {
		t.Fatalf("FindSession should prefer hook cwd before tmux window fallback: got %q", sessName)
	}
	if topicID != 1002 {
		t.Fatalf("FindSession topicID = %d, want 1002", topicID)
	}
}
