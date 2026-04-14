package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigFilePermissions tests that config is saved with correct permissions
func TestConfigFilePermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		BotToken: "secret-token",
		ChatID:   12345,
		Sessions: make(map[string]*SessionInfo),
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

	for _, configPath := range paths {
		info, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("Failed to stat config file %s: %v", configPath, err)
		}
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("Config file permissions for %s = %o, want 0600", configPath, perm)
		}
	}
}
