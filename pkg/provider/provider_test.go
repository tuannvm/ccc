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
	if len(names) != 2 {
		t.Errorf("GetProviderNames length: got %d, want 2", len(names))
	}
	if !slices.Contains(names, "anthropic") {
		t.Error("'anthropic' not in provider names (should always be included)")
	}
}
