package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallCodexHooksToPathMergesAndVerifies(t *testing.T) {
	tmpDir := t.TempDir()
	hooksPath := filepath.Join(tmpDir, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	existing := "{\"hooks\":{\"Stop\":[{\"matcher\":\"*\",\"hooks\":[{\"type\":\"command\",\"command\":\"echo keep\"}]}],\"PreToolUse\":[{\"matcher\":\"*\",\"hooks\":[{\"type\":\"command\",\"command\":\"ccc hook-permission\"}]}],\"SessionStart\":[{\"matcher\":\"startup\",\"hooks\":[{\"type\":\"command\",\"command\":\"echo startup\"}]}]}}"
	if err := os.WriteFile(hooksPath, []byte(existing), 0600); err != nil {
		t.Fatalf("write existing hooks: %v", err)
	}

	if err := InstallCodexHooksToPath(hooksPath); err != nil {
		t.Fatalf("InstallCodexHooksToPath: %v", err)
	}

	if !VerifyCodexHooksForProject(tmpDir) {
		t.Fatalf("VerifyCodexHooksForProject = false, want true")
	}

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("unmarshal hooks: %v", err)
	}
	hooksMap := settings["hooks"].(map[string]any)
	if _, ok := hooksMap["SessionStart"]; !ok {
		t.Fatalf("SessionStart hook was not preserved")
	}

	stopEntries := hooksMap["Stop"].([]any)
	if len(stopEntries) != 2 {
		t.Fatalf("Stop hooks len = %d, want 2", len(stopEntries))
	}
	if !IsCccHook(stopEntries[0]) {
		t.Fatalf("first Stop hook should be ccc hook")
	}
	if IsCccHook(stopEntries[1]) {
		t.Fatalf("second Stop hook should preserve non-ccc hook")
	}

	preEntries := hooksMap["PreToolUse"].([]any)
	if len(preEntries) != 1 {
		t.Fatalf("PreToolUse hooks len = %d, want 1", len(preEntries))
	}
}
