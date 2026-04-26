package listen

import (
	"strings"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestSessionPinMessage(t *testing.T) {
	cfg := &configpkg.Config{ActiveProvider: "anthropic"}
	info := &configpkg.SessionInfo{Path: "/Users/tuannvm/Projects/test-codex"}

	msg := sessionPinMessage(cfg, "test-codex", info)
	for _, want := range []string{
		"session: test-codex",
		"provider: anthropic",
		"path: /Users/tuannvm/Projects/test-codex",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("pin message missing %q:\n%s", want, msg)
		}
	}
}
