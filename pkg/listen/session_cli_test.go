package listen

import (
	"os"
	"strings"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestBuildCLISessionsListShowsTelegramBridge(t *testing.T) {
	cfg := &configpkg.Config{
		ActiveProvider: "openai",
		Sessions: map[string]*configpkg.SessionInfo{
			"demo": {
				TopicID:         123,
				Path:            "/tmp/demo",
				ProviderName:    "zai",
				ClaudeSessionID: "1234567890abcdef",
			},
		},
	}

	msg := BuildCLISessionsList(cfg, "/tmp/demo/subdir")
	for _, want := range []string{
		"ccc status all",
		"* demo",
		"zai",
		"topic:123",
		"conversation:12345678...",
		"/tmp/demo",
		"attach: ccc status attach <session>",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("session list missing %q:\n%s", want, msg)
		}
	}
}

func TestBuildCurrentSessionStatusShowsAttachCommand(t *testing.T) {
	cfg := &configpkg.Config{
		Sessions: map[string]*configpkg.SessionInfo{
			"demo": {
				TopicID: 123,
				Path:    "/tmp/demo",
			},
		},
	}

	msg := BuildCurrentSessionStatus(cfg, "/tmp/demo")
	for _, want := range []string{
		"session: demo",
		"telegram topic: 123",
		"attach: ccc status attach demo",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("current session missing %q:\n%s", want, msg)
		}
	}
}

func TestBuildCurrentSessionStatusNoMapping(t *testing.T) {
	msg := BuildCurrentSessionStatus(&configpkg.Config{}, "/tmp/unknown")
	if !strings.Contains(msg, "no session mapped") {
		t.Fatalf("expected no mapping message, got:\n%s", msg)
	}
}

func TestBuildCurrentSessionStatusNilConfig(t *testing.T) {
	msg := BuildCurrentSessionStatus(nil, "/tmp/unknown")
	if !strings.Contains(msg, "ccc status: config unavailable") {
		t.Fatalf("expected config unavailable message, got:\n%s", msg)
	}
}

func TestRestartCurrentSessionRequiresMappedPath(t *testing.T) {
	cfg := &configpkg.Config{Sessions: map[string]*configpkg.SessionInfo{}}
	if err := RestartCurrentSession(cfg, "/tmp/unknown"); err == nil || !strings.Contains(err.Error(), "no session mapped to") {
		t.Fatalf("RestartCurrentSession() error = %v, want unmapped path guidance", err)
	}
}

func TestAttachSessionByNameRejectsTeamSession(t *testing.T) {
	cfg := &configpkg.Config{
		TeamSessions: map[int64]*configpkg.SessionInfo{
			123: {SessionName: "demo-team"},
		},
	}
	err := AttachSessionByName(cfg, "demo-team")
	if err == nil || !strings.Contains(err.Error(), "ccc team attach demo-team") {
		t.Fatalf("AttachSessionByName(team) error = %v, want team attach guidance", err)
	}
}

func TestSyncSessionInCurrentDirCreatesAndReusesLocalMapping(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CCC_AGENT_PROVIDER", "anthropic")
	cwd := t.TempDir()
	t.Chdir(cwd)

	cfg := &configpkg.Config{
		Sessions: map[string]*configpkg.SessionInfo{},
	}

	if err := SyncSessionInCurrentDir(cfg, ""); err != nil {
		t.Fatalf("SyncSessionInCurrentDir() create error = %v", err)
	}
	if len(cfg.Sessions) != 1 {
		t.Fatalf("sessions len after create = %d, want 1", len(cfg.Sessions))
	}

	var sessionName string
	var sessionInfo *configpkg.SessionInfo
	for name, info := range cfg.Sessions {
		sessionName = name
		sessionInfo = info
	}
	if sessionInfo.Path != cwd {
		t.Fatalf("session path = %q, want %q", sessionInfo.Path, cwd)
	}
	if sessionInfo.TopicID != 0 {
		t.Fatalf("topic id = %d, want local session without topic", sessionInfo.TopicID)
	}
	if _, err := os.Stat(".claude/settings.local.json"); err != nil {
		t.Fatalf("expected project hooks to be installed: %v", err)
	}

	if err := SyncSessionInCurrentDir(cfg, ""); err != nil {
		t.Fatalf("SyncSessionInCurrentDir() reuse error = %v", err)
	}
	if len(cfg.Sessions) != 1 {
		t.Fatalf("sessions len after reuse = %d, want 1", len(cfg.Sessions))
	}
	if cfg.Sessions[sessionName] != sessionInfo {
		t.Fatalf("sync did not reuse existing session mapping")
	}
}

func TestSyncSessionInCurrentDirUsesRuntimeProviderContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CCC_AGENT_PROVIDER", "codex")
	t.Setenv("CCC_AGENT_SESSION_ID", "thread-123")
	cwd := t.TempDir()
	t.Chdir(cwd)

	cfg := &configpkg.Config{
		ActiveProvider: "anthropic",
		Sessions:       map[string]*configpkg.SessionInfo{},
	}

	if err := SyncSessionInCurrentDir(cfg, ""); err != nil {
		t.Fatalf("SyncSessionInCurrentDir() error = %v", err)
	}
	if len(cfg.Sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(cfg.Sessions))
	}
	for _, info := range cfg.Sessions {
		if info.ProviderName != "codex" {
			t.Fatalf("provider = %q, want codex", info.ProviderName)
		}
		if info.ClaudeSessionID != "thread-123" {
			t.Fatalf("session id = %q, want thread-123", info.ClaudeSessionID)
		}
	}
}
