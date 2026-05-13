package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestFindExistingWindowDoesNotCreateBootstrapSession(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"$TMUX_LOG\"\n" +
		"case \"$1\" in\n" +
		"  list-sessions) exit 1 ;;\n" +
		"  new-session|set-option) exit 0 ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	oldTmuxPath := TmuxPath
	TmuxPath = fakeTmux
	t.Setenv("TMUX_LOG", logPath)
	t.Cleanup(func() { TmuxPath = oldTmuxPath })

	target, err := FindExistingWindow("demo")
	if err != nil {
		t.Fatalf("FindExistingWindow error = %v", err)
	}
	if target != "" {
		t.Fatalf("FindExistingWindow target = %q, want empty", target)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	if strings.Contains(string(logData), "new-session") {
		t.Fatalf("FindExistingWindow created a tmux session:\n%s", string(logData))
	}
}

func TestEnsureProjectWindowInDirCreatesFirstWindowInProjectDir(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"$TMUX_LOG\"\n" +
		"case \"$1\" in\n" +
		"  list-sessions) exit 1 ;;\n" +
		"  new-session|set-option) exit 0 ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	oldTmuxPath := TmuxPath
	TmuxPath = fakeTmux
	t.Setenv("TMUX_LOG", logPath)
	t.Cleanup(func() { TmuxPath = oldTmuxPath })

	projectDir := filepath.Join(tmpDir, "project")
	target, err := EnsureProjectWindowInDir("demo.project", projectDir)
	if err != nil {
		t.Fatalf("EnsureProjectWindowInDir error = %v", err)
	}
	if target != "ccc:demo__project" {
		t.Fatalf("target = %q, want ccc:demo__project", target)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	want := "new-session -d -s ccc -n demo__project -c " + projectDir
	if !strings.Contains(string(logData), want) {
		t.Fatalf("tmux log missing project-dir first window command %q:\n%s", want, string(logData))
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

func TestDetectConsentDialog(t *testing.T) {
	currentClaudePrompt := `────────────────────────────────────────────────────────────────────────────────
 Accessing workspace:

 /Users/tuannvm/Projects/test-codex

 Quick safety check: Is this a project you created or one you trust? (Like your
 own code, a well-known open source project, or work from your team). If not,
 take a moment to review what's in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 Security guide
`

	numberedClaudePrompt := currentClaudePrompt + `

 ❯ 1. Yes, I trust this folder
   2. No, exit

 Enter to confirm · Esc to cancel
`

	activePrompt := `╭─── Claude Code v2.1.119 ─────────────────────────────────────────────────────╮
│ Welcome back Tommy!                                                          │
╰──────────────────────────────────────────────────────────────────────────────╯
❯
`

	codexExternalAgentPrompt := `External agent config detected
We found settings from another agent that you can add to this project.
Select what to import
Project: /Users/tuannvm/Projects/codex-test
  [ ] Migrate hooks from /Users/tuannvm/Projects/codex-test/.claude to .codex/hooks.json

Selected 0 of 1 item(s).
  1. Proceed with selected
› 2. Skip for now
  3. Don't ask again
Use ↑/↓ to move, space to toggle, 1/2/3 to choose, a/n for all/none
`

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "current safety screen without visible options", content: currentClaudePrompt, want: true},
		{name: "numbered trust dialog", content: numberedClaudePrompt, want: true},
		{name: "codex external agent migration", content: codexExternalAgentPrompt, want: true},
		{name: "active claude prompt", content: activePrompt, want: false},
		{name: "shell output mentioning trust", content: "run tests before you trust this result\n$", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectConsentDialog(tt.content); got != tt.want {
				t.Errorf("DetectConsentDialog() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConsentDialogChoice(t *testing.T) {
	codexExternalAgentPrompt := `External agent config detected
Select what to import
[ ] Migrate hooks from /tmp/project/.claude to .codex/hooks.json
1. Proceed with selected
2. Skip for now
3. Don't ask again
`
	if choice, ok := ConsentDialogChoice(codexExternalAgentPrompt); !ok || choice != "2" {
		t.Fatalf("ConsentDialogChoice(codex migration) = %q, %v; want 2, true", choice, ok)
	}

	trustPrompt := `Quick safety check: Is this a project you created or one you trust?
1. Yes, I trust this folder
2. No, exit
`
	if choice, ok := ConsentDialogChoice(trustPrompt); !ok || choice != "1" {
		t.Fatalf("ConsentDialogChoice(trust) = %q, %v; want 1, true", choice, ok)
	}
}

func TestHasActiveCodexPrompt(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "active codex prompt",
			content: "OpenAI Codex v0.60.0\n\n› ",
			want:    true,
		},
		{
			name:    "shell prompt after codex command",
			content: "which codex\n/Users/me/.npm-global/bin/codex\n~/repo > ",
			want:    false,
		},
		{
			name:    "bare glyph without codex context",
			content: "some unrelated tui\n› ",
			want:    false,
		},
		{
			name:    "prompt glyph after codex header scrolled away",
			content: "assistant output\n\n› ",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasActiveCodexPrompt(tt.content); got != tt.want {
				t.Fatalf("hasActiveCodexPrompt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasRecentCodexPromptGlyph(t *testing.T) {
	if !hasRecentCodexPromptGlyph("assistant output\n\n› ") {
		t.Fatal("hasRecentCodexPromptGlyph() = false, want true")
	}
	if hasRecentCodexPromptGlyph("assistant output\n› old prompt\nmore output\nmore output\nmore output\nmore output\nmore output") {
		t.Fatal("hasRecentCodexPromptGlyph() = true for stale prompt, want false")
	}
}

func TestTargetHasCodexProcessContextUsesTargetPane(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"$TMUX_LOG\"\n" +
		"case \"$1\" in\n" +
		"  display-message) printf '%%9\\t%s\\n' \"$TMUX_PANE_CMD\"; exit 0 ;;\n" +
		"  list-panes) exit 2 ;;\n" +
		"  *) exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	oldTmuxPath := TmuxPath
	TmuxPath = fakeTmux
	t.Setenv("TMUX_LOG", logPath)
	t.Setenv("TMUX_PANE_CMD", "codex")
	t.Cleanup(func() { TmuxPath = oldTmuxPath })

	if !targetHasCodexProcessContext("ccc:demo.1") {
		t.Fatal("targetHasCodexProcessContext(codex) = false, want true")
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	if strings.Contains(string(logData), "list-panes") {
		t.Fatalf("targetHasCodexProcessContext used active pane list instead of target pane:\n%s", string(logData))
	}
}

func TestFilterDescendantProcesses(t *testing.T) {
	psOutput := []byte(`  PID  PPID COMMAND
  100     1 zsh
  101   100 ccc run --provider codex
  102   101 /opt/homebrew/bin/codex --no-alt-screen
  200     1 unrelated
`)
	got := string(filterDescendantProcesses(psOutput, "100"))
	if !strings.Contains(got, "101 ccc run --provider codex") {
		t.Fatalf("missing ccc child in descendants: %q", got)
	}
	if !strings.Contains(got, "102 /opt/homebrew/bin/codex --no-alt-screen") {
		t.Fatalf("missing codex grandchild in descendants: %q", got)
	}
	if strings.Contains(got, "unrelated") {
		t.Fatalf("included unrelated process: %q", got)
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
