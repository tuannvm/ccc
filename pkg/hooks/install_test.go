package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/tmux"
)

func TestInstallSkillIsProjectScoped(t *testing.T) {
	homeDir := t.TempDir()
	codexHome := filepath.Join(homeDir, ".codex-global")
	projectDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("CODEX_HOME", codexHome)
	t.Chdir(projectDir)

	if err := InstallSkill(); err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}

	for _, path := range []string{
		filepath.Join(projectDir, ".claude", "skills", "ccc.md"),
		filepath.Join(projectDir, ".claude", "skills", "ccc-send.md"),
		filepath.Join(projectDir, ".agents", "skills", "ccc", "SKILL.md"),
		filepath.Join(projectDir, ".agents", "skills", "ccc-send", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected project skill at %s: %v", path, err)
		}
	}

	for _, path := range []string{
		filepath.Join(homeDir, ".claude", "skills", "ccc.md"),
		filepath.Join(codexHome, "config.toml"),
		filepath.Join(codexHome, "skills", "ccc", "SKILL.md"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("global path should not be touched: %s (err=%v)", path, err)
		}
	}

	if err := UninstallSkill(); err != nil {
		t.Fatalf("UninstallSkill: %v", err)
	}
	for _, path := range []string{
		filepath.Join(projectDir, ".claude", "skills", "ccc.md"),
		filepath.Join(projectDir, ".agents", "skills", "ccc", "SKILL.md"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("project skill should be removed: %s (err=%v)", path, err)
		}
	}
}

func TestInstallGlobalSkillWritesOnlyGlobalSkillDirs(t *testing.T) {
	homeDir := t.TempDir()
	codexHome := filepath.Join(homeDir, ".codex-global")
	projectDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("CODEX_HOME", codexHome)
	t.Chdir(projectDir)

	if err := InstallGlobalSkill(); err != nil {
		t.Fatalf("InstallGlobalSkill: %v", err)
	}

	for _, path := range []string{
		filepath.Join(homeDir, ".claude", "skills", "ccc.md"),
		filepath.Join(homeDir, ".claude", "skills", "ccc-send.md"),
		filepath.Join(codexHome, "skills", "ccc", "SKILL.md"),
		filepath.Join(codexHome, "skills", "ccc-send", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected global skill at %s: %v", path, err)
		}
	}

	for _, path := range []string{
		filepath.Join(projectDir, ".claude", "skills", "ccc.md"),
		filepath.Join(projectDir, ".agents", "skills", "ccc", "SKILL.md"),
		filepath.Join(codexHome, "config.toml"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("path should not be touched: %s (err=%v)", path, err)
		}
	}
}

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

func TestCodexCommandHookHashMatchesCodexTrustIdentity(t *testing.T) {
	tests := []struct {
		name string
		spec codexHookTrustSpec
		want string
	}{
		{
			name: "pre tool use",
			spec: codexHookTrustSpec{
				EventName: "pre_tool_use",
				Matcher:   "*",
				Command:   "/Users/tuannvm/bin/ccc hook-permission",
				Timeout:   300000,
			},
			want: "sha256:31a226e3617d1ce95213a671c98d062d882f281945ca93c175bf823ec7d1b6db",
		},
		{
			name: "post tool use",
			spec: codexHookTrustSpec{
				EventName: "post_tool_use",
				Matcher:   "*",
				Command:   "/Users/tuannvm/bin/ccc hook-post-tool",
				Timeout:   600,
			},
			want: "sha256:bf989ca96c9121d6af05f052c835dfd8ad814c1a510c842b4fd9aa25a47a631c",
		},
		{
			name: "stop",
			spec: codexHookTrustSpec{
				EventName: "stop",
				Command:   "/Users/tuannvm/bin/ccc hook-stop",
				Timeout:   600,
			},
			want: "sha256:ad26e4e5ec80008dffb7c9598dccb7ad08283956e7e477f217cb17570c272caf",
		},
		{
			name: "user prompt submit",
			spec: codexHookTrustSpec{
				EventName: "user_prompt_submit",
				Command:   "/Users/tuannvm/bin/ccc hook-user-prompt",
				Timeout:   600,
			},
			want: "sha256:5b1ece6ed7c85fa32359a68fb502b6e7a1a09a3dbfbc47d1e9b3585a9e959df5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codexCommandHookHash(tt.spec); got != tt.want {
				t.Fatalf("codexCommandHookHash() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRemoveCccHooksPreservesMixedMatcherHooks(t *testing.T) {
	entries := []any{
		map[string]any{
			"matcher": "*",
			"hooks": []any{
				map[string]any{"type": "command", "command": "echo keep"},
				map[string]any{"type": "command", "command": "ccc hook-stop"},
			},
		},
	}

	got := RemoveCccHooks(entries)
	if len(got) != 1 {
		t.Fatalf("filtered entries len = %d, want 1", len(got))
	}
	nested := got[0].(map[string]any)["hooks"].([]any)
	if len(nested) != 1 {
		t.Fatalf("nested hooks len = %d, want 1", len(nested))
	}
	if cmd := nested[0].(map[string]any)["command"]; cmd != "echo keep" {
		t.Fatalf("preserved command = %q, want echo keep", cmd)
	}
}

func TestInstallCodexHooksQuotesCccPathWithSpaces(t *testing.T) {
	oldCCCPath := tmux.CCCPath
	tmux.CCCPath = "/tmp/ccc path/bin/ccc"
	t.Cleanup(func() { tmux.CCCPath = oldCCCPath })

	tmpDir := t.TempDir()
	hooksPath := filepath.Join(tmpDir, ".codex", "hooks.json")
	if err := InstallCodexHooksToPath(hooksPath); err != nil {
		t.Fatalf("InstallCodexHooksToPath: %v", err)
	}

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	if !strings.Contains(string(data), "'/tmp/ccc path/bin/ccc' hook-permission") {
		t.Fatalf("hook command did not quote path with spaces:\n%s", string(data))
	}
}

func TestUpsertCodexHookTrustStatesReplacesExistingState(t *testing.T) {
	input := "[profile]\n" +
		"model = \"gpt-5.5\"\n\n" +
		"[hooks.state]\n\n" +
		"[hooks.state.\"/tmp/project/.codex/hooks.json:pre_tool_use:0:0\"]\n" +
		"trusted_hash = \"sha256:old\"\n\n" +
		"[hooks.state.\"/tmp/other/.codex/hooks.json:stop:0:0\"]\n" +
		"trusted_hash = \"sha256:keep\"\n"

	got := upsertCodexHookTrustStates(input, map[string]string{
		"/tmp/project/.codex/hooks.json:pre_tool_use:0:0": "sha256:new",
		"/tmp/project/.codex/hooks.json:stop:0:0":         "sha256:stop",
	})

	for _, want := range []string{
		"[hooks.state.\"/tmp/project/.codex/hooks.json:pre_tool_use:0:0\"]",
		"enabled = true",
		"trusted_hash = \"sha256:new\"",
		"[hooks.state.\"/tmp/project/.codex/hooks.json:stop:0:0\"]",
		"trusted_hash = \"sha256:stop\"",
		"[hooks.state.\"/tmp/other/.codex/hooks.json:stop:0:0\"]",
		"trusted_hash = \"sha256:keep\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated TOML missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sha256:old") {
		t.Fatalf("old trusted hash was not replaced:\n%s", got)
	}
	if strings.Count(got, "enabled = true") != 2 {
		t.Fatalf("updated TOML should enable both trusted project hooks:\n%s", got)
	}
}

func TestEnsureCodexHooksUsesActiveProviderConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	codexHome := filepath.Join(tmpDir, "codex-home")
	defaultHome := filepath.Join(tmpDir, "home")
	t.Setenv("HOME", defaultHome)

	cfg := &config.Config{
		ActiveProvider: "work-codex",
		Providers: map[string]*config.ProviderConfig{
			"work-codex": {
				Backend:   "codex",
				ConfigDir: codexHome,
			},
		},
		Sessions: map[string]*config.SessionInfo{
			"project": {Path: projectDir},
		},
	}

	err := EnsureCodexHooksForSession(&EnsureHooksForSessionConfig{
		Config:      cfg,
		SessionName: "project",
		SessionInfo: cfg.Sessions["project"],
		GetSessionWorkDir: func(_ *config.Config, _ string, info *config.SessionInfo) string {
			return info.Path
		},
	})
	if err != nil {
		t.Fatalf("EnsureCodexHooksForSession: %v", err)
	}

	if !VerifyCodexHooksForProject(projectDir) {
		t.Fatalf("Codex hooks were not installed for project")
	}
	configData, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read Codex config.toml: %v", err)
	}
	if !strings.Contains(string(configData), filepath.Join(projectDir, ".codex", "hooks.json")+":pre_tool_use:0:0") {
		t.Fatalf("active provider config did not receive project hook trust state:\n%s", string(configData))
	}
	if _, err := os.Stat(filepath.Join(defaultHome, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("default Codex config should not be used when active provider has ConfigDir, stat err=%v", err)
	}
}
