package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create dir for %s: %v", path, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("Failed to write %s: %v", path, err)
	}
}

// TestConfigSaveLoad tests saving and loading config
func TestConfigSaveLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		BotToken: "test-token-123",
		ChatID:   12345,
		GroupID:  -67890,
		Sessions: map[string]*SessionInfo{
			"project1":   {TopicID: 100, Path: "/home/user/project1"},
			"money/shop": {TopicID: 200, Path: "/home/user/money/shop"},
		},
		Away:           true,
		ActiveProvider: "openai",
		Providers: map[string]*ProviderConfig{
			"openai": {AuthToken: "sk-test", BaseURL: "https://example.com"},
		},
	}

	if err := Save(config); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	paths := []string{
		filepath.Join(tmpDir, ".config", "ccc", "config.json"),
		filepath.Join(tmpDir, ".config", "ccc", "config.core.json"),
		filepath.Join(tmpDir, ".config", "ccc", "config.sessions.json"),
		filepath.Join(tmpDir, ".config", "ccc", "config.providers.json"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Fatalf("Config file was not created: %s", p)
		}
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.BotToken != config.BotToken {
		t.Errorf("BotToken = %q, want %q", loaded.BotToken, config.BotToken)
	}
	if loaded.ChatID != config.ChatID {
		t.Errorf("ChatID = %d, want %d", loaded.ChatID, config.ChatID)
	}
	if loaded.GroupID != config.GroupID {
		t.Errorf("GroupID = %d, want %d", loaded.GroupID, config.GroupID)
	}
	if loaded.Away != config.Away {
		t.Errorf("Away = %v, want %v", loaded.Away, config.Away)
	}
	if loaded.ActiveProvider != config.ActiveProvider {
		t.Errorf("ActiveProvider = %q, want %q", loaded.ActiveProvider, config.ActiveProvider)
	}
	if len(loaded.Sessions) != len(config.Sessions) {
		t.Errorf("Sessions length = %d, want %d", len(loaded.Sessions), len(config.Sessions))
	}
}

// TestConfigLoadNonExistent tests loading non-existent config
func TestConfigLoadNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	_, err = Load()
	if err == nil {
		t.Error("Load should fail for non-existent file")
	}
}

func TestValidateRejectsReservedCodexProvider(t *testing.T) {
	cfg := &Config{
		Providers: map[string]*ProviderConfig{
			"codex": {AuthToken: "sk-test"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("Validate() error = nil, want reserved provider error")
	}
	if !strings.Contains(err.Error(), "reserved") || !strings.Contains(err.Error(), "Codex") {
		t.Fatalf("Validate() error = %q, want reserved Codex provider error", err.Error())
	}
}

// TestConfigSessionsInitialized tests that Sessions map is initialized on load
func TestConfigSessionsInitialized(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configPath := filepath.Join(tmpDir, ".ccc.json")
	data := []byte(`{"bot_token": "test", "chat_id": 123}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Sessions == nil {
		t.Error("Sessions should be initialized to non-nil map")
	}
}

func TestConfigLoadSplitOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-split-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	baseDir := filepath.Join(tmpDir, ".config", "ccc")
	writeJSONFile(t, filepath.Join(baseDir, "config.core.json"), coreConfig{
		BotToken:        "split-token",
		ChatID:          42,
		ProjectsDir:     "/tmp/projects",
		EnableStreaming: true,
	})
	writeJSONFile(t, filepath.Join(baseDir, "config.sessions.json"), sessionsConfig{
		Sessions: map[string]*SessionInfo{
			"split": {TopicID: 9, Path: "/tmp/split"},
		},
		TeamSessions: map[int64]*SessionInfo{
			100: {TopicID: 100, Path: "/tmp/team"},
		},
	})
	writeJSONFile(t, filepath.Join(baseDir, "config.providers.json"), providersConfig{
		ActiveProvider: "openai",
		Providers: map[string]*ProviderConfig{
			"openai": {AuthToken: "sk-abc", BaseURL: "https://example.com"},
		},
	})

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.BotToken != "split-token" {
		t.Errorf("BotToken = %q, want split-token", loaded.BotToken)
	}
	if loaded.ChatID != 42 {
		t.Errorf("ChatID = %d, want 42", loaded.ChatID)
	}
	if loaded.ActiveProvider != "openai" {
		t.Errorf("ActiveProvider = %q, want openai", loaded.ActiveProvider)
	}
	if loaded.Sessions["split"] == nil {
		t.Fatalf("Expected split session to be present")
	}
}

func TestConfigLoadSplitOverridesLegacy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-precedence-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	baseDir := filepath.Join(tmpDir, ".config", "ccc")
	writeJSONFile(t, filepath.Join(baseDir, "config.json"), Config{
		BotToken: "legacy-token",
		ChatID:   111,
		Sessions: map[string]*SessionInfo{
			"legacy": {TopicID: 1, Path: "/tmp/legacy"},
		},
		ActiveProvider: "anthropic",
	})
	writeJSONFile(t, filepath.Join(baseDir, "config.core.json"), coreConfig{
		BotToken: "split-token",
		ChatID:   222,
	})
	writeJSONFile(t, filepath.Join(baseDir, "config.sessions.json"), sessionsConfig{
		Sessions: map[string]*SessionInfo{
			"split": {TopicID: 2, Path: "/tmp/split"},
		},
	})

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.BotToken != "split-token" {
		t.Errorf("BotToken = %q, want split-token", loaded.BotToken)
	}
	if loaded.ChatID != 222 {
		t.Errorf("ChatID = %d, want 222", loaded.ChatID)
	}
	if loaded.Sessions["legacy"] != nil {
		t.Errorf("Expected legacy sessions to be replaced by split sessions")
	}
	if loaded.Sessions["split"] == nil {
		t.Errorf("Expected split sessions to be loaded")
	}
}
