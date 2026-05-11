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
		ActiveProvider: "openai",
		Providers: map[string]*configpkg.ProviderConfig{
			"codex-anthropic": {Backend: "codex", SonnetModel: "claude-opus-4-7", BaseURL: "http://127.0.0.1:8317/v1", ConfigDir: "~/.codex-anthropic"},
			"openai":          {SonnetModel: "gpt-5.5"},
			"zai":             {SonnetModel: "glm-4.6"},
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
	if len(codexButtons) != 2 {
		t.Fatalf("codex buttons len = %d, want 2", len(codexButtons))
	}
	if got := codexButtons[0][0].Text; got != "Codex default" {
		t.Fatalf("codex button label = %q, want Codex default", got)
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

func TestNewAgentButtonsUseCompactCallbacks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &configpkg.Config{}
	buttons := newAgentButtons(cfg, strings.Repeat("long-session-name-", 8))
	if len(buttons) != 2 {
		t.Fatalf("agent buttons len = %d, want 2", len(buttons))
	}
	for _, row := range buttons {
		if len(row) != 1 {
			t.Fatalf("agent row has %d buttons, want 1", len(row))
		}
		callback := row[0].CallbackData
		if !strings.HasPrefix(callback, "new:") || len(callback) > 64 {
			t.Fatalf("agent callback = %q, want compact callback", callback)
		}
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

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
