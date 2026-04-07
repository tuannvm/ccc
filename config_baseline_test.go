package main

import (
	"os"
	"testing"
)

// ============================================================================
// BASELINE TESTS - Testing existing behavior BEFORE refactoring
// These tests ensure refactoring doesn't break existing functionality
// ============================================================================

// TestBaselineConfigLoadSave tests that config load/save works correctly
func TestBaselineConfigLoadSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test 1: Create a config and save it
	config := &Config{
		BotToken:          "test-token-123",
		ChatID:            12345,
		GroupID:           67890,
		ProjectsDir:       "~/Projects",
		TranscriptionLang: "en",
		RelayURL:          "https://relay.example.com",
		Away:              true,
		OAuthToken:        "oauth-token",
		OTPSecret:         "otp-secret",
		ActiveProvider:    "test-provider",
		Providers: map[string]*ProviderConfig{
			"test-provider": {
				AuthToken:  "provider-token",
				AuthEnvVar: "TEST_API_KEY",
				BaseURL:     "https://api.example.com",
				ApiTimeout: 30000,
				OpusModel:   "claude-3-opus-20250114",
				SonnetModel: "claude-3-5-20241022",
			},
		},
		Sessions: map[string]*SessionInfo{
			"test-project": {
				TopicID:      1001,
				Path:         "/home/user/test-project",
				ProviderName: "test-provider",
			},
		},
	}

	// Save config
	if err := saveConfig(config); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	// Load config back
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Verify all fields match
	if loaded.BotToken != config.BotToken {
		t.Errorf("BotToken: got %q, want %q", loaded.BotToken, config.BotToken)
	}
	if loaded.ChatID != config.ChatID {
		t.Errorf("ChatID: got %d, want %d", loaded.ChatID, config.ChatID)
	}
	if loaded.GroupID != config.GroupID {
		t.Errorf("GroupID: got %d, want %d", loaded.GroupID, config.GroupID)
	}
	if loaded.ProjectsDir != config.ProjectsDir {
		t.Errorf("ProjectsDir: got %q, want %q", loaded.ProjectsDir, config.ProjectsDir)
	}
	if loaded.TranscriptionLang != config.TranscriptionLang {
		t.Errorf("TranscriptionLang: got %q, want %q", loaded.TranscriptionLang, config.TranscriptionLang)
	}
	if loaded.RelayURL != config.RelayURL {
		t.Errorf("RelayURL: got %q, want %q", loaded.RelayURL, config.RelayURL)
	}
	if loaded.Away != config.Away {
		t.Errorf("Away: got %v, want %v", loaded.Away, config.Away)
	}
	if loaded.OAuthToken != config.OAuthToken {
		t.Errorf("OAuthToken: got %q, want %q", loaded.OAuthToken, config.OAuthToken)
	}
	if loaded.OTPSecret != config.OTPSecret {
		t.Errorf("OTPSecret: got %q, want %q", loaded.OTPSecret, config.OTPSecret)
	}
	if loaded.ActiveProvider != config.ActiveProvider {
		t.Errorf("ActiveProvider: got %q, want %q", loaded.ActiveProvider, config.ActiveProvider)
	}

	// Verify providers
	if len(loaded.Providers) != 1 {
		t.Errorf("Providers count: got %d, want 1", len(loaded.Providers))
	}
	if p, ok := loaded.Providers["test-provider"]; !ok {
		t.Error("test-provider not found in loaded config")
	} else {
		if p.AuthToken != config.Providers["test-provider"].AuthToken {
			t.Errorf("Provider AuthToken mismatch")
		}
		if p.ApiTimeout != config.Providers["test-provider"].ApiTimeout {
			t.Errorf("Provider ApiTimeout mismatch")
		}
	}

	// Verify sessions
	if len(loaded.Sessions) != 1 {
		t.Errorf("Sessions count: got %d, want 1", len(loaded.Sessions))
	}
	if s, ok := loaded.Sessions["test-project"]; !ok {
		t.Error("test-project session not found")
	} else {
		if s.TopicID != 1001 {
			t.Errorf("Session TopicID: got %d, want 1001", s.TopicID)
		}
	}
}
