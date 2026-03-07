package tmux

import (
	"testing"
)

// TestTmuxSafeName tests the TmuxSafeName function
func TestTmuxSafeName(t *testing.T) {
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
			result := TmuxSafeName(tt.input)
			if result != tt.expected {
				t.Errorf("TmuxSafeName(%q) = %q, want %q", tt.input, result, tt.expected)
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
			result := windowNameFromTarget(tt.target)
			if result != tt.expected {
				t.Errorf("windowNameFromTarget(%q) = %q, want %q", tt.target, result, tt.expected)
			}
		})
	}
}
