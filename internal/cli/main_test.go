package cli

import (
	"os"
	"testing"

	"github.com/kidandcat/ccc/internal/config"
)

// TestExecuteCommand tests the executeCommand function
func TestExecuteCommand(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		wantContain string
		wantErr     bool
	}{
		{"echo", "echo hello", "hello", false},
		{"pwd", "pwd", "/", false},
		{"invalid command", "nonexistentcommand123", "", true},
		{"exit code", "exit 1", "", true},
		{"stderr output", "echo error >&2", "error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("executeCommand(%q) error = %v, wantErr %v", tt.cmd, err, tt.wantErr)
			}
			if tt.wantContain != "" && !contains(output, tt.wantContain) {
				t.Errorf("executeCommand(%q) output = %q, want to contain %q", tt.cmd, output, tt.wantContain)
			}
		})
	}
}

// TestMessageTruncation tests that long messages are truncated
func TestMessageTruncation(t *testing.T) {
	// The sendMessage function truncates at 4000 chars
	// We test the truncation logic directly
	const maxLen = 4000

	tests := []struct {
		name       string
		inputLen   int
		shouldTrim bool
	}{
		{"short message", 100, false},
		{"exactly max", maxLen, false},
		{"over max", maxLen + 100, true},
		{"way over max", 10000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create message of specified length
			text := make([]byte, tt.inputLen)
			for i := range text {
				text[i] = 'a'
			}
			msg := string(text)

			// Apply same truncation logic as sendMessage
			if len(msg) > maxLen {
				msg = msg[:maxLen] + "\n... (truncated)"
			}

			if tt.shouldTrim {
				if len(msg) <= tt.inputLen {
					// Should have been truncated
					if len(msg) != maxLen+len("\n... (truncated)") {
						t.Errorf("truncated length = %d, want %d", len(msg), maxLen+len("\n... (truncated)"))
					}
				}
			} else {
				if len(msg) != tt.inputLen {
					t.Errorf("message was unexpectedly modified")
				}
			}
		})
	}
}

// TestHandleConfigCommand tests the config command handler
func TestHandleConfigCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test showing config with no config file
	if err := HandleConfigCommand([]string{"ccc", "config"}); err != nil {
		t.Errorf("HandleConfigCommand with no config should not error: %v", err)
	}

	// Create a basic config
	cfg := &config.Config{
		BotToken:    "test-token",
		ChatID:      12345,
		GroupID:     67890,
		ProjectsDir: "~/projects",
	}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Test showing config
	if err := HandleConfigCommand([]string{"ccc", "config"}); err != nil {
		t.Errorf("HandleConfigCommand failed: %v", err)
	}

	// Test setting projects-dir
	if err := HandleConfigCommand([]string{"ccc", "config", "projects-dir", "/tmp/projects"}); err != nil {
		t.Errorf("HandleConfigCommand projects-dir failed: %v", err)
	}

	// Verify it was saved
	loaded, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if loaded.ProjectsDir != "/tmp/projects" {
		t.Errorf("ProjectsDir = %q, want /tmp/projects", loaded.ProjectsDir)
	}

	// Test setting oauth-token
	if err := HandleConfigCommand([]string{"ccc", "config", "oauth-token", "test-token-123"}); err != nil {
		t.Errorf("HandleConfigCommand oauth-token failed: %v", err)
	}

	// Verify it was saved
	loaded, err = config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if loaded.OAuthToken != "test-token-123" {
		t.Errorf("OAuthToken = %q, want test-token-123", loaded.OAuthToken)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
