package listen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/lookup"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// EnsureHooks wraps hooks.EnsureHooksForSession with lookup.GetSessionWorkDir
func EnsureHooks(cfg *configpkg.Config, sessionName string, info *configpkg.SessionInfo) error {
	return hooks.EnsureHooksForSession(&hooks.EnsureHooksForSessionConfig{
		Config:            cfg,
		SessionName:       sessionName,
		SessionInfo:       info,
		GetSessionWorkDir: lookup.GetSessionWorkDir,
	})
}

// HandleContinueCommand handles the /continue command - restart session preserving conversation history
func HandleContinueCommand(cfg *configpkg.Config, chatID, threadID int64) {
	cfg, _ = configpkg.Load()
	sessName := lookup.GetSessionByTopic(cfg, threadID)
	if sessName == "" {
		telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic. Use /new <name> to create one.")
		return
	}
	sessionInfo := cfg.Sessions[sessName]
	workDir := lookup.GetSessionWorkDir(cfg, sessName, sessionInfo)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}
	worktreeName, resumeSessionID, _ := lookup.GetSessionContext(sessionInfo)

	providerName := effectiveProviderName(cfg, sessionInfo)
	if err := tmux.SwitchSessionInWindow(sessName, workDir, providerName, resumeSessionID, worktreeName, true, false); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
	} else {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("%s restarted\n%s", sessName, providerSummary(cfg, sessionInfo)))
	}
}

// HandleDeleteCommand handles the /delete command - delete session and thread
func HandleDeleteCommand(cfg *configpkg.Config, chatID, threadID int64) {
	cfg, _ = configpkg.Load()
	sessName := lookup.GetSessionByTopic(cfg, threadID)
	if sessName == "" {
		telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic.")
		return
	}

	// Check if this is the currently active session and stop Claude if so
	target, err := tmux.FindExistingWindow(sessName)
	if err == nil {
		cmd := exec.Command(tmux.TmuxPath, "display-message", "-t", target, "-p", "#{window_name}")
		out, err := cmd.Output()
		if err == nil {
			currentWindowName := strings.TrimSpace(string(out))
			expectedName := tmux.SafeName(sessName)
			if currentWindowName == expectedName {
				exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "C-c").Run()
				time.Sleep(100 * time.Millisecond)
				exec.Command(tmux.TmuxPath, "kill-window", "-t", target).Run()
			}
		}
	}

	topicID := cfg.Sessions[sessName].TopicID
	delete(cfg.Sessions, sessName)
	if err := configpkg.Save(cfg); err != nil {
		cfg.Sessions[sessName] = &configpkg.SessionInfo{TopicID: topicID} // restore on failure
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Failed to save config: %v", err))
		return
	}
	if err := telegram.DeleteForumTopic(cfg, topicID); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session deleted but failed to delete thread: %v", err))
	}
}

// HandleProvidersCommand handles the /providers and /provider commands
func HandleProvidersCommand(cfg *configpkg.Config, chatID, threadID int64, text string, isGroup bool) {
	cfg, _ = configpkg.Load()
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/provider"))

	if isGroup && threadID > 0 {
		sessName := lookup.GetSessionByTopic(cfg, threadID)
		if sessName != "" {
			sessionInfo := cfg.Sessions[sessName]
			if arg != "" && text != "/providers" {
				provider := providerpkg.GetProvider(cfg, arg)
				if provider == nil {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("unknown provider: %s\navailable: %s", arg, strings.Join(providerpkg.GetProviderNames(cfg), ", ")))
					return
				}
				sessionInfo.ProviderName = provider.Name()
				if err := configpkg.Save(cfg); err != nil {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("failed to save provider: %v", err))
					return
				}
				telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("provider changed\nsession: %s\nprovider: %s\nsource: session\n\nRestart with /new to apply.", sessName, provider.Name()))
				return
			}
			current := effectiveProviderName(cfg, sessionInfo)

			var buttons [][]telegram.InlineKeyboardButton
			providerNames := providerpkg.GetProviderNames(cfg)
			for _, name := range providerNames {
				label := name
				if current == name || (sessionInfo.ProviderName == "" && cfg.ActiveProvider == name) {
					label += " ✓"
				}
				callbackData := fmt.Sprintf("provider:%s:%s", sessName, name)
				buttons = append(buttons, []telegram.InlineKeyboardButton{
					{Text: label, CallbackData: callbackData},
				})
			}

			msg := fmt.Sprintf("session: %s\n%s\n\nSelect provider:", sessName, providerSummary(cfg, sessionInfo))
			telegram.SendMessageWithKeyboard(cfg, chatID, threadID, msg, buttons)
			return
		}
	}

	// Not in a topic - show all available providers
	var msg []string
	msg = append(msg, "providers")
	providerNames := providerpkg.GetProviderNames(cfg)
	for _, name := range providerNames {
		active := ""
		if cfg.ActiveProvider == name || (cfg.ActiveProvider == "" && name == "anthropic") {
			active = " (active)"
		}
		if name == "anthropic" {
			msg = append(msg, fmt.Sprintf("  • %s%s (built-in, uses default env vars)", name, active))
		} else {
			msg = append(msg, fmt.Sprintf("  • %s%s", name, active))
		}
	}
	if len(msg) == 1 {
		msg = append(msg, "\nNo additional providers configured.\n\nConfigure providers in ~/.config/ccc/ (config.providers.json, config.core.json, config.sessions.json; legacy config.json is also supported).")
	}
	telegram.SendMessage(cfg, chatID, threadID, strings.Join(msg, "\n"))
}

// HandleCleanupCommand handles the /cleanup command - delete tmux sessions and Telegram topics
func HandleCleanupCommand(cfg *configpkg.Config, chatID, threadID int64) {
	cfg, _ = configpkg.Load()
	if len(cfg.Sessions) == 0 {
		telegram.SendMessage(cfg, chatID, threadID, "No sessions to clean up.")
		return
	}

	var cleaned []string
	var errors []string

	for sessName, info := range cfg.Sessions {
		if target, err := tmux.FindExistingWindow(sessName); err == nil && target != "" {
			exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "C-c").Run()
			time.Sleep(100 * time.Millisecond)
			exec.Command(tmux.TmuxPath, "kill-window", "-t", target).Run()
		}

		if info.TopicID > 0 && cfg.GroupID > 0 {
			if err := telegram.DeleteForumTopic(cfg, info.TopicID); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", sessName, err))
			}
		}

		cleaned = append(cleaned, sessName)
	}

	cfg.Sessions = make(map[string]*configpkg.SessionInfo)
	configpkg.Save(cfg)

	msg := fmt.Sprintf("🧹 Cleaned %d sessions: %s", len(cleaned), strings.Join(cleaned, ", "))
	if len(errors) > 0 {
		msg += fmt.Sprintf("\n\n⚠️ Errors:\n%s", strings.Join(errors, "\n"))
	}
	telegram.SendMessage(cfg, chatID, threadID, msg)
}

// HandleStopCommand handles the /stop command - interrupt current Claude execution
func HandleStopCommand(cfg *configpkg.Config, chatID, threadID int64, isGroup bool) {
	if !isGroup {
		telegram.SendMessage(cfg, chatID, threadID, "ℹ️ /stop only works in group topics. Switch to a session topic to use this command.")
		return
	}
	if threadID == 0 {
		telegram.SendMessage(cfg, chatID, threadID, "ℹ️ /stop only works in session topics. Switch to a session topic (thread) to use this command.")
		return
	}

	sessName := lookup.GetSessionByTopic(cfg, threadID)
	if sessName == "" {
		telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic.")
		return
	}

	if !tmux.SessionExists() {
		telegram.SendMessage(cfg, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	windowName := tmux.SafeName(sessName)
	cmd := exec.Command(tmux.TmuxPath, "list-windows", "-t", tmux.SessionName, "-F", "#{window_name}\t#{window_id}")
	out, err := cmd.Output()
	if err != nil {
		telegram.SendMessage(cfg, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	var target string
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[0] == windowName {
			target = tmux.SessionName + ":" + windowName
			break
		}
	}
	if target == "" {
		telegram.SendMessage(cfg, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	if err := exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "C-[").Run(); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to send interrupt: %v", err))
		return
	}

	telegram.SendMessage(cfg, chatID, threadID, "⏹️ Interrupt sent")
}

// HandlePrivateChat handles one-shot Claude execution in private chat
func HandlePrivateChat(cfg *configpkg.Config, msg telegram.TelegramMessage, chatID, threadID int64, runClaudeOneShot func(string) (string, error)) {
	telegram.SendMessage(cfg, chatID, threadID, "🤖 Running Claude...")

	prompt := strings.TrimSpace(msg.Text)
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.Text != "" {
		origText := msg.ReplyToMessage.Text
		origWords := strings.Fields(origText)
		if len(origWords) > 0 {
			home, _ := os.UserHomeDir()
			potentialDir := home + "/" + origWords[0]
			if info, err := os.Stat(potentialDir); err == nil && info.IsDir() {
				prompt = origWords[0] + " " + msg.Text
			}
		}
		prompt = fmt.Sprintf("Original message:\n%s\n\nReply:\n%s", origText, prompt)
	}

	go func(p string, cid int64) {
		defer func() {
			if r := recover(); r != nil {
				telegram.SendMessage(cfg, cid, 0, fmt.Sprintf("💥 Panic: %v", r))
			}
		}()
		output, err := runClaudeOneShot(p)
		if err != nil {
			if strings.Contains(err.Error(), "context deadline exceeded") {
				output = fmt.Sprintf("⏱️ Timeout (10min)\n\n%s", output)
			} else {
				output = fmt.Sprintf("⚠️ %s\n\nExit: %v", output, err)
			}
		}
		telegram.SendMessage(cfg, cid, 0, output)
	}(prompt, chatID)
}
