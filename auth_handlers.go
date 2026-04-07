package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Auth handlers for OAuth flow and help text.

const authTmuxSession = "claude-auth"

func handleAuth(config *Config, chatID, threadID int64) {
	if !authInProgress.TryLock() {
		sendMessage(config, chatID, threadID, "⚠️ Auth already in progress")
		return
	}

	sendMessage(config, chatID, threadID, "🔐 Starting Claude auth...")

	killTmuxSession(authTmuxSession)
	time.Sleep(500 * time.Millisecond)

	home, _ := os.UserHomeDir()
	if err := exec.Command(tmuxPath, "new-session", "-d", "-s", authTmuxSession, "-c", home).Run(); err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create tmux session: %v", err))
		authInProgress.Unlock()
		return
	}

	// Build environment string for tmux from provider config
	envCmd := ""
	provider := getActiveProvider(config)
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
	cmdStr := envCmd + claudePath + " --dangerously-skip-permissions"
	exec.Command(tmuxPath, "send-keys", "-t", authTmuxSession, cmdStr, "C-m").Run()

	var oauthURL string
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		out, err := exec.Command(tmuxPath, "capture-pane", "-t", authTmuxSession, "-p", "-S", "-30").Output()
		if err != nil {
			continue
		}
		pane := string(out)

		if strings.Contains(pane, "Dark mode") || strings.Contains(pane, "❯") || strings.Contains(pane, "Welcome back") {
			sendMessage(config, chatID, threadID, "✅ Claude is already authenticated!")
			killTmuxSession(authTmuxSession)
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
		sendMessage(config, chatID, threadID, "❌ Could not find OAuth URL. Try again.")
		killTmuxSession(authTmuxSession)
		authInProgress.Unlock()
		return
	}

	authWaitingCode = true
	sendMessage(config, chatID, threadID, fmt.Sprintf("🔗 Open this URL and authorize:\n\n%s\n\nThen paste the code here.", oauthURL))
}

func handleAuthCode(config *Config, chatID, threadID int64, code string) {
	authWaitingCode = false
	code = strings.TrimSpace(code)

	sendMessage(config, chatID, threadID, "🔄 Sending code to Claude...")

	exec.Command(tmuxPath, "send-keys", "-t", authTmuxSession, "-l", code).Run()
	time.Sleep(200 * time.Millisecond)
	exec.Command(tmuxPath, "send-keys", "-t", authTmuxSession, "C-m").Run()

	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		out, _ := exec.Command(tmuxPath, "capture-pane", "-t", authTmuxSession, "-p").Output()
		pane := string(out)

		if strings.Contains(pane, "Yes, I accept") {
			exec.Command(tmuxPath, "send-keys", "-t", authTmuxSession, "Down").Run()
			time.Sleep(200 * time.Millisecond)
			exec.Command(tmuxPath, "send-keys", "-t", authTmuxSession, "C-m").Run()
			continue
		}

		if strings.Contains(pane, "Press Enter") || strings.Contains(pane, "Enter to confirm") {
			exec.Command(tmuxPath, "send-keys", "-t", authTmuxSession, "C-m").Run()
			continue
		}

		if strings.Contains(pane, "❯") {
			sendMessage(config, chatID, threadID, "✅ Auth successful! Claude is ready.")
			killTmuxSession(authTmuxSession)
			authInProgress.Unlock()
			return
		}
	}

	out, _ := exec.Command(tmuxPath, "capture-pane", "-t", authTmuxSession, "-p").Output()
	pane := string(out)
	if strings.Contains(pane, "Login successful") || strings.Contains(pane, "❯") {
		sendMessage(config, chatID, threadID, "✅ Auth successful!")
	} else {
		sendMessage(config, chatID, threadID, "⚠️ Auth may have failed. Check VPS manually.")
	}

	killTmuxSession(authTmuxSession)
	authInProgress.Unlock()
}

func printHelp() {
	fmt.Printf(`ccc - Claude Code Companion v%s

	Your companion for Claude Code - control sessions remotely via Telegram and tmux.

	USAGE:
	    ccc                     Start/attach tmux session in current directory
	    ccc -c                  Continue previous session
	    ccc <message>           Send notification (if away mode is on)

	COMMANDS:
	    setup <token>           Complete setup (bot, hook, service - all in one!)
	    doctor                  Check all dependencies and configuration
	    config                  Show/set configuration values
	    config projects-dir <path>  Set base directory for projects
	    config oauth-token <token>  Set OAuth token
	    setgroup                Configure Telegram group for topics (if skipped during setup)
	    listen                  Start the Telegram bot listener manually
	    install                 Install Claude hook manually
	    install-hooks           Install hooks in current project directory
	    uninstall               Uninstall CCC (deprecated - hooks now per-project)
	    cleanup-hooks           Remove old ccc hooks from global config
	    send <file>             Send file to current session's Telegram topic
	    relay [port]            Start relay server for large files (default: 8080)
	    run                     Run Claude directly (used by tmux sessions)

	TELEGRAM COMMANDS:
	    /new <name>             Create new session (tap to select provider)
	    /new <name>@provider    Create session with specific provider
	    /new ~/path/name        Create session with custom path
	    /new                    Restart session in current topic
	    /team <name>            Create team session (3-pane: planner|executor|reviewer)
	    /team <name>@provider   Create team session with specific provider
	    /worktree <base> <name> Create worktree session from existing session
	    /continue               Restart session keeping conversation history
	    /providers              List available AI providers
	    /provider [name]        Show or change provider for current session
	    /c <cmd>                Execute shell command
	    /update                 Update ccc binary from GitHub
	    /restart                Restart ccc service

	OTP (permission approval):
	    When OTP is enabled (via 'ccc setup'), Claude's permission requests
	    are forwarded to Telegram. Reply with your OTP code to approve.

	FLAGS:
	    -h, --help              Show this help
	    -v, --version           Show version

	For more info: https://github.com/tuannvm/ccc
	`, version)
}
