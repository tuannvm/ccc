package main

import (
	"os"
	"path/filepath"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// TestConfigSaveLoad tests saving and loading config
func TestConfigSaveLoad(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override config path for test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test config
	config := &Config{
		BotToken: "test-token-123",
		ChatID:   12345,
		GroupID:  -67890,
		Sessions: map[string]*SessionInfo{
			"project1":   {TopicID: 100, Path: "/home/user/project1"},
			"money/shop": {TopicID: 200, Path: "/home/user/money/shop"},
		},
		Away: true,
	}

	// Save config
	if err := configpkg.Save(config); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load config
	loaded, err := configpkg.Load()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Verify loaded config matches
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
	if len(loaded.Sessions) != len(config.Sessions) {
		t.Errorf("Sessions length = %d, want %d", len(loaded.Sessions), len(config.Sessions))
	}
	for name, info := range config.Sessions {
		loadedInfo := loaded.Sessions[name]
		if loadedInfo == nil || loadedInfo.TopicID != info.TopicID {
			t.Errorf("Sessions[%q].TopicID mismatch", name)
		}
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

	_, err = configpkg.Load()
	if err == nil {
		t.Error("loadConfig should fail for non-existent file")
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

	// Write config without sessions field
	configPath := filepath.Join(tmpDir, ".ccc.json")
	data := []byte(`{"bot_token": "test", "chat_id": 123}`)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	loaded, err := configpkg.Load()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if loaded.Sessions == nil {
		t.Error("Sessions should be initialized to non-nil map")
	}
}
