package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
)

// TestRunClaudeRawStaleResumeRetry tests that when claude --resume fails with
// "No conversation found", the stale session ID is cleared and a fresh session starts.
func TestRunClaudeRawStaleResumeRetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Mock claude: fails with "No conversation found" on first call, succeeds on second.
	// claude is invoked as: claude --resume <session-id>, so $2 is the session ID.
	mockClaude := filepath.Join(tmpDir, "claude")
	mockScript := fmt.Sprintf(`#!/bin/bash
countfile="%s/.call_count"
count=0
if [ -f "$countfile" ]; then
	count=$(cat "$countfile")
fi
count=$((count + 1))
echo "$count" > "$countfile"

if [ "$count" -eq 1 ]; then
	echo "Error: No conversation found with session ID: $2" >&2
	exit 1
else
	echo "Claude started"
	exit 0
fi
`, tmpDir)

	if err := os.WriteFile(mockClaude, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	origClaudePath := ClaudePath
	ClaudePath = mockClaude
	defer func() { ClaudePath = origClaudePath }()

	err = RunClaudeRaw(false, "dead-session-id", "", "", "", nil)
	if err != nil {
		t.Errorf("RunClaudeRaw should succeed after retry, got: %v", err)
	}

	countData, err := os.ReadFile(filepath.Join(tmpDir, ".call_count"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(countData)) != "2" {
		t.Errorf("expected 2 claude invocations (initial + retry), got %s", string(countData))
	}
}

// TestRunClaudeRawNoInfiniteRetry tests that retry only happens once.
func TestRunClaudeRawNoInfiniteRetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Mock that ALWAYS fails with "No conversation found"
	mockClaude := filepath.Join(tmpDir, "claude")
	mockScript := fmt.Sprintf(`#!/bin/bash
countfile="%s/.call_count"
count=0
if [ -f "$countfile" ]; then
	count=$(cat "$countfile")
fi
count=$((count + 1))
echo "$count" > "$countfile"
echo "Error: No conversation found with session ID: $2" >&2
exit 1
`, tmpDir)

	if err := os.WriteFile(mockClaude, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	origClaudePath := ClaudePath
	ClaudePath = mockClaude
	defer func() { ClaudePath = origClaudePath }()

	err = RunClaudeRaw(false, "dead-session-id", "", "", "", nil)
	if err == nil {
		t.Error("RunClaudeRaw should return error when retry also fails")
	}

	countData, err := os.ReadFile(filepath.Join(tmpDir, ".call_count"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(countData)) != "2" {
		t.Errorf("expected exactly 2 claude invocations, got %s (infinite retry?)", string(countData))
	}
}

// TestRunClaudeRawNormalResumeSucceeds tests that a normal resume (claude exits 0)
// does not trigger retry logic.
func TestRunClaudeRawNormalResumeSucceeds(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mockClaude := filepath.Join(tmpDir, "claude")
	mockScript := fmt.Sprintf(`#!/bin/bash
countfile="%s/.call_count"
count=0
if [ -f "$countfile" ]; then
	count=$(cat "$countfile")
fi
count=$((count + 1))
echo "$count" > "$countfile"
echo "Resumed session $2"
exit 0
`, tmpDir)

	if err := os.WriteFile(mockClaude, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	origClaudePath := ClaudePath
	ClaudePath = mockClaude
	defer func() { ClaudePath = origClaudePath }()

	err = RunClaudeRaw(false, "valid-session-id", "", "", "", nil)
	if err != nil {
		t.Errorf("RunClaudeRaw should succeed on valid resume, got: %v", err)
	}

	countData, err := os.ReadFile(filepath.Join(tmpDir, ".call_count"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(countData)) != "1" {
		t.Errorf("expected 1 claude invocation (no retry), got %s", string(countData))
	}
}

// TestRunClaudeRawDifferentErrorNoRetry tests that non-"No conversation found"
// errors do not trigger retry.
func TestRunClaudeRawDifferentErrorNoRetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mockClaude := filepath.Join(tmpDir, "claude")
	mockScript := fmt.Sprintf(`#!/bin/bash
countfile="%s/.call_count"
count=0
if [ -f "$countfile" ]; then
	count=$(cat "$countfile")
fi
count=$((count + 1))
echo "$count" > "$countfile"
echo "Error: authentication failed" >&2
exit 1
`, tmpDir)

	if err := os.WriteFile(mockClaude, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	origClaudePath := ClaudePath
	ClaudePath = mockClaude
	defer func() { ClaudePath = origClaudePath }()

	err = RunClaudeRaw(false, "some-session-id", "", "", "", nil)
	if err == nil {
		t.Error("RunClaudeRaw should return error for auth failure")
	}

	countData, err := os.ReadFile(filepath.Join(tmpDir, ".call_count"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(countData)) != "1" {
		t.Errorf("expected 1 claude invocation (no retry for different error), got %s", string(countData))
	}
}

// TestRunClaudeRawWorktreeAlwaysPassesFlag tests that --worktree <name> is always
// passed to Claude regardless of whether the worktree directory exists.
// Claude handles both new and existing worktrees correctly.
func TestRunClaudeRawWorktreeAlwaysPassesFlag(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create an existing worktree directory
	worktreePath := filepath.Join(tmpDir, ".claude", "worktrees", "existing-wt")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	mockClaude := filepath.Join(tmpDir, "claude")
	mockScript := fmt.Sprintf(`#!/bin/bash
echo "ARGS: $*" > %s/output.txt
	exit 0
`, tmpDir)

	if err := os.WriteFile(mockClaude, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	origClaudePath := ClaudePath
	ClaudePath = mockClaude
	defer func() { ClaudePath = origClaudePath }()

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	// Even with existing worktree, --worktree flag should be passed to Claude
	err = RunClaudeRaw(false, "", "", "existing-wt", WorktreeAutoGenerate, nil)
	if err != nil {
		t.Errorf("RunClaudeRaw should succeed, got: %v", err)
	}

	output, err := os.ReadFile(filepath.Join(tmpDir, "output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	outputStr := string(output)

	if !strings.Contains(outputStr, "--worktree existing-wt") {
		t.Errorf("--worktree existing-wt should always be passed to Claude, got: %s", outputStr)
	}
}

// TestRunClaudeRawWorktreeNewNamePassesFlag tests that --worktree <name> is passed
// for a new (non-existing) worktree as well.
func TestRunClaudeRawWorktreeNewNamePassesFlag(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mockClaude := filepath.Join(tmpDir, "claude")
	mockScript := fmt.Sprintf(`#!/bin/bash
echo "ARGS: $*" > %s/output.txt
	exit 0
`, tmpDir)

	if err := os.WriteFile(mockClaude, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	origClaudePath := ClaudePath
	ClaudePath = mockClaude
	defer func() { ClaudePath = origClaudePath }()

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	err = RunClaudeRaw(false, "", "", "brand-new-wt", WorktreeAutoGenerate, nil)
	if err != nil {
		t.Errorf("RunClaudeRaw should succeed, got: %v", err)
	}

	output, err := os.ReadFile(filepath.Join(tmpDir, "output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	outputStr := string(output)

	if !strings.Contains(outputStr, "--worktree brand-new-wt") {
		t.Errorf("--worktree brand-new-wt should be passed for new worktrees, got: %s", outputStr)
	}
}

// TestRunClaudeRawWorktreeAutoGeneratePassesFlag tests that auto-generate
// passes --worktree without a name.
func TestRunClaudeRawWorktreeAutoGeneratePassesFlag(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mockClaude := filepath.Join(tmpDir, "claude")
	mockScript := fmt.Sprintf(`#!/bin/bash
echo "ARGS: $*" > %s/output.txt
	exit 0
`, tmpDir)

	if err := os.WriteFile(mockClaude, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	origClaudePath := ClaudePath
	ClaudePath = mockClaude
	defer func() { ClaudePath = origClaudePath }()

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	err = RunClaudeRaw(false, "", "", WorktreeAutoGenerate, WorktreeAutoGenerate, nil)
	if err != nil {
		t.Errorf("RunClaudeRaw should succeed, got: %v", err)
	}

	output, err := os.ReadFile(filepath.Join(tmpDir, "output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	outputStr := string(output)

	if !strings.Contains(outputStr, "--worktree") {
		t.Errorf("--worktree should be passed for auto-generate, got: %s", outputStr)
	}
}

func TestBuildAgentCommandCodexFresh(t *testing.T) {
	origCodexPath := CodexPath
	CodexPath = "/usr/local/bin/codex"
	defer func() { CodexPath = origCodexPath }()

	cmdPath, args, isCodex, err := buildAgentCommand(providerpkg.CodexProvider{}, false, "", "", WorktreeAutoGenerate)
	if err != nil {
		t.Fatalf("buildAgentCommand(codex) error = %v", err)
	}
	if cmdPath != CodexPath {
		t.Fatalf("cmdPath = %q, want %q", cmdPath, CodexPath)
	}
	if !isCodex {
		t.Fatal("isCodex = false, want true")
	}
	if strings.Join(args, " ") != "--no-alt-screen" {
		t.Fatalf("args = %v, want --no-alt-screen", args)
	}
}

func TestBuildAgentCommandConfiguredCodexProvider(t *testing.T) {
	origCodexPath := CodexPath
	CodexPath = "/usr/local/bin/codex"
	defer func() { CodexPath = origCodexPath }()

	provider := providerpkg.CodexProvider{
		ProviderName: "codex-anthropic",
		Config: &configpkg.ProviderConfig{
			Backend:     providerpkg.BackendCodex,
			BaseURL:     "http://127.0.0.1:8317/v1",
			SonnetModel: "claude-opus-4-7",
		},
	}
	cmdPath, args, isCodex, err := buildAgentCommand(provider, false, "", "", WorktreeAutoGenerate)
	if err != nil {
		t.Fatalf("buildAgentCommand(configured codex) error = %v", err)
	}
	if cmdPath != CodexPath || !isCodex {
		t.Fatalf("cmdPath=%q isCodex=%v", cmdPath, isCodex)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		`model_provider="cliproxyapi"`,
		`model_providers.cliproxyapi.base_url="http://127.0.0.1:8317/v1"`,
		"--model claude-opus-4-7",
		"--no-alt-screen",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

func TestBuildAgentCommandCodexResume(t *testing.T) {
	origCodexPath := CodexPath
	CodexPath = "/usr/local/bin/codex"
	defer func() { CodexPath = origCodexPath }()

	_, args, isCodex, err := buildAgentCommand(providerpkg.CodexProvider{}, false, "thread-id", "", WorktreeAutoGenerate)
	if err != nil {
		t.Fatalf("buildAgentCommand(codex resume) error = %v", err)
	}
	if !isCodex {
		t.Fatal("isCodex = false, want true")
	}
	if strings.Join(args, " ") != "resume --no-alt-screen thread-id" {
		t.Fatalf("args = %v, want codex resume args", args)
	}
}

func TestBuildAgentCommandCodexContinueUsesResumeLast(t *testing.T) {
	origCodexPath := CodexPath
	CodexPath = "/usr/local/bin/codex"
	defer func() { CodexPath = origCodexPath }()

	_, args, isCodex, err := buildAgentCommand(providerpkg.CodexProvider{}, true, "", "", WorktreeAutoGenerate)
	if err != nil {
		t.Fatalf("buildAgentCommand(codex continue) error = %v", err)
	}
	if !isCodex {
		t.Fatal("isCodex = false, want true")
	}
	if strings.Join(args, " ") != "resume --last --no-alt-screen" {
		t.Fatalf("args = %v, want codex resume --last args", args)
	}
}

func TestBuildAgentCommandCodexRejectsWorktree(t *testing.T) {
	origCodexPath := CodexPath
	CodexPath = "/usr/local/bin/codex"
	defer func() { CodexPath = origCodexPath }()

	_, _, _, err := buildAgentCommand(providerpkg.CodexProvider{}, false, "", "feature", WorktreeAutoGenerate)
	if err == nil {
		t.Fatal("buildAgentCommand(codex worktree) error = nil, want error")
	}
}
