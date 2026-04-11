package auth

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tuannvm/ccc/pkg/config"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// Auth state for OAuth flow
var (
	authInProgress sync.Mutex
	authWaitingCode atomic.Bool
	otpAttempts = make(map[string]int) // session -> failed attempts
)

// AuthTmuxSession is the tmux session name for auth flow
const AuthTmuxSession = "claude-auth"

// HandleAuth starts the OAuth authentication flow via tmux
func HandleAuth(cfg *config.Config, chatID, threadID int64) {
	if !authInProgress.TryLock() {
		telegram.SendMessage(cfg, chatID, threadID, "⚠️ Auth already in progress")
		return
	}

	telegram.SendMessage(cfg, chatID, threadID, "🔐 Starting Claude auth...")

	tmux.KillSession(AuthTmuxSession)
	time.Sleep(500 * time.Millisecond)

	home, _ := os.UserHomeDir()
	if err := exec.Command(tmux.TmuxPath, "new-session", "-d", "-s", AuthTmuxSession, "-c", home).Run(); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to create tmux session: %v", err))
		authInProgress.Unlock()
		return
	}

	// Build environment string for tmux from provider config
	envCmd := buildProviderEnv(cfg, home)

	time.Sleep(500 * time.Millisecond)
	cmdStr := envCmd + shellQuote(tmux.ClaudePath) + " --dangerously-skip-permissions"
	exec.Command(tmux.TmuxPath, "send-keys", "-t", AuthTmuxSession, cmdStr, "C-m").Run()

	var oauthURL string
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		out, err := exec.Command(tmux.TmuxPath, "capture-pane", "-t", AuthTmuxSession, "-p", "-S", "-30").Output()
		if err != nil {
			continue
		}
		pane := string(out)

		if strings.Contains(pane, "Dark mode") || strings.Contains(pane, "❯") || strings.Contains(pane, "Welcome back") {
			telegram.SendMessage(cfg, chatID, threadID, "✅ Claude is already authenticated!")
			tmux.KillSession(AuthTmuxSession)
			authInProgress.Unlock()
			return
		}

		if strings.Contains(pane, "claude.ai/oauth/authorize") {
			oauthURL = extractOAuthURL(pane)
			break
		}
	}

	if oauthURL == "" {
		telegram.SendMessage(cfg, chatID, threadID, "❌ Could not find OAuth URL. Try again.")
		tmux.KillSession(AuthTmuxSession)
		authInProgress.Unlock()
		return
	}

	authWaitingCode.Store(true)
	telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("🔗 Open this URL and authorize:\n\n%s\n\nThen paste the code here.", oauthURL))
}

// HandleAuthCode processes an OAuth code submission
func HandleAuthCode(cfg *config.Config, chatID, threadID int64, code string) {
	authWaitingCode.Store(false)
	code = strings.TrimSpace(code)

	telegram.SendMessage(cfg, chatID, threadID, "🔄 Sending code to Claude...")

	exec.Command(tmux.TmuxPath, "send-keys", "-t", AuthTmuxSession, "-l", code).Run()
	time.Sleep(200 * time.Millisecond)
	exec.Command(tmux.TmuxPath, "send-keys", "-t", AuthTmuxSession, "C-m").Run()

	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		out, _ := exec.Command(tmux.TmuxPath, "capture-pane", "-t", AuthTmuxSession, "-p").Output()
		pane := string(out)

		if strings.Contains(pane, "Yes, I accept") {
			exec.Command(tmux.TmuxPath, "send-keys", "-t", AuthTmuxSession, "Down").Run()
			time.Sleep(200 * time.Millisecond)
			exec.Command(tmux.TmuxPath, "send-keys", "-t", AuthTmuxSession, "C-m").Run()
			continue
		}

		if strings.Contains(pane, "Press Enter") || strings.Contains(pane, "Enter to confirm") {
			exec.Command(tmux.TmuxPath, "send-keys", "-t", AuthTmuxSession, "C-m").Run()
			continue
		}

		if strings.Contains(pane, "❯") {
			telegram.SendMessage(cfg, chatID, threadID, "✅ Auth successful! Claude is ready.")
			tmux.KillSession(AuthTmuxSession)
			authInProgress.Unlock()
			return
		}
	}

	out, _ := exec.Command(tmux.TmuxPath, "capture-pane", "-t", AuthTmuxSession, "-p").Output()
	pane := string(out)
	if strings.Contains(pane, "Login successful") || strings.Contains(pane, "❯") {
		telegram.SendMessage(cfg, chatID, threadID, "✅ Auth successful!")
	} else {
		telegram.SendMessage(cfg, chatID, threadID, "⚠️ Auth may have failed. Check VPS manually.")
	}

	tmux.KillSession(AuthTmuxSession)
	authInProgress.Unlock()
}

// IsAuthWaitingCode returns whether auth is waiting for a code submission
func IsAuthWaitingCode() bool {
	return authWaitingCode.Load()
}

// HandleOTPResponse handles OTP code responses for permission approval.
// Returns true if the text was handled as an OTP response.
func HandleOTPResponse(cfg *config.Config, text string, chatID, threadID int64) bool {
	if !IsOTPEnabled(cfg) || strings.HasPrefix(text, "/") {
		return false
	}

	pendingSession := FindPendingOTPSession()
	if pendingSession == "" {
		return false
	}

	code := strings.TrimSpace(text)
	if ValidateOTP(cfg.OTPSecret, code) {
		WriteOTPResponse(pendingSession, true)
		delete(otpAttempts, pendingSession)
		telegram.SendMessage(cfg, chatID, threadID, "✅ Permission approved (valid for 5 min)")
	} else {
		otpAttempts[pendingSession]++
		remaining := 5 - otpAttempts[pendingSession]
		if remaining <= 0 {
			WriteOTPResponse(pendingSession, false)
			delete(otpAttempts, pendingSession)
			telegram.SendMessage(cfg, chatID, threadID, "❌ Too many failed attempts - permission denied")
		} else {
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Invalid code — %d attempts remaining", remaining))
		}
	}
	return true
}

// shellQuote wraps a string in single quotes, escaping any embedded single quotes
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// buildProviderEnv constructs environment variable exports from provider config
func buildProviderEnv(cfg *config.Config, home string) string {
	envCmd := ""
	provider := providerpkg.GetActiveProvider(cfg)
	if provider != nil {
		authToken := provider.AuthToken
		if authToken == "" && provider.AuthEnvVar != "" {
			authToken = os.Getenv(provider.AuthEnvVar)
		}
		if authToken != "" {
			envCmd += "export ANTHROPIC_AUTH_TOKEN=" + shellQuote(authToken) + "; "
		}

		if provider.BaseURL != "" {
			envCmd += "export ANTHROPIC_BASE_URL=" + shellQuote(provider.BaseURL) + "; "
		}
		if provider.OpusModel != "" {
			envCmd += "export ANTHROPIC_DEFAULT_OPUS_MODEL=" + shellQuote(provider.OpusModel) + "; "
		}
		if provider.SonnetModel != "" {
			envCmd += "export ANTHROPIC_DEFAULT_SONNET_MODEL=" + shellQuote(provider.SonnetModel) + "; "
			envCmd += "export ANTHROPIC_MODEL=" + shellQuote(provider.SonnetModel) + "; "
		} else if provider.OpusModel != "" {
			envCmd += "export ANTHROPIC_MODEL=" + shellQuote(provider.OpusModel) + "; "
		}
		if provider.HaikuModel != "" {
			envCmd += "export ANTHROPIC_DEFAULT_HAIKU_MODEL=" + shellQuote(provider.HaikuModel) + "; "
		}
		if provider.SubagentModel != "" {
			envCmd += "export CLAUDE_CODE_SUBAGENT_MODEL=" + shellQuote(provider.SubagentModel) + "; "
		}

		if provider.ConfigDir != "" {
			configDir := provider.ConfigDir
			if strings.HasPrefix(configDir, "~/") {
				configDir = home + configDir[1:]
			} else if configDir == "~" {
				configDir = home
			}
			envCmd += "export CLAUDE_CONFIG_DIR=" + shellQuote(configDir) + "; "
		}

		if provider.ApiTimeout > 0 {
			envCmd += fmt.Sprintf("export API_TIMEOUT_MS=%d; ", provider.ApiTimeout)
		}
	}
	return envCmd
}

// extractOAuthURL extracts the OAuth URL from tmux pane output
func extractOAuthURL(pane string) string {
	var oauthURL string
	lines := strings.Split(pane, "\n")
	capturing := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://claude.ai/oauth/") {
			oauthURL = line
			capturing = true
		} else if capturing && line != "" && !strings.Contains(line, "Paste code") && !strings.Contains(line, "Browser") {
			oauthURL += line
		} else if capturing {
			capturing = false
		}
	}
	return oauthURL
}
