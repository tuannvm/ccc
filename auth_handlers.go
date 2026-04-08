package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// Auth handlers for OAuth flow and help text.

const authTmuxSession = "claude-auth"

func handleAuth(config *Config, chatID, threadID int64) {
	if !authInProgress.TryLock() {
		telegram.SendMessage(config, chatID, threadID, "⚠️ Auth already in progress")
		return
	}

	telegram.SendMessage(config, chatID, threadID, "🔐 Starting Claude auth...")

	tmux.KillSession(authTmuxSession)
	time.Sleep(500 * time.Millisecond)

	home, _ := os.UserHomeDir()
	if err := exec.Command(tmux.TmuxPath, "new-session", "-d", "-s", authTmuxSession, "-c", home).Run(); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create tmux session: %v", err))
		authInProgress.Unlock()
		return
	}

	// Build environment string for tmux from provider config
	envCmd := ""
	provider := providerpkg.GetActiveProvider(config)
	if provider != nil {
		authToken := provider.AuthToken
		if authToken == "" && provider.AuthEnvVar != "" {
			authToken = os.Getenv(provider.AuthEnvVar)
		}
		if authToken != "" {
			envCmd += fmt.Sprintf("export ANTHROPIC_AUTH_TOKEN='%s'; ", authToken)
		}

		if provider.BaseURL != "" {
			envCmd += fmt.Sprintf("export ANTHROPIC_BASE_URL='%s'; ", provider.BaseURL)
		}
		if provider.OpusModel != "" {
			envCmd += fmt.Sprintf("export ANTHROPIC_DEFAULT_OPUS_MODEL='%s'; ", provider.OpusModel)
		}
		if provider.SonnetModel != "" {
			envCmd += fmt.Sprintf("export ANTHROPIC_DEFAULT_SONNET_MODEL='%s'; ", provider.SonnetModel)
			envCmd += fmt.Sprintf("export ANTHROPIC_MODEL='%s'; ", provider.SonnetModel)
		} else if provider.OpusModel != "" {
			envCmd += fmt.Sprintf("export ANTHROPIC_MODEL='%s'; ", provider.OpusModel)
		}
		if provider.HaikuModel != "" {
			envCmd += fmt.Sprintf("export ANTHROPIC_DEFAULT_HAIKU_MODEL='%s'; ", provider.HaikuModel)
		}
		if provider.SubagentModel != "" {
			envCmd += fmt.Sprintf("export CLAUDE_CODE_SUBAGENT_MODEL='%s'; ", provider.SubagentModel)
		}

		if provider.ConfigDir != "" {
			configDir := provider.ConfigDir
			if strings.HasPrefix(configDir, "~/") {
				configDir = home + configDir[1:]
			} else if configDir == "~" {
				configDir = home
			}
			envCmd += fmt.Sprintf("export CLAUDE_CONFIG_DIR='%s'; ", configDir)
		}

		if provider.ApiTimeout > 0 {
			envCmd += fmt.Sprintf("export API_TIMEOUT_MS=%d; ", provider.ApiTimeout)
		}
	}

	time.Sleep(500 * time.Millisecond)
	cmdStr := envCmd + tmux.ClaudePath + " --dangerously-skip-permissions"
	exec.Command(tmux.TmuxPath, "send-keys", "-t", authTmuxSession, cmdStr, "C-m").Run()

	var oauthURL string
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		out, err := exec.Command(tmux.TmuxPath, "capture-pane", "-t", authTmuxSession, "-p", "-S", "-30").Output()
		if err != nil {
			continue
		}
		pane := string(out)

		if strings.Contains(pane, "Dark mode") || strings.Contains(pane, "❯") || strings.Contains(pane, "Welcome back") {
			telegram.SendMessage(config, chatID, threadID, "✅ Claude is already authenticated!")
			tmux.KillSession(authTmuxSession)
			authInProgress.Unlock()
			return
		}

		if strings.Contains(pane, "claude.ai/oauth/authorize") {
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
			break
		}
	}

	if oauthURL == "" {
		telegram.SendMessage(config, chatID, threadID, "❌ Could not find OAuth URL. Try again.")
		tmux.KillSession(authTmuxSession)
		authInProgress.Unlock()
		return
	}

	authWaitingCode = true
	telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("🔗 Open this URL and authorize:\n\n%s\n\nThen paste the code here.", oauthURL))
}

func handleAuthCode(config *Config, chatID, threadID int64, code string) {
	authWaitingCode = false
	code = strings.TrimSpace(code)

	telegram.SendMessage(config, chatID, threadID, "🔄 Sending code to Claude...")

	exec.Command(tmux.TmuxPath, "send-keys", "-t", authTmuxSession, "-l", code).Run()
	time.Sleep(200 * time.Millisecond)
	exec.Command(tmux.TmuxPath, "send-keys", "-t", authTmuxSession, "C-m").Run()

	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		out, _ := exec.Command(tmux.TmuxPath, "capture-pane", "-t", authTmuxSession, "-p").Output()
		pane := string(out)

		if strings.Contains(pane, "Yes, I accept") {
			exec.Command(tmux.TmuxPath, "send-keys", "-t", authTmuxSession, "Down").Run()
			time.Sleep(200 * time.Millisecond)
			exec.Command(tmux.TmuxPath, "send-keys", "-t", authTmuxSession, "C-m").Run()
			continue
		}

		if strings.Contains(pane, "Press Enter") || strings.Contains(pane, "Enter to confirm") {
			exec.Command(tmux.TmuxPath, "send-keys", "-t", authTmuxSession, "C-m").Run()
			continue
		}

		if strings.Contains(pane, "❯") {
			telegram.SendMessage(config, chatID, threadID, "✅ Auth successful! Claude is ready.")
			tmux.KillSession(authTmuxSession)
			authInProgress.Unlock()
			return
		}
	}

	out, _ := exec.Command(tmux.TmuxPath, "capture-pane", "-t", authTmuxSession, "-p").Output()
	pane := string(out)
	if strings.Contains(pane, "Login successful") || strings.Contains(pane, "❯") {
		telegram.SendMessage(config, chatID, threadID, "✅ Auth successful!")
	} else {
		telegram.SendMessage(config, chatID, threadID, "⚠️ Auth may have failed. Check VPS manually.")
	}

	tmux.KillSession(authTmuxSession)
	authInProgress.Unlock()
}
