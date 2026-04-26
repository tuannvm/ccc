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
