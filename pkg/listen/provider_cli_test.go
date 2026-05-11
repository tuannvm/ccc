package listen

import (
	"strings"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestBuildCLIProviderStatusCurrentSession(t *testing.T) {
	cfg := &configpkg.Config{
		ActiveProvider: "openai",
		Providers: map[string]*configpkg.ProviderConfig{
			"openai": {},
			"zai":    {},
		},
	}
	info := &configpkg.SessionInfo{ProviderName: "zai"}

	msg := BuildCLIProviderStatus(cfg, "demo", info)
	for _, want := range []string{
		"session: demo",
		"provider: zai",
		"source: session",
		"- zai (current)",
		"change: ccc provider <name>",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("provider status missing %q:\n%s", want, msg)
		}
	}
}

func TestBuildCLIProviderStatusGlobal(t *testing.T) {
	cfg := &configpkg.Config{ActiveProvider: "openai", Providers: map[string]*configpkg.ProviderConfig{"openai": {}}}
	msg := BuildCLIProviderStatus(cfg, "", nil)
	for _, want := range []string{
		"providers",
		"- codex (builtin)",
		"- openai (active)",
		"run inside a mapped project directory",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("global provider status missing %q:\n%s", want, msg)
		}
	}
}
