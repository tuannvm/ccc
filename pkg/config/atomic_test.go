package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestAtomicSaveConfigConcurrent tests that concurrent writes don't corrupt config
func TestAtomicSaveConfigConcurrent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-concurrent-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")

	// Create initial config
	config := &Config{
		BotToken: "test-token",
		ChatID:   12345,
		Sessions: make(map[string]*SessionInfo),
	}
	if err := Save(config); err != nil {
		t.Fatalf("Initial save failed: %v", err)
	}

	// Spawn 100 goroutines, each writing different data
	const numGoroutines = 100
	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)

	for i := range numGoroutines {
		wg.Add(1)
		go func(sessionNum int) {
			defer wg.Done()
			cfg := &Config{
				BotToken: "test-token",
				ChatID:   12345,
				Sessions: map[string]*SessionInfo{
					fmt.Sprintf("session-%d", sessionNum): {
						TopicID: int64(sessionNum),
						Path:    fmt.Sprintf("/path/session-%d", sessionNum),
					},
				},
			}
			if err := Save(cfg); err != nil {
				errCh <- fmt.Errorf("goroutine %d: %w", sessionNum, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		t.Fatalf("Concurrent writes had %d errors: %v", len(errors), errors[0])
	}

	// Verify config is valid JSON
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read final config: %v", err)
	}

	var finalConfig Config
	if err := json.Unmarshal(data, &finalConfig); err != nil {
		t.Fatalf("Final config is not valid JSON: %v", err)
	}

	// All sessions should be present (or one of the last writes won)
	if finalConfig.Sessions == nil {
		t.Error("Sessions map is nil after concurrent writes")
	}

	// File permissions should be 0600
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("Config permissions = %o, want 0600", perm)
	}

	// Verify no temp files left
	tmpFiles, _ := filepath.Glob(filepath.Join(tmpDir, ".config", "ccc", "config-*.tmp"))
	if len(tmpFiles) > 0 {
		t.Errorf("Found %d temp files left behind: %v", len(tmpFiles), tmpFiles)
	}
}

// TestAtomicSaveConfigPreservesOriginal tests that original config is preserved on failure
func TestAtomicSaveConfigPreservesOriginal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-preserve-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")

	// Write initial valid config
	initialConfig := &Config{
		BotToken: "original-token",
		ChatID:   11111,
		GroupID:  22222,
		Sessions: map[string]*SessionInfo{
			"original": {TopicID: 100, Path: "/original/path"},
		},
	}
	if err := Save(initialConfig); err != nil {
		t.Fatalf("Initial save failed: %v", err)
	}

	// Read the initial content for comparison
	originalData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read initial config: %v", err)
	}

	// Make config directory read-only to simulate write failure
	configDir := filepath.Dir(configPath)
	if err := os.Chmod(configDir, 0555); err != nil {
		t.Fatalf("Failed to make config dir read-only: %v", err)
	}
	defer os.Chmod(configDir, 0755) // Restore permissions

	// Try to save (should fail)
	newConfig := &Config{
		BotToken: "new-token",
		ChatID:   99999,
	}
	if err := Save(newConfig); err == nil {
		t.Error("Expected save to fail on read-only directory, but it succeeded")
	}

	// Verify original config is unchanged
	currentData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config after failed save: %v", err)
	}

	if string(currentData) != string(originalData) {
		t.Errorf("Original config was modified!\nExpected: %s\nGot: %s", string(originalData), string(currentData))
	}

	// Verify it's still valid JSON
	var config Config
	if err := json.Unmarshal(currentData, &config); err != nil {
		t.Fatalf("Original config is no longer valid JSON: %v", err)
	}

	if config.BotToken != "original-token" {
		t.Errorf("BotToken changed from 'original-token' to '%s'", config.BotToken)
	}
	if config.ChatID != 11111 {
		t.Errorf("ChatID changed from 11111 to %d", config.ChatID)
	}
}

// TestAtomicSaveConfigPermissions verifies config file has correct permissions
func TestAtomicSaveConfigPermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-perms-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config := &Config{
		BotToken: "secret-token-12345",
		ChatID:   12345,
		Sessions: map[string]*SessionInfo{
			"test": {TopicID: 100, Path: "/test/path"},
		},
	}

	if err := Save(config); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Config permissions = %o, want 0600 (owner read/write only)", perm)
	}
}

// TestAtomicSaveConfigTempCleanup verifies temp files are cleaned up on error
func TestAtomicSaveConfigTempCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-cleanup-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configPath := filepath.Join(tmpDir, ".config", "ccc", "config.json")

	// Write initial config
	initialConfig := &Config{BotToken: "initial"}
	if err := Save(initialConfig); err != nil {
		t.Fatalf("Initial save failed: %v", err)
	}

	// Count temp files before
	configDir := filepath.Dir(configPath)
	beforeFiles, _ := os.ReadDir(configDir)

	// Make the parent directory read-only to prevent rename
	// This will cause os.Rename to fail
	if err := os.Chmod(configDir, 0555); err != nil {
		t.Fatalf("Failed to make config dir read-only: %v", err)
	}

	// Try to save (should fail during rename)
	newConfig := &Config{BotToken: "new-token"}
	err = Save(newConfig)

	// Restore permissions before any assertions
	os.Chmod(configDir, 0755)

	if err == nil {
		t.Error("Expected save to fail on read-only directory, but it succeeded")
	}

	// Count temp files after
	afterFiles, _ := os.ReadDir(configDir)

	// Filter for .tmp files
	countTmp := func(files []os.DirEntry) int {
		count := 0
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".tmp") {
				count++
			}
		}
		return count
	}

	beforeTmp := countTmp(beforeFiles)
	afterTmp := countTmp(afterFiles)

	if afterTmp > beforeTmp {
		t.Errorf("Temp files leaked: before=%d, after=%d", beforeTmp, afterTmp)
	}
}
