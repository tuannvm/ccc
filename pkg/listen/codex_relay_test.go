package listen

import (
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestLegacyEmptyProviderSessionDoesNotUseCodexRelayFromActiveDefault(t *testing.T) {
	cfg := &configpkg.Config{
		ActiveProvider: "codex",
		CodexRelay:     configpkg.CodexRelayConfig{Enabled: true},
	}
	info := &configpkg.SessionInfo{}
	providerName := effectiveProviderName(cfg, info)
	if providerName != builtinProviderName {
		t.Fatalf("providerName = %q, want %q", providerName, builtinProviderName)
	}
	if useCodexRelay(cfg, providerName) {
		t.Fatal("legacy empty-provider session should not use Codex relay from active default")
	}
}
