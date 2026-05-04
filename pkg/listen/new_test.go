package listen

import (
	"strings"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestNewProviderButtonsForAgent(t *testing.T) {
	cfg := &configpkg.Config{
		ActiveProvider: "openai",
		Providers: map[string]*configpkg.ProviderConfig{
			"openai": {SonnetModel: "gpt-5.5"},
			"zai":    {SonnetModel: "glm-4.6"},
		},
	}

	claudeButtons := newProviderButtonsForAgent(cfg, "demo", "claude")
	if len(claudeButtons) != 3 {
		t.Fatalf("claude buttons len = %d, want 3", len(claudeButtons))
	}
	var claudeLabels []string
	for _, row := range claudeButtons {
		claudeLabels = append(claudeLabels, row[0].Text)
		if strings.Contains(row[0].CallbackData, ":codex") {
			t.Fatalf("claude provider choices included codex callback: %s", row[0].CallbackData)
		}
	}
	if !containsLabel(claudeLabels, "openai · gpt-5.5 ⭐") {
		t.Fatalf("claude labels missing active model: %v", claudeLabels)
	}

	codexButtons := newProviderButtonsForAgent(cfg, "demo", "codex")
	if len(codexButtons) != 1 {
		t.Fatalf("codex buttons len = %d, want 1", len(codexButtons))
	}
	if got := codexButtons[0][0].Text; got != "Codex default" {
		t.Fatalf("codex button label = %q, want Codex default", got)
	}
	if got := codexButtons[0][0].CallbackData; got != "new-provider:demo:codex" {
		t.Fatalf("codex callback = %q", got)
	}
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
