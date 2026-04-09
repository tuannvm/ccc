package exec

import (
	"strings"
	"testing"
)

// TestRunShell tests the RunShell function
func TestRunShell(t *testing.T) {
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
			output, err := RunShell(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunShell(%q) error = %v, wantErr %v", tt.cmd, err, tt.wantErr)
			}
			if tt.wantContain != "" && !strings.Contains(output, tt.wantContain) {
				t.Errorf("RunShell(%q) output = %q, want to contain %q", tt.cmd, output, tt.wantContain)
			}
		})
	}
}
