package listen

import (
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

func TestRestartCurrentSessionRequiresMappedPath(t *testing.T) {
	cfg := &configpkg.Config{Sessions: map[string]*configpkg.SessionInfo{}}
	if err := RestartCurrentSession(cfg, "/tmp/unknown"); err == nil {
		t.Fatal("RestartCurrentSession() error = nil, want unmapped path error")
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
