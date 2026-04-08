package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// TestGetSessionByTopic tests the getSessionByTopic function
func TestGetSessionByTopic(t *testing.T) {
	config := &Config{
		Sessions: map[string]*SessionInfo{
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
			result := getSessionByTopic(config, tt.topicID)
			if result != tt.expected {
				t.Errorf("getSessionByTopic(config, %d) = %q, want %q", tt.topicID, result, tt.expected)
			}
		})
	}
}

// TestGetSessionByTopicNilSessions tests with nil sessions map
func TestGetSessionByTopicNilSessions(t *testing.T) {
	config := &Config{
		Sessions: nil,
	}
	result := getSessionByTopic(config, 100)
	if result != "" {
		t.Errorf("getSessionByTopic with nil sessions = %q, want empty string", result)
	}
}

// TestEmptySessionsMap tests behavior with empty sessions
func TestEmptySessionsMap(t *testing.T) {
	config := &Config{
		Sessions: make(map[string]*SessionInfo),
	}

	result := getSessionByTopic(config, 100)
	if result != "" {
		t.Errorf("getSessionByTopic with empty sessions = %q, want empty", result)
	}
}

// TestBaselineProviderResolution tests provider lookup functions
func TestBaselineProviderResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		ActiveProvider: "custom-provider",
		Providers: map[string]*ProviderConfig{
			"anthropic": {
				BaseURL:    "https://api.anthropic.com",
				AuthEnvVar: "ANTHROPIC_API_KEY",
			},
			"custom-provider": {
				BaseURL:     "https://custom.ai",
				AuthToken:   "custom-key",
				SonnetModel: "custom-sonnet",
			},
		},
	}

	// Test getActiveProvider - it returns *ProviderConfig, not Provider interface
	active := getActiveProvider(config)
	if active == nil {
		t.Error("getActiveProvider returned nil")
	} else if active.BaseURL != "https://custom.ai" {
		t.Errorf("getActiveProvider().BaseURL: got %q, want 'https://custom.ai'", active.BaseURL)
	}

	// Test getProvider with specific name
	provider := getProvider(config, "anthropic")
	if provider == nil {
		t.Error("getProvider('anthropic') returned nil")
	} else if provider.Name() != "anthropic" {
		t.Errorf("getProvider('anthropic').Name(): got %q, want 'anthropic'", provider.Name())
	}

	// Test getProvider with empty string (should return active)
	provider = getProvider(config, "")
	if provider == nil {
		t.Error("getProvider('') returned nil")
	} else if provider.Name() != "custom-provider" {
		t.Errorf("getProvider('').Name(): got %q, want 'custom-provider'", provider.Name())
	}

	// Test getProviderNames
	names := getProviderNames(config)
	if len(names) != 2 {
		t.Errorf("getProviderNames length: got %d, want 2", len(names))
	}
	// Check anthropic is always included
	if !slices.Contains(names, "anthropic") {
		t.Error("'anthropic' not in provider names (should always be included)")
	}
}

// TestBaselineSessionLookup tests session lookup functions
func TestBaselineSessionLookup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		GroupID: 12345,
		Sessions: map[string]*SessionInfo{
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
	sessName := getSessionByTopic(config, 1001)
	if sessName != "project1" {
		t.Errorf("getSessionByTopic(1001): got %q, want 'project1'", sessName)
	}

	sessName = getSessionByTopic(config, 9999)
	if sessName != "" {
		t.Errorf("getSessionByTopic(9999): got %q, want empty", sessName)
	}

	// Test findSessionByClaudeID
	sessName, topicID := findSessionByClaudeID(config, "claude-session-1")
	if sessName != "project1" {
		t.Errorf("findSessionByClaudeID('claude-session-1'): got %q, want 'project1'", sessName)
	}
	if topicID != 1001 {
		t.Errorf("findSessionByClaudeID topicID: got %d, want 1001", topicID)
	}

	// Test findSessionByCwd
	sessName, topicID = findSessionByCwd(config, "/home/user/project1")
	if sessName != "project1" {
		t.Errorf("findSessionByCwd('/home/user/project1'): got %q, want 'project1'", sessName)
	}

	// Test non-existent path
	sessName, topicID = findSessionByCwd(config, "/nonexistent")
	if sessName != "" {
		t.Errorf("findSessionByCwd('/nonexistent'): got %q, want empty", sessName)
	}

	// Test findSession (combined lookup)
	sessName, topicID = findSession(config, "/home/user/project1", "")
	if sessName != "project1" {
		t.Errorf("findSession with claudeID='': got %q, want 'project1'", sessName)
	}

	sessName, topicID = findSession(config, "", "claude-session-2")
	if sessName != "project2" {
		t.Errorf("findSession with cwd='': got %q, want 'project2'", sessName)
	}
}

// TestBaselinePathUtilities tests path resolution functions
func TestBaselinePathUtilities(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		ProjectsDir: "~/CustomProjects",
	}

	// Test getProjectsDir
	projectsDir := configpkg.GetProjectsDir(config)
	expected := filepath.Join(tmpDir, "CustomProjects")
	if projectsDir != expected {
		t.Errorf("getProjectsDir: got %q, want %q", projectsDir, expected)
	}

	// Test resolveProjectPath with absolute path
	path := configpkg.ResolveProjectPath(config, "/absolute/path")
	if path != "/absolute/path" {
		t.Errorf("configpkg.ResolveProjectPath(absolute): got %q, want '/absolute/path'", path)
	}

	// Test resolveProjectPath with home-relative path
	path = configpkg.ResolveProjectPath(config, "~/from/home")
	expected = filepath.Join(tmpDir, "from/home")
	if path != expected {
		t.Errorf("configpkg.ResolveProjectPath(~/from/home): got %q, want %q", path, expected)
	}

	// Test resolveProjectPath with relative path
	path = configpkg.ResolveProjectPath(config, "relative/path")
	expected = filepath.Join(tmpDir, "CustomProjects", "relative/path")
	if path != expected {
		t.Errorf("configpkg.ResolveProjectPath(relative): got %q, want %q", path, expected)
	}

	// Test expandPath
	path = expandPath("~/test/path")
	expected = filepath.Join(tmpDir, "test/path")
	if path != expected {
		t.Errorf("expandPath(~/test/path): got %q, want %q", path, expected)
	}

	// Test expandPath with non-tilde path
	path = expandPath("/absolute/path")
	if path != "/absolute/path" {
		t.Errorf("expandPath(absolute): got %q, want '/absolute/path'", path)
	}
}
