package listen

import (
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestEffectiveProviderName(t *testing.T) {
	cfg := &configpkg.Config{ActiveProvider: "openai"}
	if got := effectiveProviderName(cfg, nil); got != "openai" {
		t.Fatalf("effectiveProviderName() = %q, want openai", got)
	}

	info := &configpkg.SessionInfo{ProviderName: "zai"}
	if got := effectiveProviderName(cfg, info); got != "zai" {
		t.Fatalf("effectiveProviderName(session) = %q, want zai", got)
	}

	if got := effectiveProviderName(&configpkg.Config{}, nil); got != "anthropic" {
		t.Fatalf("effectiveProviderName(default) = %q, want anthropic", got)
	}
}

func TestProviderSource(t *testing.T) {
	if got := providerSource(&configpkg.Config{ActiveProvider: "openai"}, nil); got != "active default" {
		t.Fatalf("providerSource(active) = %q, want active default", got)
	}
	if got := providerSource(&configpkg.Config{}, nil); got != "builtin default" {
		t.Fatalf("providerSource(default) = %q, want builtin default", got)
	}
	if got := providerSource(&configpkg.Config{ActiveProvider: "openai"}, &configpkg.SessionInfo{ProviderName: "zai"}); got != "session" {
		t.Fatalf("providerSource(session) = %q, want session", got)
	}
}

func TestProviderModelOptionLabel(t *testing.T) {
	cfg := &configpkg.Config{
		Providers: map[string]*configpkg.ProviderConfig{
			"openai": {SonnetModel: "gpt-5.5"},
			"zai":    {OpusModel: "glm-4.6"},
		},
	}
	if got := providerModelOptionLabel(cfg, "anthropic"); got != "Anthropic default" {
		t.Fatalf("anthropic label = %q", got)
	}
	if got := providerModelOptionLabel(cfg, "codex"); got != "Codex default" {
		t.Fatalf("codex label = %q", got)
	}
	if got := providerModelOptionLabel(cfg, "openai"); got != "openai · gpt-5.5" {
		t.Fatalf("openai label = %q", got)
	}
	if got := providerModelOptionLabel(cfg, "zai"); got != "zai · glm-4.6" {
		t.Fatalf("zai label = %q", got)
	}
}

func TestAgentOptionLabel(t *testing.T) {
	if got := agentOptionLabel("claude"); got != "Claude Code" {
		t.Fatalf("claude label = %q", got)
	}
	if got := agentOptionLabel("codex"); got != "Codex CLI" {
		t.Fatalf("codex label = %q", got)
	}
	if got := agentOptionLabel("anthropic"); got != "Claude" {
		t.Fatalf("anthropic label = %q", got)
	}
}
