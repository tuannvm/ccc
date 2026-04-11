package tmux

import (
	"bytes"
	"fmt"
	"io"
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

// RunWithArgs parses args and runs claude in a tmux session.
// Convenience wrapper combining ParseRunArgs + RunClaudeRaw.
func RunWithArgs(args []string, autoGen string, ensureHooks EnsureHooksFunc) error {
	ra := ParseRunArgs(args, autoGen)
	return RunClaudeRaw(ra.ContinueSession, ra.ResumeSessionID, ra.ProviderOverride, ra.WorktreeName, autoGen, ensureHooks)
}

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
			telegramFlagPath := filepath.Join(configpkg.CacheDir(), "telegram-active-"+SafeName(name))
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

	// When resuming, capture stderr to detect "No conversation found" for auto-retry.
	// Use a multi-writer to both capture and display stderr.
	var stderrBuf bytes.Buffer
	if resumeSessionID != "" {
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	} else {
		cmd.Stderr = os.Stderr
	}
	cmd.Env = os.Environ()

	cwd, _ := os.Getwd()
	configDirCode := os.Getenv("CLAUDE_CODE_CONFIG_DIR")
	configDirZai := os.Getenv("CLAUDE_CONFIG_DIR")
	homeDir := os.Getenv("HOME")
	loggingpkg.ListenLog("runClaudeRaw: claude=%s args=%v cwd=%s config_code_dir=%q config_dir=%q home=%q", ClaudePath, args, cwd, configDirCode, configDirZai, homeDir)

	config, loadErr := configpkg.Load()
	if loadErr == nil {
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

	runErr := cmd.Run()

	// If --resume failed because the session doesn't exist, clear the stale ID and retry fresh
	staleSession := runErr != nil && resumeSessionID != "" && strings.Contains(stderrBuf.String(), "No conversation found")
	if staleSession {
		sessionName := filepath.Base(cwd)
		loggingpkg.ListenLog("Resume failed for session %s, clearing stale session ID and starting fresh", resumeSessionID)
		if config != nil {
			cleared := false
			// Clear from single sessions (try base name and worktree name pattern)
			if config.Sessions != nil {
				// Try exact session name
				if info, exists := config.Sessions[sessionName]; exists && info != nil && info.ClaudeSessionID != "" {
					info.ClaudeSessionID = ""
					cleared = true
				}
				// Try worktree session name pattern: basename_worktreeName
				if worktreeName != "" {
					worktreeSessName := sessionName + "_" + worktreeName
					if info, exists := config.Sessions[worktreeSessName]; exists && info != nil && info.ClaudeSessionID != "" {
						info.ClaudeSessionID = ""
						cleared = true
					}
				}
				// Fallback: scan all sessions for matching stale ID
				if !cleared {
					for _, info := range config.Sessions {
						if info != nil && info.ClaudeSessionID == resumeSessionID {
							info.ClaudeSessionID = ""
							cleared = true
						}
					}
				}
			}
			// Clear from team session panes (stale ID may be stored in a pane)
			if config.TeamSessions != nil {
				for _, info := range config.TeamSessions {
					if info != nil && info.Panes != nil {
						for _, pane := range info.Panes {
							if pane != nil && pane.ClaudeSessionID == resumeSessionID {
								pane.ClaudeSessionID = ""
								cleared = true
							}
						}
					}
				}
			}
			if cleared {
				configpkg.Save(config)
			}
		}
		return RunClaudeRaw(false, "", providerOverride, worktreeName, worktreeAutoGen, ensureHooks)
	}

	return runErr
}
