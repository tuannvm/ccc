package listen

import (
	"strings"
	"sync"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

func TestNewProviderButtonsForAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &configpkg.Config{
		ActiveProvider: "codex",
		Providers: map[string]*configpkg.ProviderConfig{
			"codex-anthropic": {Backend: "codex", SonnetModel: "claude-opus-4-7", BaseURL: "http://127.0.0.1:8317/v1", ConfigDir: "~/.codex-anthropic"},
			"openai":          {SonnetModel: "gpt-5.5"},
			"zai":             {SonnetModel: "glm-4.6"},
		},
	}

	codexButtons := newProviderButtonsForAgent(cfg, "demo", "codex")
	if len(codexButtons) != 2 {
		t.Fatalf("codex buttons len = %d, want 2", len(codexButtons))
	}
	if got := codexButtons[0][0].Text; got != "Codex default ⭐" {
		t.Fatalf("codex button label = %q, want Codex default ⭐", got)
	}
	if got := codexButtons[1][0].Text; got != "codex-anthropic · claude-opus-4-7" {
		t.Fatalf("codex-anthropic button label = %q", got)
	}
	callback := codexButtons[0][0].CallbackData
	if !strings.HasPrefix(callback, "new:") || len(callback) > 64 {
		t.Fatalf("codex callback = %q, want compact new callback", callback)
	}
	record, ok := loadNewSessionCallback(strings.TrimPrefix(callback, "new:"))
	if !ok {
		t.Fatalf("callback token was not persisted: %q", callback)
	}
	if record.Action != "provider" || record.SessionName != "demo" || record.AgentName != "codex" || record.ProviderName != "codex" {
		t.Fatalf("callback record = %#v", record)
	}
}

func TestNewProviderButtonsDefaultToCodexWhenNoActiveProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &configpkg.Config{}
	buttons := newProviderButtonsForAgent(cfg, strings.Repeat("long-session-name-", 8), "codex")
	if len(buttons) == 0 {
		t.Fatal("codex provider buttons are empty")
	}
	if got := buttons[0][0].Text; got != "Codex default ⭐" {
		t.Fatalf("default codex label = %q, want Codex default ⭐", got)
	}
}

func TestNewProviderButtonsDefaultToCodexWhenActiveProviderIsClaude(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &configpkg.Config{
		ActiveProvider: "anthropic",
		Providers: map[string]*configpkg.ProviderConfig{
			"codex-anthropic": {Backend: "codex", SonnetModel: "claude-opus-4-7"},
			"openai":          {SonnetModel: "gpt-5.5"},
		},
	}

	buttons := newProviderButtonsForAgent(cfg, "demo", "codex")
	if len(buttons) != 2 {
		t.Fatalf("codex buttons len = %d, want 2", len(buttons))
	}
	if got := buttons[0][0].Text; got != "Codex default ⭐" {
		t.Fatalf("default codex label = %q, want Codex default ⭐", got)
	}
	if got := buttons[1][0].Text; strings.Contains(got, "⭐") {
		t.Fatalf("configured codex label = %q, want no star", got)
	}
}

func TestNewProviderButtonsPreferActiveCodexProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &configpkg.Config{
		ActiveProvider: "codex-anthropic",
		Providers: map[string]*configpkg.ProviderConfig{
			"codex-anthropic": {Backend: "codex", SonnetModel: "claude-opus-4-7"},
		},
	}

	buttons := newProviderButtonsForAgent(cfg, "demo", "codex")
	if len(buttons) != 2 {
		t.Fatalf("codex buttons len = %d, want 2", len(buttons))
	}
	if got := buttons[0][0].Text; strings.Contains(got, "⭐") {
		t.Fatalf("builtin codex label = %q, want no star", got)
	}
	if got := buttons[1][0].Text; got != "codex-anthropic · claude-opus-4-7 ⭐" {
		t.Fatalf("active configured codex label = %q, want codex-anthropic · claude-opus-4-7 ⭐", got)
	}
}

func TestEnsureNewSessionsMapInitializesMissingMap(t *testing.T) {
	cfg := &configpkg.Config{}
	ensureNewSessionsMap(cfg)
	if cfg.Sessions == nil {
		t.Fatal("Sessions map was not initialized")
	}
}

func TestSaveNewSessionCallbackConcurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const count = 25
	tokens := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := saveNewSessionCallback(newSessionCallback{
				Action:      "agent",
				SessionName: "demo",
				AgentName:   "codex",
			})
			if err != nil {
				errs <- err
				return
			}
			tokens <- token
		}()
	}
	wg.Wait()
	close(tokens)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("saveNewSessionCallback failed: %v", err)
		}
	}

	seen := make(map[string]bool)
	for token := range tokens {
		if seen[token] {
			t.Fatalf("duplicate token saved: %s", token)
		}
		seen[token] = true
		record, ok := loadNewSessionCallback(token)
		if !ok {
			t.Fatalf("token was not persisted: %s", token)
		}
		if record.Action != "agent" || record.SessionName != "demo" || record.AgentName != "codex" {
			t.Fatalf("callback record = %#v", record)
		}
	}
	if len(seen) != count {
		t.Fatalf("saved %d tokens, want %d", len(seen), count)
	}
}
