package provider

import (
	"os"
	"strings"

	"github.com/tuannvm/ccc/pkg/config"
)

// ApplyProviderEnv applies provider-specific environment variables to cmd.Env.
// Returns the modified environment slice.
func ApplyProviderEnv(baseEnv []string, p Provider, cfg *config.Config) []string {
	if p == nil {
		return baseEnv
	}

	env := baseEnv

	// Get provider variables and track which ones we'll actually set
	providerVars := p.EnvVars(cfg)

	// For ConfiguredProvider with auth, we need to check if auth_env_var expands to non-empty
	// If it expands to empty, we should preserve ambient credentials instead
	shouldUnsetAuth := false
	if !p.IsBuiltin() {
		for _, v := range providerVars {
			if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=$") {
				envVarName := strings.TrimPrefix(v, "ANTHROPIC_AUTH_TOKEN=$")
				if envVal := os.Getenv(envVarName); envVal != "" {
					shouldUnsetAuth = true
				}
				break
			} else if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=") && !strings.Contains(v, "$") {
				shouldUnsetAuth = true
				break
			}
		}
	}

	// Unset auth vars only if we're actually replacing them
	if shouldUnsetAuth {
		env = unsetEnvVars(env, []string{
			"ANTHROPIC_API_KEY",
			"CLAUDE_API_KEY",
			"ANTHROPIC_AUTH_TOKEN",
		})
		env = unsetEnvVars(env, []string{
			"ANTHROPIC_BASE_URL",
			"ANTHROPIC_MODEL",
			"ANTHROPIC_DEFAULT_OPUS_MODEL",
			"ANTHROPIC_DEFAULT_SONNET_MODEL",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL",
			"CLAUDE_CODE_SUBAGENT_MODEL",
		})
	}

	// Add provider-specific environment variables
	for _, v := range providerVars {
		if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=$") {
			envVarName := strings.TrimPrefix(v, "ANTHROPIC_AUTH_TOKEN=$")
			if envVal := os.Getenv(envVarName); envVal != "" {
				env = append(env, "ANTHROPIC_AUTH_TOKEN="+envVal)
			}
		} else {
			env = append(env, v)
		}
	}

	// Common settings for all providers
	env = append(env, []string{
		"TMPDIR=/tmp/claude",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DISABLE_COST_WARNINGS=1",
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	}...)

	return env
}

// unsetEnvVars removes specified environment variables from env slice
func unsetEnvVars(env []string, keys []string) []string {
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	var result []string
	for _, e := range env {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			result = append(result, e)
			continue
		}
		key := e[:idx]
		if !keyMap[key] {
			result = append(result, e)
		}
	}
	return result
}
