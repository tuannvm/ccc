package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
)

// EnsureHooksFunc is a callback for ensuring hooks are installed for a session.
// This avoids import cycles (pkg/hooks imports pkg/tmux, so pkg/tmux can't import pkg/hooks).
type EnsureHooksFunc func(cfg *configpkg.Config, sessionName string, info *configpkg.SessionInfo) error

// RunClaudeRaw runs claude directly (used inside tmux sessions).
// providerOverride: if non-empty, specifies which provider to use instead of active_provider
// resumeSessionID: if non-empty, resumes a specific session by ID
// worktreeName: if non-empty, creates/uses a git worktree session
// worktreeAutoGen: special value that triggers auto-generation of worktree name
// ensureHooks: callback to ensure hooks are installed (passed from root to avoid import cycles)
func RunClaudeRaw(continueSession bool, resumeSessionID string, providerOverride string, worktreeName string, worktreeAutoGen string, ensureHooks EnsureHooksFunc) error {
	if ClaudePath == "" {
		return fmt.Errorf("claude binary not found")
	}

	// Clean stale Telegram flag from previous sessions
	if winName, err := exec.Command(TmuxPath, "display-message", "-p", "#{window_name}").Output(); err == nil {
		name := strings.TrimSpace(string(winName))
		if name != "" {
			telegramFlagPath := filepath.Join(configpkg.CacheDir(), "telegram-active-"+name)
			os.Remove(telegramFlagPath)
		}
	}

	var args []string
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	} else if continueSession {
		args = append(args, "-c")
	}
	if worktreeName != "" {
		if worktreeName == worktreeAutoGen {
			args = append(args, "--worktree")
		} else {
			args = append(args, "--worktree", worktreeName)
		}
	}

	cmd := exec.Command(ClaudePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	cwd, _ := os.Getwd()
	configDirCode := os.Getenv("CLAUDE_CODE_CONFIG_DIR")
	configDirZai := os.Getenv("CLAUDE_CONFIG_DIR")
	homeDir := os.Getenv("HOME")
	loggingpkg.ListenLog("runClaudeRaw: claude=%s args=%v cwd=%s config_code_dir=%q config_dir=%q home=%q", ClaudePath, args, cwd, configDirCode, configDirZai, homeDir)

	config, err := configpkg.Load()
	if err == nil {
		sessionName := filepath.Base(cwd)
		sessionInfo := &configpkg.SessionInfo{Path: cwd}
		if config.Sessions != nil {
			if existing := config.Sessions[sessionName]; existing != nil {
				sessionInfo = existing
			}
		}
		if ensureHooks != nil {
			if err := ensureHooks(config, sessionName, sessionInfo); err != nil {
				loggingpkg.ListenLog("Warning: Failed to ensure hooks in %s: %v", cwd, err)
			}
		}

		provider := providerpkg.GetProvider(config, providerOverride)

		if providerOverride != "" && provider == nil {
			return fmt.Errorf("unknown provider: %s (available providers: %v)", providerOverride, providerpkg.GetProviderNames(config))
		}

		shouldApplyProviderEnv := (resumeSessionID == "") || (providerOverride != "")

		if err := providerpkg.EnsureProviderSettings(provider); err != nil {
			loggingpkg.ListenLog("Failed to update provider settings: %v", err)
		}

		if shouldApplyProviderEnv {
			cmd.Env = providerpkg.ApplyProviderEnv(cmd.Env, provider, config)
			loggingpkg.ListenLog("Applying provider env: providerOverride=%q resumeSessionID=%q provider=%q", providerOverride, resumeSessionID, provider.Name())
		} else {
			loggingpkg.ListenLog("Preserving original session environment for resumeSessionID=%q", resumeSessionID)
		}

		if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" && config.OAuthToken != "" {
			cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+config.OAuthToken)
		}
	}

	return cmd.Run()
}
