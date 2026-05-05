package provider

import (
	"slices"
	"testing"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// TestProviderResolution tests provider lookup functions
func TestProviderResolution(t *testing.T) {
	config := &configpkg.Config{
		ActiveProvider: "custom-provider",
		Providers: map[string]*configpkg.ProviderConfig{
			"anthropic": {
				BaseURL:    "https://api.anthropic.com",
				AuthEnvVar: "ANTHROPIC_API_KEY",
			},
			"custom-provider": {
				BaseURL:     "https://custom.ai",
				AuthToken:   "custom-key",
				SonnetModel: "custom-sonnet",
			},
		},
	}

	// Test GetActiveProvider
	active := GetActiveProvider(config)
	if active == nil {
		t.Error("GetActiveProvider returned nil")
	} else if active.BaseURL != "https://custom.ai" {
		t.Errorf("GetActiveProvider().BaseURL: got %q, want 'https://custom.ai'", active.BaseURL)
	}

	// Test GetProvider with specific name
	p := GetProvider(config, "anthropic")
	if p == nil {
		t.Error("GetProvider('anthropic') returned nil")
	} else if p.Name() != "anthropic" {
		t.Errorf("GetProvider('anthropic').Name(): got %q, want 'anthropic'", p.Name())
	}

	// Test GetProvider with empty string (should return active)
	p = GetProvider(config, "")
	if p == nil {
		t.Error("GetProvider('') returned nil")
	} else if p.Name() != "custom-provider" {
		t.Errorf("GetProvider('').Name(): got %q, want 'custom-provider'", p.Name())
	}

	// Test GetProviderNames
	names := GetProviderNames(config)
	if len(names) != 3 {
		t.Errorf("GetProviderNames length: got %d, want 3", len(names))
	}
	if !slices.Contains(names, "anthropic") {
		t.Error("'anthropic' not in provider names (should always be included)")
	}
	if !slices.Contains(names, "codex") {
		t.Error("'codex' not in provider names (should always be included)")
	}
}

func TestCodexProviderResolution(t *testing.T) {
	cfg := &configpkg.Config{ActiveProvider: "codex"}
	p := GetProvider(cfg, "")
	if p == nil {
		t.Fatal("GetProvider('') returned nil")
	}
	if p.Name() != "codex" {
		t.Fatalf("active codex provider name = %q, want codex", p.Name())
	}
	if p.Backend() != BackendCodex {
		t.Fatalf("active codex backend = %q, want %q", p.Backend(), BackendCodex)
	}

	p = GetProvider(&configpkg.Config{}, "codex")
	if p == nil || p.Backend() != BackendCodex {
		t.Fatalf("explicit codex provider = %#v, want codex backend", p)
	}
}

func TestConfiguredCodexProviderResolution(t *testing.T) {
	cfg := &configpkg.Config{
		Providers: map[string]*configpkg.ProviderConfig{
			"codex-anthropic": {
				Backend:     BackendCodex,
				BaseURL:     "http://127.0.0.1:8317/v1",
				SonnetModel: "claude-opus-4-7",
				ConfigDir:   "~/.codex-anthropic",
			},
		},
	}
	p := GetProvider(cfg, "codex-anthropic")
	if p == nil {
		t.Fatal("GetProvider(codex-anthropic) returned nil")
	}
	if p.Name() != "codex-anthropic" || p.Backend() != BackendCodex {
		t.Fatalf("provider = %s/%s, want codex-anthropic/%s", p.Name(), p.Backend(), BackendCodex)
	}
	if p.BaseURL() != "http://127.0.0.1:8317/v1" {
		t.Fatalf("BaseURL = %q", p.BaseURL())
	}
	if p.Models().Sonnet != "claude-opus-4-7" {
		t.Fatalf("Sonnet = %q", p.Models().Sonnet)
	}
	if !slices.Contains(GetProviderNames(cfg), "codex-anthropic") {
		t.Fatalf("codex-anthropic missing from provider names: %v", GetProviderNames(cfg))
	}
}
