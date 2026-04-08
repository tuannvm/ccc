package tmux

import (
	"os/exec"
	"strings"
	"testing"
)

// TestSafeName tests the SafeName function
func TestSafeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple name", "myproject", "myproject"},
		{"with dash", "my-project", "my-project"},
		{"with dot", "my.project", "my__project"},
		{"empty", "", ""},
		{"with spaces", "my project", "my project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeName(tt.input)
			if result != tt.expected {
				t.Errorf("SafeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestWindowNameFromTarget tests extracting window name from tmux target
func TestWindowNameFromTarget(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		expected string
	}{
		{"session:window", "ccc:myproject", "myproject"},
		{"no colon", "myproject", "myproject"},
		{"multiple colons", "sess:win:extra", "extra"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WindowNameFromTarget(tt.target)
			if result != tt.expected {
				t.Errorf("WindowNameFromTarget(%q) = %q, want %q", tt.target, result, tt.expected)
			}
		})
	}
}

// TestCaptureVisiblePane tests bounded pane capture
// Note: This test requires tmux to be running and is skipped in CI
func TestCaptureVisiblePane(t *testing.T) {
	// Check if tmux is available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping TestCaptureVisiblePane")
	}

	// Initialize paths
	InitPaths()

	// Check if ccc session exists
	if !SessionExists() {
		t.Skip("ccc tmux session not found, skipping TestCaptureVisiblePane")
	}

	// Find an existing window to test
	target, err := FindExistingWindow("test")
	if err != nil {
		t.Skip("no test window found, skipping TestCaptureVisiblePane")
	}

	// Test capturing visible pane
	content := CaptureVisiblePane(target)
	if content == "" {
		t.Error("CaptureVisiblePane returned empty string")
	}

	// Verify content doesn't contain excessive scrollback
	// (bounded capture should limit to visible window)
	lines := strings.Split(content, "\n")
	// A typical tmux pane is 24-50 lines, allow some margin
	if len(lines) > 100 {
		t.Logf("Warning: captured %d lines, may include scrollback", len(lines))
	}
}

// TestAutoAcceptTrustDialog tests the auto-accept function
// Note: This is an integration test and requires proper tmux setup
func TestAutoAcceptTrustDialog(t *testing.T) {
	// Check if tmux is available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping TestAutoAcceptTrustDialog")
	}

	// Initialize paths
	InitPaths()

	// Test with invalid target (no actual dialog expected)
	result := AutoAcceptTrustDialog("invalid:target.pane")
	if result {
		t.Error("AutoAcceptTrustDialog returned true for invalid target")
	}
}
