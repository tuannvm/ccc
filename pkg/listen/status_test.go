package listen

import (
	"strings"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestBuildStatusMessageShowsProvider(t *testing.T) {
	cfg := &configpkg.Config{
		ActiveProvider: "openai",
		Sessions: map[string]*configpkg.SessionInfo{
			"demo": {
				TopicID:         42,
				Path:            "/tmp/demo",
				ProviderName:    "zai",
				ClaudeSessionID: "1234567890abcdef",
			},
		},
	}

	msg := buildStatusMessage(cfg, 42)
	for _, want := range []string{
		"session: demo",
		"provider: zai",
		"source: session",
		"conversation: 12345678...",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("status message missing %q:\n%s", want, msg)
		}
	}
}

func TestBuildGlobalStatusShowsDefaultProvider(t *testing.T) {
	msg := buildStatusMessage(&configpkg.Config{}, 0)
	for _, want := range []string{
		"ccc status",
		"provider: anthropic",
		"source: builtin default",
		"daily: /new, /provider, /worktree, /status",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("global status missing %q:\n%s", want, msg)
		}
	}
}
