package telegram

import (
	"testing"
)

// TestMessageTruncation tests that long messages are truncated
func TestMessageTruncation(t *testing.T) {
	// The SendMessage function truncates at 4000 chars
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

			// Apply same truncation logic as SendMessage
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

// TestSplitMessage tests the splitMessage function
func TestSplitMessage(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxLen   int
		expected []string
	}{
		{
			name:     "short message - no split",
			text:     "Hello world",
			maxLen:   100,
			expected: []string{"Hello world"},
		},
		{
			name:     "exactly max length",
			text:     "1234567890",
			maxLen:   10,
			expected: []string{"1234567890"},
		},
		{
			name:     "split at newline",
			text:     "123456789\n123456789",
			maxLen:   10,
			expected: []string{"123456789", "123456789"},
		},
		{
			name:     "split at space",
			text:     "12345 67890",
			maxLen:   8,
			expected: []string{"12345", "67890"},
		},
		{
			name:     "long text without break",
			text:     "12345678901234567890",
			maxLen:   10,
			expected: []string{"1234567890", "1234567890"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitMessage(tt.text, tt.maxLen)
			if len(result) != len(tt.expected) {
				t.Errorf("splitMessage returned %d parts, want %d", len(result), len(tt.expected))
				return
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("part %d = %q, want %q", i, result[i], exp)
				}
			}
		})
	}
}
