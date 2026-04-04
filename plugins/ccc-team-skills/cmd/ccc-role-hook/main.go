// ccc-role-hook is DEPRECATED and orphaned code.
// This binary was never built or registered and is kept for reference only.
//
// DEPRECATED: This approach used CCC_ACTIVE_ROLE which is inconsistent with
// the CCC_ROLE environment variable used by hooks.go.
//
// DEPRECATED: This writes to ~/.claude/tmp/ccc-active-role.txt which violates
// the requirement of no creation in global ~/.claude/
//
// The correct implementation is in the CCC hooks.go handleSessionStartHook()
// function, which writes to CLAUDE_ENV_FILE instead.
//
// To build this deprecated binary for reference:
//   go build -o ccc-role-hook ./cmd/ccc-role-hook
//
// But DO NOT use it - it is kept only for historical reference.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const markerFile = "ccc-active-role.txt"
const tmpDir = ".claude/tmp"

// sessionStartInput represents Claude Code SessionStart hook input
type sessionStartInput struct {
	Source string `json:"source"`
}

// readStdin reads from stdin with timeout (matches CCC hook pattern)
func readStdin() ([]byte, error) {
	stdinData := make(chan []byte, 1)
	go func() {
		defer func() { recover() }()
		data, _ := io.ReadAll(os.Stdin)
		stdinData <- data
	}()

	select {
	case rawData := <-stdinData:
		return rawData, nil
	case <-time.After(2 * time.Second):
		return nil, nil
	}
}

func main() {
	defer func() { recover() }()

	rawData, err := readStdin()
	if err != nil || len(rawData) == 0 {
		os.Exit(0)
	}

	var input sessionStartInput
	if err := json.Unmarshal(rawData, &input); err != nil {
		os.Exit(0)
	}

	// Only trigger on new session startup
	if input.Source != "startup" {
		os.Exit(0)
	}

	cccRole := os.Getenv("CCC_ROLE")
	if cccRole == "" {
		os.Exit(0)
	}

	// Normalize role name
	switch cccRole {
	case "planner", "executor", "reviewer":
	default:
		os.Exit(0)
	}

	// Create marker file for the skill to detect
	home, err := os.UserHomeDir()
	if err != nil {
		os.Exit(0)
	}

	tmpPath := filepath.Join(home, tmpDir)
	if err := os.MkdirAll(tmpPath, 0755); err != nil {
		os.Exit(0)
	}

	markerPath := filepath.Join(tmpPath, markerFile)
	if err := os.WriteFile(markerPath, []byte(cccRole), 0644); err != nil {
		os.Exit(0)
	}

	// Also write to CLAUDE_ENV_FILE if set (for session-wide env persistence)
	if envFile := os.Getenv("CLAUDE_ENV_FILE"); envFile != "" {
		f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			fmt.Fprintf(f, "export CCC_ACTIVE_ROLE=\"%s\"\n", cccRole)
			f.Close()
		}
	}

	os.Exit(0)
}
