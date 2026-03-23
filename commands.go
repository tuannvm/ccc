package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/tuannvm/ccc/session"
)

// listenLog writes timestamped log entries to ccc.log AND stdout.
// This ensures logs are always persisted regardless of how the process is started.
var listenLogFile *os.File

func listenLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] [pid:%d] %s\n", time.Now().Format("2006-01-02 15:04:05"), os.Getpid(), msg)
	fmt.Print(line)
	if listenLogFile != nil {
		listenLogFile.WriteString(line)
	}
}

func initListenLog() {
	logPath := filepath.Join(cacheDir(), "ccc.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		listenLogFile = f
	}
}

var authInProgress sync.Mutex
var authWaitingCode bool
var otpAttempts = make(map[string]int) // session -> failed attempts

// isAuthorizedCallback checks if a callback query is authorized based on multi-user mode
func isAuthorizedCallback(config *Config, cb *CallbackQuery) bool {
	if cb == nil || cb.Message == nil {
		return false
	}

	if config.MultiUserMode {
		// Multi-user mode: anyone in the configured group can interact
		if config.GroupID == 0 {
			return false // Group not configured
		}
		return cb.Message.Chat.ID == config.GroupID
	}
	// Single-user mode (default): only the authorized user can interact
	return cb.From.ID == config.ChatID
}

// isAuthorizedMessage checks if a message is authorized based on multi-user mode
func isAuthorizedMessage(config *Config, msg TelegramMessage) bool {
	if config.MultiUserMode {
		// Multi-user mode: anyone in the configured group can interact
		if config.GroupID == 0 {
			return false // Group not configured
		}
		return msg.Chat.ID == config.GroupID
	}
	// Single-user mode (default): only the authorized user can interact
	return msg.From.ID == config.ChatID
}

// getSystemStats returns machine stats (works on Linux and macOS)
func getSystemStats() string {
	var sb strings.Builder
	hostname, _ := os.Hostname()
	sb.WriteString(fmt.Sprintf("🖥 %s\n\n", hostname))

	// Uptime
	if out, err := exec.Command("uptime").Output(); err == nil {
		sb.WriteString(fmt.Sprintf("⏱ %s\n", strings.TrimSpace(string(out))))
	}

	// CPU info
	if out, err := exec.Command("uname", "-m").Output(); err == nil {
		arch := strings.TrimSpace(string(out))
		// Count cores: nproc on Linux, sysctl on macOS
		var cores string
		if c, err := exec.Command("nproc").Output(); err == nil {
			cores = strings.TrimSpace(string(c))
		} else if c, err := exec.Command("sysctl", "-n", "hw.ncpu").Output(); err == nil {
			cores = strings.TrimSpace(string(c))
		}
		sb.WriteString(fmt.Sprintf("🧠 CPU: %s cores (%s)\n", cores, arch))
	}

	// Memory: Linux uses free, macOS uses vm_stat + sysctl
	if out, err := exec.Command("free", "-h").Output(); err == nil {
		// Linux
		lines := strings.Split(string(out), "\n")
		for _, l := range lines {
			if strings.HasPrefix(l, "Mem:") {
				fields := strings.Fields(l)
				if len(fields) >= 4 {
					sb.WriteString(fmt.Sprintf("💾 RAM: %s used / %s total (available: %s)\n", fields[2], fields[1], fields[6]))
				}
				break
			}
		}
	} else {
		// macOS fallback
		total, _ := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if len(total) > 0 {
			totalBytes := strings.TrimSpace(string(total))
			// Parse and convert to GB
			if tb, err := strconv.ParseUint(totalBytes, 10, 64); err == nil {
				totalGB := float64(tb) / (1024 * 1024 * 1024)
				sb.WriteString(fmt.Sprintf("💾 RAM: %.1f GB total\n", totalGB))
			}
		}
	}

	// Disk usage
	if out, err := exec.Command("df", "-h", "/").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				sb.WriteString(fmt.Sprintf("💿 Disk /: %s used / %s (%s)\n", fields[2], fields[1], fields[4]))
			}
		}
	}
	if out, err := exec.Command("df", "-h", "/home").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				// Only show if different from /
				sb.WriteString(fmt.Sprintf("💿 Disk /home: %s used / %s (%s)\n", fields[2], fields[1], fields[4]))
			}
		}
	}

	// Tmux sessions
	if out, err := exec.Command("tmux", "list-sessions").Output(); err == nil {
		sessions := strings.TrimSpace(string(out))
		if sessions != "" {
			count := len(strings.Split(sessions, "\n"))
			sb.WriteString(fmt.Sprintf("\n📟 Tmux sessions: %d\n", count))
			sb.WriteString(sessions)
		}
	}

	return sb.String()
}

// Execute shell command
func executeCommand(cmdStr string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	shell := "bash"
	if _, err := exec.LookPath("zsh"); err == nil {
		shell = "zsh"
	}
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", cmdStr)
	cmd.Dir, _ = os.UserHomeDir()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if output == "" {
		if err != nil {
			output = fmt.Sprintf("Error: %v", err)
		} else {
			output = "(no output)"
		}
	}

	return strings.TrimSpace(output), err
}

// One-shot Claude run (for private chat)
func runClaude(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	home, _ := os.UserHomeDir()
	workDir := home

	words := strings.Fields(prompt)
	if len(words) > 0 {
		firstWord := words[0]
		potentialDir := filepath.Join(home, firstWord)
		if info, err := os.Stat(potentialDir); err == nil && info.IsDir() {
			workDir = potentialDir
			prompt = strings.TrimSpace(strings.TrimPrefix(prompt, firstWord))
			if prompt == "" {
				return "Error: no prompt provided after directory name", nil
			}
		}
	}

	if claudePath == "" {
		return "Error: claude binary not found", fmt.Errorf("claude not found")
	}
	cmd := exec.CommandContext(ctx, claudePath, "--dangerously-skip-permissions", "-p", prompt)
	cmd.Dir = workDir

	// Start with current environment
	cmd.Env = os.Environ()

	// Load config and apply provider settings
	config, err := loadConfig()
	if err == nil {
		provider := getProvider(config, "")
		cmd.Env = applyProviderEnv(cmd.Env, provider, config)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if output == "" {
		if err != nil {
			output = fmt.Sprintf("Error: %v", err)
		} else {
			output = "(no output)"
		}
	}

	return strings.TrimSpace(output), err
}

func stopListenerService() {
	home, _ := os.UserHomeDir()
	if _, err := os.Stat("/Library"); err == nil {
		// macOS - launchd
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ccc.plist")
		exec.Command("launchctl", "unload", plistPath).Run()
	} else {
		// Linux - systemd
		exec.Command("systemctl", "--user", "stop", "ccc").Run()
	}
	// Also kill any manual listener via lock file PID
	lockPath := filepath.Join(cacheDir(), "ccc.lock")
	if data, err := os.ReadFile(lockPath); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pidStr != "" {
			exec.Command("kill", pidStr).Run()
		}
	}
	time.Sleep(500 * time.Millisecond)
}

func startListenerService() {
	home, _ := os.UserHomeDir()
	if _, err := os.Stat("/Library"); err == nil {
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ccc.plist")
		exec.Command("launchctl", "load", plistPath).Run()
	} else {
		exec.Command("systemctl", "--user", "start", "ccc").Run()
	}
}

func setup(botToken string) error {
	fmt.Println("🚀 Claude Code Companion Setup")
	fmt.Println("==============================")
	fmt.Println()

	// Load existing config if present (preserve sessions, group, etc.)
	config, _ := loadConfig()
	if config == nil {
		config = &Config{Sessions: make(map[string]*SessionInfo)}
	}
	config.BotToken = botToken

	// Stop listener to avoid getUpdates conflict (409 Conflict)
	fmt.Println("Stopping listener...")
	stopListenerService()

	// Step 1: Permission mode
	fmt.Println("Step 1/6: Permission mode")
	var permMode string
	err := huh.NewSelect[string]().
		Title("How should remote sessions handle permissions?").
		Description("This controls what happens when Claude Code needs\npermission to run tools in Telegram-controlled sessions.").
		Options(
			huh.NewOption[string](
				"Auto-approve\n"+
					"  All permissions granted automatically. Claude works without\n"+
					"  interruptions. Best for trusted environments where you control\n"+
					"  physical access to your machine.",
				"auto"),
			huh.NewOption[string](
				"OTP (secure)\n"+
					"  Each permission requires a 6-digit TOTP code from your\n"+
					"  authenticator app (Google Authenticator, Authy, etc.).\n"+
					"  Local terminal sessions keep their normal interactive UI.",
				"otp"),
		).
		Value(&permMode).
		Run()
	if err != nil {
		return fmt.Errorf("selection cancelled: %w", err)
	}
	fmt.Println()

	// Step 2: Get chat ID
	fmt.Println("Step 2/6: Connecting to Telegram...")
	fmt.Println("   📱 Send any message to your bot in Telegram")
	fmt.Println("   Waiting...")

	offset := 0
	for {
		resp, err := telegramGet(botToken, fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", botToken, offset))
		if err != nil {
			return fmt.Errorf("failed to get updates: %w", err)
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()

		var updates TelegramUpdate
		if err := json.Unmarshal(body, &updates); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if !updates.OK {
			return fmt.Errorf("telegram API error - check your bot token")
		}

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			if update.Message.Chat.ID != 0 {
				config.ChatID = update.Message.Chat.ID
				if err := saveConfig(config); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
				fmt.Printf("✅ Connected! (User: @%s)\n\n", update.Message.From.Username)
				goto step2
			}
		}

		time.Sleep(time.Second)
	}

step2:
	// Step 2: Group setup (optional)
	fmt.Println("Step 3/6: Group setup (optional)")
	fmt.Println("   For session topics, create a Telegram group with Topics enabled,")
	fmt.Println("   add your bot as admin, and send a message there.")
	fmt.Println("   Or press Enter to skip...")

	// Non-blocking check for group message with timeout
	fmt.Println("   Waiting 30 seconds for group message...")

	client := &http.Client{Timeout: 35 * time.Second}
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=5", config.BotToken, offset)
		resp, err := telegramClientGet(client, config.BotToken, reqURL)
		if err != nil {
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()

		var updates TelegramUpdate
		json.Unmarshal(body, &updates)

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			chat := update.Message.Chat
			// Only accept group configuration from the owner (security: prevent hijacking)
			if chat.Type == "supergroup" && update.Message.From.ID == config.ChatID {
				config.GroupID = chat.ID
				saveConfig(config)
				fmt.Printf("✅ Group configured!\n\n")
				goto step3
			}
		}
	}
	fmt.Println("⏭️  Skipped (you can run 'ccc setgroup' later)")

step3:
	// Step 3: Install Claude hook and skill
	fmt.Println("Step 4/6: Installing Claude hook and skill...")
	if err := installHook(); err != nil {
		fmt.Printf("⚠️  Hook installation failed: %v\n", err)
		fmt.Println("   You can install it later with: ccc install")
	}
	if err := installSkill(); err != nil {
		fmt.Printf("⚠️  Skill installation failed: %v\n", err)
	} else {
		fmt.Println()
	}

	// Step 4: Install service
	fmt.Println("Step 5/6: Installing background service...")
	if err := installService(); err != nil {
		fmt.Printf("⚠️  Service installation failed: %v\n", err)
		fmt.Println("   You can start manually with: ccc listen")
	} else {
		fmt.Println()
	}

	// Step 6: Apply permission mode
	fmt.Println("Step 6/6: Configuring permission mode...")
	if permMode == "otp" {
		msg, err := setupOTP(config)
		if err != nil {
			fmt.Printf("⚠️  OTP setup failed: %v\n", err)
		} else {
			fmt.Println()
			fmt.Println(msg)
			fmt.Println()
			fmt.Println("   Save this secret! You'll need it to approve remote permission requests.")
		}
	} else {
		config.OTPSecret = ""
		if err := saveConfig(config); err != nil {
			fmt.Printf("⚠️  Failed to save config: %v\n", err)
		}
		fmt.Println("✅ Auto-approve mode — all remote permissions granted automatically")
	}

	// Done!
	fmt.Println()
	fmt.Println("==============================")
	fmt.Println("✅ Setup complete!")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ccc           Start Claude Code in current directory")
	fmt.Println("  ccc -c        Continue previous session")
	fmt.Println()
	if config.GroupID != 0 {
		fmt.Println("Telegram commands (in your group):")
		fmt.Println("  /new <name>   Create new session")
		fmt.Println("  /list         List sessions")
	} else {
		fmt.Println("To enable Telegram session topics:")
		fmt.Println("  1. Create a group with Topics enabled")
		fmt.Println("  2. Add bot as admin")
		fmt.Println("  3. Run: ccc setgroup")
	}

	// Restart listener service
	fmt.Println()
	fmt.Println("Restarting listener...")
	startListenerService()

	return nil
}

func setGroup(config *Config) error {
	fmt.Println("Send a message in the group where you want to use topics...")
	fmt.Println("(Make sure Topics are enabled in group settings)")

	offset := 0
	client := &http.Client{Timeout: 35 * time.Second}

	for {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", config.BotToken, offset)
		resp, err := telegramClientGet(client, config.BotToken, reqURL)
		if err != nil {
			return err
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()

		var updates TelegramUpdate
		if err := json.Unmarshal(body, &updates); err != nil {
			continue
		}

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			chat := update.Message.Chat
			if chat.Type == "supergroup" && update.Message.From.ID == config.ChatID {
				config.GroupID = chat.ID
				if err := saveConfig(config); err != nil {
					return err
				}
				fmt.Printf("Group set: %d\n", chat.ID)
				fmt.Println("You can now create sessions with: /new <name>")
				return nil
			}
		}
	}
}

func doctor() {
	fmt.Println("🩺 ccc doctor")
	fmt.Println("=============")
	fmt.Println()

	allGood := true

	// Check tmux
	fmt.Print("tmux.............. ")
	if tmuxPath != "" {
		fmt.Printf("✅ %s\n", tmuxPath)
	} else {
		fmt.Println("❌ not found")
		fmt.Println("   Install: brew install tmux (macOS) or apt install tmux (Linux)")
		allGood = false
	}

	// Check claude
	fmt.Print("claude............ ")
	if claudePath != "" {
		fmt.Printf("✅ %s\n", claudePath)
	} else {
		fmt.Println("❌ not found")
		fmt.Println("   Install: npm install -g @anthropic-ai/claude-code")
		allGood = false
	}

	// Check ccc is in PATH (for hooks)
	fmt.Print("ccc in PATH....... ")
	home, _ := os.UserHomeDir()
	cccPaths := []string{
		filepath.Join(home, "bin", "ccc"),
		filepath.Join(home, "go", "bin", "ccc"),
	}
	foundCccPath := ""
	for _, p := range cccPaths {
		if _, err := os.Stat(p); err == nil {
			foundCccPath = p
			break
		}
	}
	if foundCccPath != "" {
		fmt.Printf("✅ %s\n", foundCccPath)
	} else {
		fmt.Println("❌ not found")
		fmt.Println("   Run: go install . (from ccc repo) or cp ccc ~/bin/")
		allGood = false
	}

	// Check config
	fmt.Print("config............ ")
	config, err := loadConfig()
	if err != nil {
		fmt.Println("❌ not found")
		fmt.Println("   Run: ccc setup <bot_token>")
		allGood = false
	} else {
		fmt.Printf("✅ %s\n", getConfigPath())

		// Check bot token
		fmt.Print("  bot_token....... ")
		if config.BotToken != "" {
			fmt.Println("✅ configured")
		} else {
			fmt.Println("❌ missing")
			allGood = false
		}

		// Check chat ID
		fmt.Print("  chat_id......... ")
		if config.ChatID != 0 {
			fmt.Printf("✅ %d\n", config.ChatID)
		} else {
			fmt.Println("❌ missing")
			allGood = false
		}

		// Check group ID (optional)
		fmt.Print("  group_id........ ")
		if config.GroupID != 0 {
			fmt.Printf("✅ %d\n", config.GroupID)
		} else {
			fmt.Println("⚠️  not set (optional, run: ccc setgroup)")
		}
	}

	// Check Claude hooks (Stop, Notification, PreToolUse)
	fmt.Print("claude hooks...... ")
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var settings map[string]interface{}
		if json.Unmarshal(data, &settings) == nil {
			if hooks, ok := settings["hooks"].(map[string]interface{}); ok {
				var installed []string
				if stop, has := hooks["Stop"].([]interface{}); has && len(stop) > 0 {
					installed = append(installed, "Stop")
				}
				if pre, has := hooks["PreToolUse"].([]interface{}); has && len(pre) > 0 {
					installed = append(installed, "PreToolUse")
				}
				if len(installed) == 2 {
					fmt.Printf("✅ installed (%s)\n", strings.Join(installed, ", "))
				} else if len(installed) > 0 {
					fmt.Printf("⚠️  partial (%s) - run: ccc install\n", strings.Join(installed, ", "))
				} else {
					fmt.Println("❌ not installed (run: ccc install)")
				}
			} else {
				fmt.Println("❌ not installed (run: ccc install)")
			}
		} else {
			fmt.Println("⚠️  settings.json parse error")
		}
	} else {
		fmt.Println("⚠️  ~/.claude/settings.json not found")
	}

	// Check service
	fmt.Print("service........... ")
	if _, err := os.Stat("/Library"); err == nil {
		// macOS - check launchd
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ccc.plist")
		if _, err := os.Stat(plistPath); err == nil {
			// Check if loaded
			cmd := exec.Command("launchctl", "list", "com.ccc")
			if cmd.Run() == nil {
				fmt.Println("✅ running (launchd)")
			} else {
				fmt.Println("⚠️  installed but not running")
				fmt.Println("   Run: launchctl load ~/Library/LaunchAgents/com.ccc.plist")
			}
		} else {
			fmt.Println("❌ not installed")
			fmt.Println("   Run: ccc setup <token> (or manually create plist)")
			allGood = false
		}
	} else {
		// Linux - check systemd
		cmd := exec.Command("systemctl", "--user", "is-active", "ccc")
		if output, err := cmd.Output(); err == nil && strings.TrimSpace(string(output)) == "active" {
			fmt.Println("✅ running (systemd)")
		} else {
			servicePath := filepath.Join(home, ".config", "systemd", "user", "ccc.service")
			if _, err := os.Stat(servicePath); err == nil {
				fmt.Println("⚠️  installed but not running")
				fmt.Println("   Run: systemctl --user start ccc")
			} else {
				fmt.Println("❌ not installed")
				fmt.Println("   Run: ccc setup <token> (or manually create service)")
				allGood = false
			}
		}
	}

	// Check transcription support
	doctorCheckWhisper()

	// Check OAuth token
	fmt.Print("oauth token....... ")
	if config != nil && config.OAuthToken != "" {
		fmt.Println("✅ configured (in config)")
	} else if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") != "" {
		fmt.Println("✅ configured (from environment)")
	} else {
		fmt.Println("⚠️  not set (optional)")
	}

	// Check OTP (permission approval)
	fmt.Print("OTP (permissions). ")
	if config != nil && isOTPEnabled(config) {
		fmt.Println("✅ enabled")
	} else {
		fmt.Println("⚠️  disabled (run: ccc setup <token> to enable)")
	}

	fmt.Println()
	if allGood {
		fmt.Println("✅ All checks passed!")
	} else {
		fmt.Println("❌ Some issues found. Fix them and run 'ccc doctor' again.")
	}
}

// Send notification (only if away)
func send(message string) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	if !config.Away {
		fmt.Println("Away mode off, skipping notification.")
		return nil
	}

	// Try to send to session topic if we're in a session directory
	if config.GroupID != 0 {
		cwd, _ := os.Getwd()
		for name, info := range config.Sessions {
			if info == nil {
				continue
			}
			// Match against saved path, subdirectories of saved path, or suffix
			if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") || strings.HasSuffix(cwd, "/"+name) {
				return sendMessage(config, config.GroupID, info.TopicID, message)
			}
		}
	}

	// Fallback to private chat
	return sendMessage(config, config.ChatID, 0, message)
}

// Main listen loop
func listen() error {
	// Small random delay to avoid race conditions when multiple instances start
	time.Sleep(time.Duration(os.Getpid()%500) * time.Millisecond)

	// Use a lock file to ensure only one instance runs
	lockPath := filepath.Join(cacheDir(), "ccc.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	defer lockFile.Close()

	// Try to acquire exclusive lock (non-blocking)
	releaseLock := acquireFileLock(lockFile)
	defer releaseLock()

	// Write our PID to the lock file
	lockFile.Truncate(0)
	lockFile.Seek(0, 0)
	fmt.Fprintf(lockFile, "%d\n", os.Getpid())

	initListenLog()
	if listenLogFile != nil {
		defer listenLogFile.Close()
	}

	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	listenLog("Bot started (chat: %d, group: %d, sessions: %d)", config.ChatID, config.GroupID, len(config.Sessions))

	setBotCommands(config.BotToken)

	// Recover undelivered Telegram messages from ledger
	for sessName, info := range config.Sessions {
		if info == nil || info.TopicID == 0 || config.GroupID == 0 {
			continue
		}
		undelivered := findUndelivered(sessName, "telegram")
		for _, ur := range undelivered {
			if ur.Type == "assistant_text" || ur.Type == "notification" {
				sendMessage(config, config.GroupID, info.TopicID, fmt.Sprintf("*%s:*\n%s", sessName, ur.Text))
				updateDelivery(sessName, ur.ID, "telegram_delivered", true)
			}
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	offset := 0
	client := &http.Client{Timeout: 35 * time.Second}

	go func() {
		sig := <-sigChan
		listenLog("Shutting down (signal: %v)", sig)
		os.Exit(0)
	}()

	// Typing indicator goroutine: sends "typing" action for sessions with thinking flag
	go func() {
		for {
			time.Sleep(4 * time.Second)
			cfg, err := loadConfig()
			if err != nil || cfg == nil {
				continue
			}
			for sessName, info := range cfg.Sessions {
				if info == nil || info.TopicID == 0 || cfg.GroupID == 0 {
					continue
				}
				if flagInfo, err := os.Stat(thinkingFlag(sessName)); err == nil {
					// Auto-expire after 10 minutes to handle missed stop hooks
					if time.Since(flagInfo.ModTime()) > 10*time.Minute {
						clearThinking(sessName)
						continue
					}
					sendTypingAction(cfg, cfg.GroupID, info.TopicID)
				}
			}
		}
	}()

	for {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", config.BotToken, offset)
		resp, err := telegramClientGet(client, config.BotToken, reqURL)
		if err != nil {
			listenLog("Network error: %v (retrying...)", err)
			time.Sleep(5 * time.Second)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()

		var updates TelegramUpdate
		if err := json.Unmarshal(body, &updates); err != nil {
			listenLog("Parse error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if !updates.OK {
			listenLog("Telegram API error: %s", updates.Description)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates.Result {
			offset = update.UpdateID + 1

			// Handle callback queries (button presses)
			if update.CallbackQuery != nil {
				cb := update.CallbackQuery
				// Authorization check: depends on multi-user mode
				if !isAuthorizedCallback(config, cb) {
					continue
				}

				answerCallbackQuery(config, cb.ID)

				// Parse callback data
				parts := strings.Split(cb.Data, ":")
				if len(parts) == 0 {
					continue
				}

				// Handle different callback types
				switch parts[0] {
				case "new":
					// Provider selection for /new command: new:session_name:provider_name
					if len(parts) == 3 {
						sessionName := parts[1]
						providerName := parts[2]
						handleNewWithProvider(config, cb, sessionName, providerName)
					}
					continue

				case "team":
					// Provider selection for /team command: team:team_name:provider_name
					if len(parts) == 3 {
						teamName := parts[1]
						providerName := parts[2]
						handleTeamWithProvider(config, cb, teamName, providerName)
					}
					continue

				case "provider":
					// Provider selection for /provider command: provider:session_name:provider_name
					if len(parts) == 3 {
						sessionName := parts[1]
						providerName := parts[2]
						handleProviderChange(config, cb, sessionName, providerName)
					}
					continue

				default:
					// Legacy: AskUserQuestion callback - session:questionIndex:totalQuestions:optionIndex
					if len(parts) >= 3 {
						sessionName := parts[0]
						questionIndex, _ := strconv.Atoi(parts[1])
						var totalQuestions, optionIndex int
						if len(parts) == 4 {
							totalQuestions, _ = strconv.Atoi(parts[2])
							optionIndex, _ = strconv.Atoi(parts[3])
						} else {
							// Legacy format: session:questionIndex:optionIndex
							optionIndex, _ = strconv.Atoi(parts[2])
						}

						// Edit message to show selection and remove buttons
						if cb.Message != nil {
							originalText := cb.Message.Text
							newText := fmt.Sprintf("%s\n\n✓ Selected option %d", originalText, optionIndex+1)
							editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID, newText)
						}

						// Switch to the session and send arrow keys
						sessionInfo, exists := config.Sessions[sessionName]
						if exists && sessionInfo != nil {
							workDir := getSessionWorkDir(config, sessionName, sessionInfo)

							// Get worktree name if this is a worktree session
							worktreeName := ""
							if sessionInfo.IsWorktree {
								worktreeName = sessionInfo.WorktreeName
							}

							// Use stored Claude session ID to resume existing conversation
							resumeSessionID := sessionInfo.ClaudeSessionID

							// Switch to the session (preserve session context for callbacks)
							// Since currentSession == sessionName, this will skip restart
							if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
								target, _ := getCccWindowTarget(sessionName)
								// Send arrow down keys to select option, then Enter
								for i := 0; i < optionIndex; i++ {
									exec.Command(tmuxPath, "send-keys", "-t", target, "Down").Run()
									time.Sleep(50 * time.Millisecond)
								}
								exec.Command(tmuxPath, "send-keys", "-t", target, "Enter").Run()
								listenLog("[callback] Selected option %d for %s (question %d/%d)", optionIndex, sessionName, questionIndex+1, totalQuestions)

								// After the last question, send Enter to confirm "Submit answers"
								if totalQuestions > 0 && questionIndex == totalQuestions-1 {
									time.Sleep(300 * time.Millisecond)
									exec.Command(tmuxPath, "send-keys", "-t", target, "Enter").Run()
									listenLog("[callback] Auto-submitted answers for %s", sessionName)
								}
							}
						}
					}
				}

				continue
			}

			msg := update.Message

			// Authorization check: depends on multi-user mode
			if !isAuthorizedMessage(config, msg) {
				continue
			}

			chatID := msg.Chat.ID
			threadID := msg.MessageThreadID
			isGroup := msg.Chat.Type == "supergroup"

			// Handle voice messages
			if msg.Voice != nil && isGroup && threadID > 0 {
				config, _ = loadConfig()
				sessionName := getSessionByTopic(config, threadID)
				if sessionName != "" {
					sessionInfo := config.Sessions[sessionName]
					if sessionInfo != nil {
						sendMessage(config, chatID, threadID, "🎤 Transcribing...")
						// Download and transcribe
						audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("voice_%d.ogg", time.Now().UnixNano()))
						if err := downloadTelegramFile(config, msg.Voice.FileID, audioPath); err != nil {
							sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
						} else {
							transcription, err := transcribeAudio(config, audioPath)
							os.Remove(audioPath)
							if err != nil {
								sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Transcription failed: %v", err))
							} else if transcription != "" {
								listenLog("[voice] @%s: %s", msg.From.Username, transcription)
								sendMessage(config, chatID, threadID, fmt.Sprintf("📝 %s", transcription))
								voiceText := "[Audio transcription, may contain errors]: " + transcription
								voiceLedgerID := fmt.Sprintf("tg:%d:voice", msg.MessageID)
								appendMessage(&MessageRecord{
									ID: voiceLedgerID, Session: sessionName, Type: "user_prompt",
									Text: voiceText, Origin: "telegram",
									TerminalDelivered: false, TelegramDelivered: true,
								})
								// Switch to session and send
								workDir := getSessionWorkDir(config, sessionName, sessionInfo)
								worktreeName := ""
								if sessionInfo.IsWorktree {
									worktreeName = sessionInfo.WorktreeName
								}
								// Use stored Claude session ID to resume existing conversation
								resumeSessionID := sessionInfo.ClaudeSessionID
								if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
									target, _ := getCccWindowTarget(sessionName)
									if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessionName), voiceText); err == nil {
										updateDelivery(sessionName, voiceLedgerID, "terminal_delivered", true)
									}
								}
							}
						}
					}
				}
				continue
			}

			// Handle photo messages
			if len(msg.Photo) > 0 && isGroup && threadID > 0 {
				config, _ = loadConfig()
				sessionName := getSessionByTopic(config, threadID)
				if sessionName != "" {
					sessionInfo := config.Sessions[sessionName]
					if sessionInfo != nil {
						// Get largest photo (last in array)
						photo := msg.Photo[len(msg.Photo)-1]
						imgPath := filepath.Join(os.TempDir(), fmt.Sprintf("telegram_%d.jpg", time.Now().UnixNano()))
						if err := downloadTelegramFile(config, photo.FileID, imgPath); err != nil {
							sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
						} else {
							caption := msg.Caption
							if caption == "" {
								caption = "Analyze this image:"
							}
							prompt := fmt.Sprintf("%s %s", caption, imgPath)
							sendMessage(config, chatID, threadID, fmt.Sprintf("📷 Image saved, sending to Claude..."))
							photoLedgerID := fmt.Sprintf("tg:%d:photo", msg.MessageID)
							appendMessage(&MessageRecord{
								ID: photoLedgerID, Session: sessionName, Type: "user_prompt",
								Text: caption, Origin: "telegram",
								TerminalDelivered: false, TelegramDelivered: true,
							})
							// Switch to session and send
							workDir := getSessionWorkDir(config, sessionName, sessionInfo)
							worktreeName := ""
							if sessionInfo.IsWorktree {
								worktreeName = sessionInfo.WorktreeName
							}
							// Use stored Claude session ID to resume existing conversation
							resumeSessionID := sessionInfo.ClaudeSessionID
							if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
								target, _ := getCccWindowTarget(sessionName)
								if err := sendToTmuxFromTelegramWithDelay(target, tmuxSafeName(sessionName), prompt, 2*time.Second); err == nil {
									updateDelivery(sessionName, photoLedgerID, "terminal_delivered", true)
								}
							}
						}
					}
				}
				continue
			}

			// Handle document messages
			if msg.Document != nil && isGroup && threadID > 0 {
				config, _ = loadConfig()
				sessionName := getSessionByTopic(config, threadID)
				if sessionName != "" {
					sessionInfo := config.Sessions[sessionName]
					if sessionInfo != nil {
						destDir := sessionInfo.Path
						if destDir == "" {
							destDir = resolveProjectPath(config, sessionName)
						}
						destPath := filepath.Join(destDir, msg.Document.FileName)
						if err := downloadTelegramFile(config, msg.Document.FileID, destPath); err != nil {
							sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
						} else {
							caption := msg.Caption
							if caption == "" {
								caption = fmt.Sprintf("I sent you this file: %s", destPath)
							} else {
								caption = fmt.Sprintf("%s\n\nFile: %s", caption, destPath)
							}
							sendMessage(config, chatID, threadID, fmt.Sprintf("📎 File saved: %s", destPath))
							docLedgerID := fmt.Sprintf("tg:%d:doc", msg.MessageID)
							appendMessage(&MessageRecord{
								ID: docLedgerID, Session: sessionName, Type: "user_prompt",
								Text: caption, Origin: "telegram",
								TerminalDelivered: false, TelegramDelivered: true,
							})
							// Switch to session and send
							workDir := getSessionWorkDir(config, sessionName, sessionInfo)
							worktreeName := ""
							if sessionInfo.IsWorktree {
								worktreeName = sessionInfo.WorktreeName
							}
							// Use stored Claude session ID to resume existing conversation
							resumeSessionID := sessionInfo.ClaudeSessionID
							if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
								target, _ := getCccWindowTarget(sessionName)
								if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessionName), caption); err == nil {
									updateDelivery(sessionName, docLedgerID, "terminal_delivered", true)
								}
							}
						}
					}
				}
				continue
			}

			text := strings.TrimSpace(msg.Text)
			if text == "" {
				continue
			}

			// Strip bot mention from commands (e.g., /ping@botname -> /ping)
			if strings.HasPrefix(text, "/") {
				if idx := strings.Index(text, "@"); idx != -1 {
					spaceIdx := strings.Index(text, " ")
					if spaceIdx == -1 || idx < spaceIdx {
						text = text[:idx] + text[strings.Index(text+" ", " "):]
					}
				}
				text = strings.TrimSpace(text)
			}

			listenLog("[%s] @%s: %s", msg.Chat.Type, msg.From.Username, redactGitURLsInText(text))

			// Handle OTP code responses (for permission approval)
			if isOTPEnabled(config) && !strings.HasPrefix(text, "/") {
				pendingSession := findPendingOTPSession()
				if pendingSession != "" {
					code := strings.TrimSpace(text)
					if validateOTP(config.OTPSecret, code) {
						writeOTPResponse(pendingSession, true)
						delete(otpAttempts, pendingSession)
						sendMessage(config, chatID, threadID, "✅ Permission approved (valid for 5 min)")
					} else {
						otpAttempts[pendingSession]++
						remaining := 5 - otpAttempts[pendingSession]
						if remaining <= 0 {
							writeOTPResponse(pendingSession, false)
							delete(otpAttempts, pendingSession)
							sendMessage(config, chatID, threadID, "❌ Too many failed attempts - permission denied")
						} else {
							sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Invalid code — %d attempts remaining", remaining))
						}
					}
					continue
				}
			}

			// Handle commands
			if strings.HasPrefix(text, "/c ") {
				cmdStr := strings.TrimPrefix(text, "/c ")
				output, err := executeCommand(cmdStr)
				if err != nil {
					output = fmt.Sprintf("⚠️ %s\n\nExit: %v", output, err)
				}
				sendMessage(config, chatID, threadID, output)
				continue
			}

			if text == "/update" {
				updateCCC(config, chatID, threadID, offset)
				continue
			}

			if text == "/restart" {
				sendMessage(config, chatID, threadID, "🔄 Restarting ccc service...")
				// Re-exec ourselves to restart cleanly
				go func() {
					time.Sleep(500 * time.Millisecond)
					exe, err := os.Executable()
					if err != nil {
						return
					}
					exec.Command(exe, "listen").Start()
					os.Exit(0)
				}()
				continue
			}

			if text == "/stats" {
				stats := getSystemStats()
				sendMessage(config, chatID, threadID, stats)
				continue
			}

			if text == "/version" {
				sendMessage(config, chatID, threadID, fmt.Sprintf("ccc %s", version))
				continue
			}

			if text == "/auth" {
				go handleAuth(config, chatID, threadID)
				continue
			}

			// /stop command - interrupt current Claude execution
			if text == "/stop" {
				if !isGroup {
					sendMessage(config, chatID, threadID, "ℹ️ /stop only works in group topics. Switch to a session topic to use this command.")
					continue
				}
				if threadID == 0 {
					sendMessage(config, chatID, threadID, "ℹ️ /stop only works in session topics. Switch to a session topic (thread) to use this command.")
					continue
				}

				sessName := getSessionByTopic(config, threadID)
				if sessName == "" {
					sendMessage(config, chatID, threadID, "❌ No session mapped to this topic.")
					continue
				}

				// Check if ccc session exists without creating it (avoid side effects)
				if !cccSessionExists() {
					sendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
					continue
				}

				// Do read-only window lookup without calling ensureCccSession
				// to avoid race condition where session gets deleted between checks
				windowName := tmuxSafeName(sessName)
				cmd := exec.Command(tmuxPath, "list-windows", "-t", cccSessionName, "-F", "#{window_name}\t#{window_id}")
				out, err := cmd.Output()
				if err != nil {
					sendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
					continue
				}

				var target string
				scanner := bufio.NewScanner(bytes.NewReader(out))
				for scanner.Scan() {
					parts := strings.SplitN(scanner.Text(), "\t", 2)
					if len(parts) == 2 && parts[0] == windowName {
						target = cccSessionName + ":" + windowName
						break
					}
				}
				// Check for scanner errors
				if err := scanner.Err(); err != nil {
					sendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
					continue
				}
				if target == "" {
					sendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
					continue
				}

				// Send Esc (Escape key) for graceful cancellation
				if err := exec.Command(tmuxPath, "send-keys", "-t", target, "C-[").Run(); err != nil {
					sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to send interrupt: %v", err))
					continue
				}

				sendMessage(config, chatID, threadID, "⏹️ Interrupt sent")
				continue
			}

			// If auth is waiting for code, send it
			if authWaitingCode && !strings.HasPrefix(text, "/") {
				go handleAuthCode(config, chatID, threadID, text)
				continue
			}

			// /continue command - restart session preserving conversation history
			if text == "/continue" && isGroup && threadID > 0 {
				config, _ = loadConfig()
				sessName := getSessionByTopic(config, threadID)
				if sessName == "" {
					sendMessage(config, chatID, threadID, "❌ No session mapped to this topic. Use /new <name> to create one.")
					continue
				}
				// Use the stored path from config, fallback to resolveProjectPath
				sessionInfo := config.Sessions[sessName]
				workDir := getSessionWorkDir(config, sessName, sessionInfo)
				if _, err := os.Stat(workDir); os.IsNotExist(err) {
					os.MkdirAll(workDir, 0755)
				}
				// Preserve worktree name if this is a worktree session
				worktreeName := ""
				if sessionInfo.IsWorktree {
					worktreeName = sessionInfo.WorktreeName
				}
				// Use stored Claude session ID to resume existing conversation
				resumeSessionID := sessionInfo.ClaudeSessionID

				if err := switchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
					sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
				} else {
					sendMessage(config, chatID, threadID, fmt.Sprintf("🔄 Session '%s' restarted with conversation history", sessName))
				}
				continue
			}

			// /delete command - delete session and thread
			if text == "/delete" && isGroup && threadID > 0 {
				config, _ = loadConfig()
				sessName := getSessionByTopic(config, threadID)
				if sessName == "" {
					sendMessage(config, chatID, threadID, "❌ No session mapped to this topic.")
					continue
				}

				// Check if this is the currently active session and stop Claude if so
				target, err := findExistingWindow(sessName)
				if err == nil {
					// Get current window name to check if it matches the session being deleted
					cmd := exec.Command(tmuxPath, "display-message", "-t", target, "-p", "#{window_name}")
					out, err := cmd.Output()
					if err == nil {
						currentWindowName := strings.TrimSpace(string(out))
						// Window names are tmux-safe (dots replaced with underscores)
						expectedName := tmuxSafeName(sessName)
						if currentWindowName == expectedName {
							// This is the active session, send C-c to stop Claude
							exec.Command(tmuxPath, "send-keys", "-t", target, "C-c").Run()
							time.Sleep(100 * time.Millisecond)
							// Also kill the window to clean up
							exec.Command(tmuxPath, "kill-window", "-t", target).Run()
						}
					}
				}

				// Remove from config
				topicID := config.Sessions[sessName].TopicID
				delete(config.Sessions, sessName)
				saveConfig(config)
				// Delete telegram thread
				if err := deleteForumTopic(config, topicID); err != nil {
					sendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Session deleted but failed to delete thread: %v", err))
				}
				// No message needed - thread is gone
				continue
			}

			// /providers command - list available providers or change session provider
			if text == "/providers" || strings.HasPrefix(text, "/provider") {
				config, _ := loadConfig()

				// If in a topic (session), show current provider + change keyboard
				if isGroup && threadID > 0 {
					sessName := getSessionByTopic(config, threadID)
					if sessName != "" {
						sessionInfo := config.Sessions[sessName]

						// Show current provider and selection keyboard
						current := sessionInfo.ProviderName
						if current == "" {
							current = config.ActiveProvider
							if current == "" {
								current = "anthropic" // Default builtin provider
							}
						}

						// Show keyboard with all available providers
						var buttons [][]InlineKeyboardButton

						// Get all providers (builtin + configured)
						providerNames := getProviderNames(config)
						for _, name := range providerNames {
							label := name
							if current == name || (sessionInfo.ProviderName == "" && config.ActiveProvider == name) {
								label += " ✓"
							}
							callbackData := fmt.Sprintf("provider:%s:%s", sessName, name)
							buttons = append(buttons, []InlineKeyboardButton{
								{Text: label, CallbackData: callbackData},
							})
						}

						msg := fmt.Sprintf("🤖 **%s**\n\nCurrent provider: %s\n\nSelect a new provider:", sessName, current)
						sendMessageWithKeyboard(config, chatID, threadID, msg, buttons)
						continue
					}
				}

				// Not in a topic - show all available providers
				var msg []string
				msg = append(msg, "📋 Available providers:")

				// Get all providers (builtin + configured)
				providerNames := getProviderNames(config)
				for _, name := range providerNames {
					active := ""
					// Mark provider as active if it's the configured active provider
					// OR if no active provider is configured and this is "anthropic" (the default)
					if config.ActiveProvider == name || (config.ActiveProvider == "" && name == "anthropic") {
						active = " (active)"
					}
					if name == "anthropic" {
						msg = append(msg, fmt.Sprintf("  • %s%s (built-in, uses default env vars)", name, active))
					} else {
						msg = append(msg, fmt.Sprintf("  • %s%s", name, active))
					}
				}

				if len(msg) == 1 {
					msg = append(msg, "\nNo additional providers configured.\n\nConfigure providers in ~/.config/ccc/config.json.")
				}
				sendMessage(config, chatID, threadID, strings.Join(msg, "\n"))
				continue
			}

			// /resume command - manage Claude session IDs
			if strings.HasPrefix(text, "/resume") && isGroup && threadID > 0 {
				config, _ = loadConfig()
				sessName := getSessionByTopic(config, threadID)
				if sessName == "" {
					sendMessage(config, chatID, threadID, "❌ No session mapped to this topic.")
					continue
				}
				sessionInfo := config.Sessions[sessName]

				// Get work dir once for both listing and validation
				workDir := getSessionWorkDir(config, sessName, sessionInfo)
				arg := strings.TrimSpace(strings.TrimPrefix(text, "/resume"))
				if arg == "" {
					// List available Claude session IDs for this project
					home, _ := os.UserHomeDir()

					// For absolute paths, extract relative path component for transcript lookup
					// This handles filepath.Join behavior where absolute paths drop earlier components
					var pathComponent string
					if filepath.IsAbs(workDir) {
						// Use the full path with leading - replaced (ZAI-style format)
						pathComponent = strings.ReplaceAll(workDir, "/", "-")
						if strings.HasPrefix(pathComponent, "/") {
							pathComponent = "-" + pathComponent[1:]
						}
					} else {
						pathComponent = workDir
					}

					// Get transcript directory from provider config
					// Priority: session provider > active provider > default
					providerName := sessionInfo.ProviderName
					if providerName == "" {
						providerName = config.ActiveProvider
					}
					var transcriptDir string
					if providerName != "" && config.Providers != nil {
						if p := config.Providers[providerName]; p != nil && p.ConfigDir != "" {
							// Expand ~ in config_dir
							configDir := p.ConfigDir
							if strings.HasPrefix(configDir, "~/") {
								configDir = filepath.Join(home, configDir[2:])
							} else if configDir == "~" {
								configDir = home
							}
							transcriptDir = filepath.Join(configDir, "projects", pathComponent)
						}
					}
					// Fallback to default Claude Code transcripts location (for anthropic)
					if transcriptDir == "" {
						transcriptDir = filepath.Join(home, ".claude", "projects", pathComponent)
					}

					// Read all .jsonl files (these are the Claude sessions)
					var sessions []string
					entries, _ := os.ReadDir(transcriptDir)
					for _, e := range entries {
						if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
							// Remove .jsonl extension to get session ID
							sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
							sessions = append(sessions, sessionID)
						}
					}

					if len(sessions) == 0 {
						sendMessage(config, chatID, threadID, "📋 No previous Claude sessions found for this project.")
						continue
					}

					// Build list of available session IDs
					var msg []string
					msg = append(msg, "📋 Available Claude sessions for this project:")

					// Show current first
					currentID := sessionInfo.ClaudeSessionID
					if currentID != "" {
						msg = append(msg, fmt.Sprintf("  • %s (current)", currentID))
					}

					// Show others (sorted by most recent first)
					// Reverse to show newest first
					for i := len(sessions) - 1; i >= 0; i-- {
						sessionID := sessions[i]
						if sessionID != currentID {
							msg = append(msg, fmt.Sprintf("  • %s", sessionID))
						}
					}

					msg = append(msg, "", fmt.Sprintf("Usage: /resume <session_id> to switch sessions"))
					sendMessage(config, chatID, threadID, strings.Join(msg, "\n"))
					continue
				}

				// Validate session ID (check if transcript exists)
				home, _ := os.UserHomeDir()

				// For absolute paths, extract relative path component for transcript lookup
				var pathComponent string
				if filepath.IsAbs(workDir) {
					// Use the full path with leading - replaced (ZAI-style format)
					pathComponent = strings.ReplaceAll(workDir, "/", "-")
					if strings.HasPrefix(pathComponent, "/") {
						pathComponent = "-" + pathComponent[1:]
					}
				} else {
					pathComponent = workDir
				}

				// Get transcript directory from provider config
				// Priority: session provider > active provider > default
				providerName := sessionInfo.ProviderName
				if providerName == "" {
					providerName = config.ActiveProvider
				}
				var transcriptDir string
				if providerName != "" && config.Providers != nil {
					if p := config.Providers[providerName]; p != nil && p.ConfigDir != "" {
						// Expand ~ in config_dir
						configDir := p.ConfigDir
						if strings.HasPrefix(configDir, "~/") {
							configDir = filepath.Join(home, configDir[2:])
						} else if configDir == "~" {
							configDir = home
						}
						transcriptDir = filepath.Join(configDir, "projects", pathComponent)
					}
				}
				// Fallback to default Claude Code transcripts location
				if transcriptDir == "" {
					transcriptDir = filepath.Join(home, ".claude", "projects", pathComponent)
				}

				transcriptPath := filepath.Join(transcriptDir, arg+".jsonl")

				if _, err := os.Stat(transcriptPath); os.IsNotExist(err) {
					sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Session not found: %s\n\nUse /resume to list available sessions.", arg))
					continue
				}

				// Update session's Claude session ID
				oldID := sessionInfo.ClaudeSessionID
				sessionInfo.ClaudeSessionID = arg
				saveConfig(config)

				msg := fmt.Sprintf("✅ Switched to session: %s", arg)
				if oldID != "" && oldID != arg {
					shortOld := oldID
					if len(oldID) > 8 {
						shortOld = oldID[:8] + "..."
					}
					msg += fmt.Sprintf("\n\nPrevious: %s", shortOld)
				}
				msg += "\n\nRestarting session..."

				sendMessage(config, chatID, threadID, msg)

				// Restart the session
				if _, err := os.Stat(workDir); os.IsNotExist(err) {
					os.MkdirAll(workDir, 0755)
				}

				// Get worktree name if this is a worktree session
				worktreeName := ""
				if sessionInfo.IsWorktree {
					worktreeName = sessionInfo.WorktreeName
				}

				if err := switchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, arg, worktreeName, false, false); err != nil {
					sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
				} else {
					// Update the stored session ID
					sessionInfo.ClaudeSessionID = arg
					saveConfig(config)
					sendMessage(config, chatID, threadID, fmt.Sprintf("🚀 Session '%s' resumed with Claude session %s", sessName, arg))
				}
				continue
			}

			// /cleanup command - delete tmux sessions and Telegram topics (NOT folders)
			if text == "/cleanup" {
				config, _ = loadConfig()
				if len(config.Sessions) == 0 {
					sendMessage(config, chatID, threadID, "No sessions to clean up.")
					continue
				}

				// In multi-window architecture, we need to stop Claude in each project window
				// and close the windows before cleaning up

				var cleaned []string
				var errors []string

				for sessName, info := range config.Sessions {
					// Stop Claude process in the project window
					if target, err := findExistingWindow(sessName); err == nil && target != "" {
						exec.Command(tmuxPath, "send-keys", "-t", target, "C-c").Run()
						time.Sleep(100 * time.Millisecond)
						// Kill the window to clean up
						exec.Command(tmuxPath, "kill-window", "-t", target).Run()
					}

					// Delete telegram thread
					if info.TopicID > 0 && config.GroupID > 0 {
						if err := deleteForumTopic(config, info.TopicID); err != nil {
							errors = append(errors, fmt.Sprintf("%s: %v", sessName, err))
						}
					}

					cleaned = append(cleaned, sessName)
				}

				// Clear all sessions from config
				config.Sessions = make(map[string]*SessionInfo)
				saveConfig(config)

				msg := fmt.Sprintf("🧹 Cleaned %d sessions: %s", len(cleaned), strings.Join(cleaned, ", "))
				if len(errors) > 0 {
					msg += fmt.Sprintf("\n\n⚠️ Errors:\n%s", strings.Join(errors, "\n"))
				}
				sendMessage(config, chatID, threadID, msg)
				continue
			}

			// /team command - create team session
			if strings.HasPrefix(text, "/team") && isGroup {
				config, _ = loadConfig()
				arg := strings.TrimSpace(strings.TrimPrefix(text, "/team"))

				// /team <name>[@provider] - create brand new team session + topic
				if arg != "" {
					// Parse provider from argument: name@provider or name --provider provider
					teamName := arg
					providerName := ""

					// Check for @provider syntax
					if idx := strings.Index(arg, "@"); idx > 0 {
						teamName = arg[:idx]
						providerName = strings.TrimSpace(arg[idx+1:])
					} else if strings.Contains(arg, " --provider ") {
						// Check for --provider syntax
						parts := strings.SplitN(arg, " --provider ", 2)
						teamName = strings.TrimSpace(parts[0])
						providerName = strings.TrimSpace(parts[1])
					}

					// Check for duplicate team session BEFORE any provider validation or creation
					teamExists := false
					var existingTopicID int64
					for topicID, sessInfo := range config.TeamSessions {
						if sessInfo != nil {
							sessName := getSessionNameFromInfo(sessInfo)
							if sessName == teamName {
								teamExists = true
								existingTopicID = topicID
								break
							}
						}
					}

					// If duplicate exists, warn and skip creation
					if teamExists {
						sendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Team session '%s' already exists (topic: %d). Use /team without args in that topic to restart.", teamName, existingTopicID))
						continue
					}

					// Validate provider if specified using getProvider()
					if providerName != "" {
						provider := getProvider(config, providerName)
						if provider == nil {
							// List available providers
							available := getProviderNames(config)
							msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
								providerName, strings.Join(available, ", "))
							sendMessage(config, chatID, threadID, msg)
							continue
						}
					}

					// Always show keyboard if no explicit provider selected
					if providerName == "" {
						// Build provider selection keyboard using getProviderNames()
						var buttons [][]InlineKeyboardButton

						// Get all providers (builtin + configured)
						providerNames := getProviderNames(config)
						for _, name := range providerNames {
							label := name
							if config.ActiveProvider == name {
								label += " ⭐"
							}
							callbackData := fmt.Sprintf("team:%s:%s", teamName, name)
							buttons = append(buttons, []InlineKeyboardButton{
								{Text: label, CallbackData: callbackData},
							})
						}

						msg := fmt.Sprintf("🤖 Select provider for team '%s':", teamName)
						sendMessageWithKeyboard(config, chatID, threadID, msg, buttons)
						continue
					}

					// Use provider from arg or default to active provider
					if providerName == "" {
						providerName = config.ActiveProvider
					}

					// Resolve working directory using the same logic as /new
					// Supports: team_name, ~/path, /absolute/path
					workDir := resolveProjectPath(config, teamName)

					// Create directory if it doesn't exist
					if _, err := os.Stat(workDir); os.IsNotExist(err) {
						os.MkdirAll(workDir, 0755)
					}

					// Create SessionInfo for team session
					sessInfo := &SessionInfo{
						Path:         workDir,
						SessionName:  teamName,
						ProviderName: providerName,
						Type:         session.SessionKindTeam,
						LayoutName:   "team-3pane",
						Panes:        make(map[session.PaneRole]*PaneInfo),
					}

					// Initialize panes
					sessInfo.Panes[session.RolePlanner] = &PaneInfo{Role: session.RolePlanner}
					sessInfo.Panes[session.RoleExecutor] = &PaneInfo{Role: session.RoleExecutor}
					sessInfo.Panes[session.RoleReviewer] = &PaneInfo{Role: session.RoleReviewer}

					// Create the 3-pane tmux layout using TeamRuntime
					runtime := session.GetRuntime(session.SessionKindTeam)
					if runtime == nil {
						sendMessage(config, chatID, threadID, "❌ Team runtime not available. Check your installation.")
						continue
					}

					if err := runtime.EnsureLayout(sessInfo, workDir); err != nil {
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create team layout: %v", err))
						continue
					}

					// Create Telegram topic
					topicID, err := createForumTopic(config, teamName, providerName, "")
					if err != nil {
						// Cleanup: kill the tmux window we just created (sanitize name)
						sanitizedName := strings.ReplaceAll(teamName, ".", "__")
						exec.Command("tmux", "kill-window", "-t", "ccc-team:"+sanitizedName).Run()
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
						continue
					}

					// Update sessInfo with the topic ID
					sessInfo.TopicID = topicID

					// Save to config
					config.SetTeamSession(topicID, sessInfo)
					if err := saveConfig(config); err != nil {
						// Cleanup: delete topic and kill window (sanitize name)
						sanitizedName := strings.ReplaceAll(teamName, ".", "__")
						deleteForumTopic(config, topicID)
						exec.Command("tmux", "kill-window", "-t", "ccc-team:"+sanitizedName).Run()
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to save config: %v", err))
						continue
					}

					// Ensure hooks are installed
					if err := ensureHooksForSession(config, teamName, sessInfo); err != nil {
						listenLog("[/team] Failed to install hooks for %s: %v", teamName, err)
					}

					// Start Claude in each pane
					if err := runtime.StartClaude(sessInfo, workDir); err != nil {
						listenLog("[/team] Failed to start Claude for %s: %v", teamName, err)
						// Non-fatal - Claude can be started manually
					}

					msg := fmt.Sprintf("✅ Team session '%s' created!\n\n", teamName)
					msg += fmt.Sprintf("📂 Path: %s\n", workDir)
					msg += fmt.Sprintf("🤖 Provider: %s\n", providerName)
					msg += fmt.Sprintf("💬 Topic ID: %d\n\n", topicID)
					msg += "📱 Send messages:\n"
					msg += "  /planner <msg>   - Send to planner\n"
					msg += "  /executor <msg>  - Send to executor\n"
					msg += "  /reviewer <msg>  - Send to reviewer\n"
					msg += "  <msg> (no cmd)   - Send to executor (default)"
					sendMessage(config, chatID, threadID, msg)
					continue
				}

				// /team without args - restart in current topic
				if config.IsTeamSession(threadID) {
					sessInfo, exists := config.GetTeamSession(threadID)
					if !exists || sessInfo == nil {
						sendMessage(config, chatID, threadID, "❌ Team session not found. Use /team <name> to create one.")
						continue
					}

					// Get runtime and restart
					runtime := session.GetRuntime(session.SessionKindTeam)
					if runtime == nil {
						sendMessage(config, chatID, threadID, "❌ Team runtime not available.")
						continue
					}

					if err := runtime.StartClaude(sessInfo, sessInfo.Path); err != nil {
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to start Claude: %v", err))
						continue
					}

					sendMessage(config, chatID, threadID, "✅ Team session restarted!")
					continue
				}

				// /team in non-team topic
				sendMessage(config, chatID, threadID, "❌ No team session linked to this topic. Use /team <name> to create one.")
				continue
			}

			// /new command - create/restart session
			if strings.HasPrefix(text, "/new") && isGroup {
				config, _ = loadConfig()
				arg := strings.TrimSpace(strings.TrimPrefix(text, "/new"))

				// /new <name>[@provider] - create brand new session + topic
				if arg != "" {
					// Parse provider from argument: name@provider or name --provider provider
					// For git URLs, only --provider syntax is supported (since @ is part of SSH URLs)
					providerName := ""
					sessionInput := arg
					gitURLHandled := false // Track if we already handled a git URL clone

					if strings.Contains(arg, " --provider ") {
						parts := strings.SplitN(arg, " --provider ", 2)
						sessionInput = strings.TrimSpace(parts[0])
						providerName = strings.TrimSpace(parts[1])
					} else if !isGitURL(arg) {
						// @provider syntax only for non-git URLs (to avoid breaking SSH/authenticated URLs)
						if idx := strings.Index(arg, "@"); idx > 0 {
							sessionInput = arg[:idx]
							providerName = strings.TrimSpace(arg[idx+1:])
						}
					}

					// Check if sessionInput is a git URL
					gitURL := ""
					sessionName := sessionInput

					if isGitURL(sessionInput) {
						gitURL = sessionInput
						sessionName = extractRepoName(sessionInput)

						// Validate sessionName was extracted successfully
						if sessionName == "" {
							sendMessage(config, chatID, threadID, "❌ Invalid git URL: could not extract repository name")
							continue
						}

						// Validate provider BEFORE cloning to avoid unnecessary work
						if providerName != "" {
							provider := getProvider(config, providerName)
							if provider == nil {
								// List available providers
								available := getProviderNames(config)
								msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
									providerName, strings.Join(available, ", "))
								sendMessage(config, chatID, threadID, msg)
								continue
							}
						}

						// Check if session already exists before cloning
						// This prevents modifying an existing session's directory
						existing, exists := config.Sessions[sessionName]
						if exists && existing != nil && existing.TopicID != 0 {
							sendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
							continue
						}

						// Check if session already exists with a custom path - use it instead of default
						workDir := filepath.Join(getProjectsDir(config), sessionName)
						// existing, exists already checked above, just check for custom path
						if exists && existing != nil && existing.Path != "" {
							workDir = existing.Path
						}

						displayURL := redactGitURL(gitURL)
						sendMessage(config, chatID, threadID, fmt.Sprintf("📥 Cloning %s into session '%s'...", displayURL, sessionName))

						// Use context with timeout to prevent blocking indefinitely
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

						result, err := cloneRepo(ctx, gitURL, workDir)
						cancel() // Clean up context immediately

						if err != nil {
							// Check for specific error types
							errMsg := fmt.Sprintf("❌ Failed to clone repository: %v", err)
							if strings.Contains(err.Error(), "directory exists but is not a git repository") {
								errMsg = fmt.Sprintf("⚠️ Directory exists but is not a git repository: %s\n\nPlease remove or rename it and try again.", workDir)
							} else if strings.Contains(err.Error(), "different git repository") {
								errMsg = "⚠️ Directory exists as a different git repository.\n\nPlease remove it or pick a different session name."
							} else if strings.Contains(err.Error(), "no origin remote") {
								errMsg = fmt.Sprintf("⚠️ Directory is a git repository but has no origin remote: %s\n\nPlease remove or use a different session name.", workDir)
							} else if strings.Contains(err.Error(), "context deadline exceeded") {
								errMsg = fmt.Sprintf("⏱️ Cloning timed out after 5 minutes. The repository may be very large or the network may be slow.")
							}
							sendMessage(config, chatID, threadID, errMsg)
							continue
						}

						// Send appropriate message based on clone result
						if result == CloneResultCloned {
							sendMessage(config, chatID, threadID, "✅ Repository cloned")
						} else if result == CloneResultAlreadyExists {
							sendMessage(config, chatID, threadID, "✅ Repository ready (using existing clone)")
						}

						// Mark that we've handled the git URL - skip provider keyboard below
						gitURLHandled = true
					}

					// Validate provider if specified (for non-git URLs or after git clone)
					if providerName != "" {
						provider := getProvider(config, providerName)
						if provider == nil {
							// List available providers
							available := getProviderNames(config)
							msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
								providerName, strings.Join(available, ", "))
							sendMessage(config, chatID, threadID, msg)
							continue
						}
					}

					// Always show keyboard if no explicit provider selected
					// Skip if we already handled a git URL clone (repo is already cloned)
					if providerName == "" && !gitURLHandled {
						// Check if session already exists
						existing, exists := config.Sessions[sessionName]
						if exists && existing != nil && existing.TopicID != 0 {
							sendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
							continue
						}

						// Build provider selection keyboard using getProviderNames()
						var buttons [][]InlineKeyboardButton

						// Get all providers (builtin + configured)
						providerNames := getProviderNames(config)
						for _, name := range providerNames {
							label := name
							if config.ActiveProvider == name {
								label += " ⭐"
							}
							callbackData := fmt.Sprintf("new:%s:%s", sessionName, name)
							buttons = append(buttons, []InlineKeyboardButton{
								{Text: label, CallbackData: callbackData},
							})
						}

						msg := fmt.Sprintf("🤖 Select provider for '%s':", sessionName)
						sendMessageWithKeyboard(config, chatID, threadID, msg, buttons)
						continue
					}

					// Direct creation (single provider or provider specified)
					existing, exists := config.Sessions[sessionName]
					if exists && existing != nil && existing.TopicID != 0 {
						sendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
						continue
					}
					// Use provider from arg or default to active provider
					if providerName == "" {
						providerName = config.ActiveProvider
					}
					topicID, err := createForumTopic(config, sessionName, providerName, "")
					if err != nil {
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
						continue
					}
					// Use pre-configured path if session was preset, otherwise resolve from name
					workDir := resolveProjectPath(config, sessionName)
					if exists && existing != nil && existing.Path != "" {
						workDir = existing.Path
					}
					config.Sessions[sessionName] = &SessionInfo{
						TopicID:      topicID,
						Path:         workDir,
						ProviderName: providerName,
					}
					saveConfig(config)

					// Ensure hooks are installed in the project directory
					if err := ensureHooksForSession(config, sessionName, config.Sessions[sessionName]); err != nil {
						listenLog("[/new] Failed to install hooks for %s: %v", sessionName, err)
					}

					if _, err := os.Stat(workDir); os.IsNotExist(err) {
						os.MkdirAll(workDir, 0755)
					}
					providerMsg := ""
					if providerName != "" {
						providerMsg = fmt.Sprintf("\n🤖 Provider: %s", providerName)
					}
					// Switch to the new session in the single ccc window
					if err := switchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
						sendMessage(config, config.GroupID, topicID, fmt.Sprintf("❌ Failed to start session: %v", err))
					} else {
						sendMessage(config, config.GroupID, topicID, fmt.Sprintf("🚀 Session '%s' started!%s\n\nSend messages here to interact with Claude.", sessionName, providerMsg))
					}
					continue
				}

				// Without args - restart session in current topic
				if threadID > 0 {
					sessionName := getSessionByTopic(config, threadID)
					if sessionName == "" {
						sendMessage(config, chatID, threadID, "❌ No session mapped to this topic. Use /new <name> to create one.")
						continue
					}
					sessionInfo := config.Sessions[sessionName]
					workDir := getSessionWorkDir(config, sessionName, sessionInfo)
					if _, err := os.Stat(workDir); os.IsNotExist(err) {
						os.MkdirAll(workDir, 0755)
					}
					// Preserve worktree name if this is a worktree session
					worktreeName := ""
					if sessionInfo.IsWorktree {
						worktreeName = sessionInfo.WorktreeName
					}
					// Use stored Claude session ID to resume existing conversation
					resumeSessionID := sessionInfo.ClaudeSessionID

					// Ensure hooks are installed before resuming
					if err := ensureHooksForSession(config, sessionName, sessionInfo); err != nil {
						listenLog("[/new] Failed to verify/install hooks for %s: %v", sessionName, err)
					}

					if err := switchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
					} else {
						sendMessage(config, chatID, threadID, fmt.Sprintf("🚀 Session '%s' restarted", sessionName))
					}
				} else {
					sendMessage(config, chatID, threadID, "Usage: /new <name> to create a new session")
				}
				continue
			}

			// /worktree command - create worktree session from existing session
			if strings.HasPrefix(text, "/worktree") && isGroup {
				args := strings.TrimSpace(strings.TrimPrefix(text, "/worktree"))
				parts := strings.Fields(args)

				var baseSessionName, worktreeName string
				autoGenerated := false

				// Determine base session and worktree name based on args and context
				if len(parts) == 0 && threadID > 0 {
					// No args in topic: use current session as base, let Claude generate worktree name
					baseSessionName = getSessionByTopic(config, threadID)
					if baseSessionName == "" {
						sendMessage(config, chatID, threadID, "❌ No active session. Use /new to create a session first.")
						continue
					}

					baseSession, exists := config.Sessions[baseSessionName]
					if !exists || baseSession == nil {
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Session '%s' not found.", baseSessionName))
						continue
					}

					// Check if base session path is a git repository
					basePath := baseSession.Path
					if basePath == "" {
						basePath = resolveProjectPath(config, baseSessionName)
					}
					gitCmd := exec.Command("git", "-C", basePath, "rev-parse", "--is-inside-work-tree")
					if out, err := gitCmd.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Session '%s' is not a git repository: %s\n\nCannot create worktree. Initialize git first with 'git init'.", baseSessionName, basePath))
						continue
					}

					// Get provider from base session or active provider
					providerName := baseSession.ProviderName
					if providerName == "" {
						providerName = config.ActiveProvider
					}

					// Start Claude with --worktree flag (no name = auto-generate)
					// Take snapshot of existing worktrees before invoking Claude
					existingWorktrees := getWorktreeNames(basePath)
					if existingWorktrees == nil {
						existingWorktrees = make(map[string]bool)
					}

					sendMessage(config, chatID, threadID, "🌳 Creating worktree...")
					listenLog("[/worktree] Starting Claude with --worktree for %s in %s (existing worktrees: %d)", baseSessionName, basePath, len(existingWorktrees))

					// Start Claude in tmux with --worktree (AUTO = auto-generate name)
					if err := switchSessionInWindow(baseSessionName, basePath, providerName, "", WorktreeAutoGenerate, false, false); err != nil {
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to start Claude: %v", err))
						continue
					}

					// Wait for new worktree directory to be created (poll-based, up to 30s)
					worktreeName = waitForNewWorktree(basePath, existingWorktrees, 30*time.Second)
					if worktreeName == "" {
						sendMessage(config, chatID, threadID, "❌ Failed to detect generated worktree name (timeout after 30s)\n\n💡 Tip: A worktree may have been created. Check with: git worktree list")
						continue
					}

					listenLog("[/worktree] Detected worktree name: %s", worktreeName)
					worktreeSessionName := baseSessionName + "_" + worktreeName
					autoGenerated = true

					// Create Telegram topic with color based on base project
					topicID, err := createForumTopic(config, worktreeSessionName, providerName, baseSessionName)
					if err != nil {
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
						continue
					}

					// Create session info with worktree metadata
					// Path points to the worktree directory for file operations, but hooks match via base directory
					worktreePath := filepath.Join(basePath, ".claude", "worktrees", worktreeName)
					config.Sessions[worktreeSessionName] = &SessionInfo{
						TopicID:      topicID,
						Path:         worktreePath,
						ProviderName: providerName,
						IsWorktree:   true,
						WorktreeName: worktreeName,
						BaseSession:  baseSessionName,
					}
					saveConfig(config)

					// Ensure hooks are installed
					if err := ensureHooksForSession(config, worktreeSessionName, config.Sessions[worktreeSessionName]); err != nil {
						listenLog("[/worktree] Failed to install hooks for %s: %v", worktreeSessionName, err)
					}

					// Switch to the worktree session with the proper worktree name
					if err := switchSessionInWindow(worktreeSessionName, basePath, providerName, "", worktreeName, false, false); err != nil {
						sendMessage(config, config.GroupID, topicID, fmt.Sprintf("❌ Failed to start session: %v", err))
						continue
					}

					providerMsg := ""
					if providerName != "" {
						providerMsg = fmt.Sprintf("\n🤖 Provider: %s", providerName)
					}
					worktreeMsg := fmt.Sprintf("🌳 Worktree session '%s' started!\nBase: %s\nWorktree: %s%s\n\n✨ Worktree name was auto-generated by Claude", worktreeSessionName, baseSessionName, worktreeName, providerMsg)
					worktreeMsg += "\n\nSend messages here to interact with Claude."
					sendMessage(config, config.GroupID, topicID, worktreeMsg)
					continue
				}

				if len(parts) == 1 && threadID > 0 {
					// Single arg in topic: treat as worktree name, use current session as base
					worktreeName = parts[0]
					baseSessionName = getSessionByTopic(config, threadID)
					if baseSessionName == "" {
						sendMessage(config, chatID, threadID, "❌ No active session. Use /new to create a session first, or use: /worktree <session_name> <worktree_name>")
						continue
					}
				} else if len(parts) == 1 {
					// Single arg not in topic: require both arguments
					sendMessage(config, chatID, threadID, "Usage: /worktree <session_name> <worktree_name>\n\nCreates a new worktree session from an existing session's repository.\n\nTip: In a session topic, you can use just /worktree <name>.")
					continue
				} else {
					// Two arguments: session name and worktree name
					baseSessionName = parts[0]
					worktreeName = parts[1]
				}

				// Check if base session exists
				baseSession, exists := config.Sessions[baseSessionName]
				if !exists || baseSession == nil {
					sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Base session '%s' not found. Use /new to create it first.", baseSessionName))
					continue
				}

				// Check if base session path is a git repository
				basePath := baseSession.Path
				if basePath == "" {
					basePath = resolveProjectPath(config, baseSessionName)
				}
				// Use git to check if path is inside a work tree (handles worktrees and nested dirs)
				gitCmd := exec.Command("git", "-C", basePath, "rev-parse", "--is-inside-work-tree")
				if out, err := gitCmd.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
					sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Base session path is not a git repository: %s", basePath))
					continue
				}

				// Create unique session name for worktree: base_worktree
				worktreeSessionName := baseSessionName + "_" + worktreeName

				// Check if worktree session already exists
				if _, exists := config.Sessions[worktreeSessionName]; exists {
					sendMessage(config, chatID, threadID, fmt.Sprintf("⚠️ Worktree session '%s' already exists. Use /new in that topic to restart.", worktreeSessionName))
					continue
				}

				// Get provider from base session or active provider
				providerName := baseSession.ProviderName
				if providerName == "" {
					providerName = config.ActiveProvider
				}

				// Create Telegram topic with color based on base project for visual grouping
				topicID, err := createForumTopic(config, worktreeSessionName, providerName, baseSessionName)
				if err != nil {
					sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
					continue
				}

				// Create session info with worktree metadata
				// Path points to the worktree directory for file operations, but hooks match via base directory
				worktreePath := filepath.Join(basePath, ".claude", "worktrees", worktreeName)
				config.Sessions[worktreeSessionName] = &SessionInfo{
					TopicID:      topicID,
					Path:         worktreePath,
					ProviderName: providerName,
					IsWorktree:   true,
					WorktreeName: worktreeName,
					BaseSession:  baseSessionName,
				}
				saveConfig(config)

				// Ensure hooks are installed in the base project directory
				if err := ensureHooksForSession(config, worktreeSessionName, config.Sessions[worktreeSessionName]); err != nil {
					listenLog("[/worktree] Failed to install hooks for %s: %v", worktreeSessionName, err)
				}

				// Switch to the worktree session in the single ccc window
				// Use basePath for cd (worktree dir is created by Claude Code with --worktree flag)
				if err := switchSessionInWindow(worktreeSessionName, basePath, providerName, "", worktreeName, false, false); err != nil {
					sendMessage(config, config.GroupID, topicID, fmt.Sprintf("❌ Failed to start session: %v", err))
					continue
				}

				providerMsg := ""
				if providerName != "" {
					providerMsg = fmt.Sprintf("\n🤖 Provider: %s", providerName)
				}
				worktreeMsg := fmt.Sprintf("🌳 Worktree session '%s' started!\nBase: %s\nWorktree: %s%s", worktreeSessionName, baseSessionName, worktreeName, providerMsg)
				if autoGenerated {
					worktreeMsg += "\n\n✨ Worktree name was auto-generated"
				}
				worktreeMsg += "\n\nSend messages here to interact with Claude."
				sendMessage(config, config.GroupID, topicID, worktreeMsg)
				continue
			}

			// Check if message is in a topic (interactive session)
			if isGroup && threadID > 0 {
				// Reload config to get latest sessions
				config, _ = loadConfig()

				// Check if this is a team session (NEW: multi-pane support)
				if config.IsTeamSession(threadID) {
					// Built-in commands that should bypass team routing and be handled by CCC directly
					if isBuiltinCommand(text) {
						// Let the command fall through to standard handling below
						listenLog("[team-session] Bypassing team routing for built-in command: %s", text)
					} else {
						// Handle team session routing (goes to specific pane)
						if handled := handleTeamSessionMessage(config, text, threadID, chatID, threadID); handled {
							continue
						}
						// If handleTeamSessionMessage returns false, fall through to standard handling
					}
				}

				sessName := getSessionByTopic(config, threadID)
				if sessName != "" {
					var target string
					var err error

					// Only switch if the requested session is different from current
					// Compare using tmux-safe names since window names are sanitized
					currentSession := getCurrentSessionName()
					needsSwitch := currentSession != tmuxSafeName(sessName)

					if needsSwitch {
						// Switch to the correct session in the single ccc window
						sessionInfo := config.Sessions[sessName]
						workDir := getSessionWorkDir(config, sessName, sessionInfo)
						if _, err := os.Stat(workDir); os.IsNotExist(err) {
							os.MkdirAll(workDir, 0755)
						}

						// Preserve worktree name if this is a worktree session
						worktreeName := ""
						if sessionInfo.IsWorktree {
							worktreeName = sessionInfo.WorktreeName
						}

						// Use stored Claude session ID to resume existing conversation
						resumeSessionID := sessionInfo.ClaudeSessionID

						// Ensure hooks are installed before resuming session
						if err := ensureHooksForSession(config, sessName, sessionInfo); err != nil {
							listenLog("[sendToTmux] Failed to verify/install hooks for %s: %v", sessName, err)
						}

						// Switch to the session in the single ccc window (always restart when switching sessions)
						if err := switchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
							sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
							continue
						}

						// Get the target for the project window
						target, err = getCccWindowTarget(sessName)
						if err != nil {
							sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to get ccc window: %v", err))
							continue
						}

						// Wait for Claude to be ready after switching
						time.Sleep(1 * time.Second)

						// Replay any undelivered terminal messages for this session
						undelivered := findUndelivered(sessName, "terminal")
						for _, ur := range undelivered {
							if ur.Type == "user_prompt" && ur.Origin == "telegram" {
								if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessName), ur.Text); err == nil {
									updateDelivery(sessName, ur.ID, "terminal_delivered", true)
								}
								time.Sleep(500 * time.Millisecond)
							}
						}

						listenLog("sendToTmux: target=%s session=%s (switched from %s)", target, sessName, currentSession)
					} else {
						// Already in the correct session, just get the target
						target, err = getCccWindowTarget(sessName)
						if err != nil {
							sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to get ccc window: %v", err))
							continue
						}
						listenLog("sendToTmux: target=%s session=%s (already active)", target, sessName)
					}

					// Record in ledger before sending
					ledgerID := fmt.Sprintf("tg:%d", update.UpdateID)
					appendMessage(&MessageRecord{
						ID:                ledgerID,
						Session:           sessName,
						Type:              "user_prompt",
						Text:              text,
						Origin:            "telegram",
						TerminalDelivered: false,
						TelegramDelivered: true,
					})

					if err := sendToTmuxFromTelegram(target, tmuxSafeName(sessName), text); err != nil {
						listenLog("sendToTmux FAILED: target=%s err=%v", target, err)
						sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to send: %v", err))
					} else {
						updateDelivery(sessName, ledgerID, "terminal_delivered", true)
					}
				} else {
					sendMessage(config, chatID, threadID, "⚠️ No session linked to this topic. Use /new <name> to create one.")
				}
				continue
			}

			// Private chat: run one-shot Claude
			if !isGroup {
				sendMessage(config, chatID, threadID, "🤖 Running Claude...")

				prompt := text
				if msg.ReplyToMessage != nil && msg.ReplyToMessage.Text != "" {
					origText := msg.ReplyToMessage.Text
					origWords := strings.Fields(origText)
					if len(origWords) > 0 {
						home, _ := os.UserHomeDir()
						potentialDir := filepath.Join(home, origWords[0])
						if info, err := os.Stat(potentialDir); err == nil && info.IsDir() {
							prompt = origWords[0] + " " + text
						}
					}
					prompt = fmt.Sprintf("Original message:\n%s\n\nReply:\n%s", origText, prompt)
				}

				go func(p string, cid int64) {
					defer func() {
						if r := recover(); r != nil {
							sendMessage(config, cid, 0, fmt.Sprintf("💥 Panic: %v", r))
						}
					}()
					output, err := runClaude(p)
					if err != nil {
						if strings.Contains(err.Error(), "context deadline exceeded") {
							output = fmt.Sprintf("⏱️ Timeout (10min)\n\n%s", output)
						} else {
							output = fmt.Sprintf("⚠️ %s\n\nExit: %v", output, err)
						}
					}
					sendMessage(config, cid, 0, output)
				}(prompt, chatID)
			}
		}
	}
}

// handleNewWithProvider creates a session after provider selection via inline keyboard
func handleNewWithProvider(config *Config, cb *CallbackQuery, sessionName, providerName string) {
	// Check if session already exists
	existing, exists := config.Sessions[sessionName]
	if exists && existing != nil && existing.TopicID != 0 {
		if cb.Message != nil {
			editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("⚠️ Session '%s' already exists.\n\nUse /new without args in that topic to restart.", sessionName))
		}
		return
	}

	// Create topic
	topicID, err := createForumTopic(config, sessionName, providerName, "")
	if err != nil {
		if cb.Message != nil {
			editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create topic: %v", err))
		}
		return
	}

	// Resolve work directory
	workDir := resolveProjectPath(config, sessionName)
	if exists && existing != nil && existing.Path != "" {
		workDir = existing.Path
	}

	// Create session
	config.Sessions[sessionName] = &SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	saveConfig(config)

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	// Switch to the new session in the single ccc window
	resultMsg := fmt.Sprintf("🚀 Session '%s' started!\n🤖 Provider: %s\n\nSend messages here to interact with Claude.", sessionName, providerName)
	if err := switchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		resultMsg = fmt.Sprintf("❌ Failed to start session: %v", err)
	}

	if cb.Message != nil {
		editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

// handleProviderChange changes provider for an existing session via inline keyboard
func handleProviderChange(config *Config, cb *CallbackQuery, sessionName, providerName string) {
	// Check if session exists
	session, exists := config.Sessions[sessionName]
	if !exists || session == nil {
		if cb.Message != nil {
			editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Session '%s' not found.", sessionName))
		}
		return
	}

	// Validate provider using getProvider()
	// getProvider returns nil for unknown providers
	provider := getProvider(config, providerName)
	if provider == nil {
		if cb.Message != nil {
			editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Provider '%s' not found. Available: %v", providerName, getProviderNames(config)))
		}
		return
	}

	// Update session provider
	session.ProviderName = providerName
	saveConfig(config)

	resultMsg := fmt.Sprintf("✅ Provider changed to %s for session '%s'\n\nRestart with /new to apply the new provider.", provider.Name(), sessionName)
	if cb.Message != nil {
		editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

// handleTeamWithProvider creates a team session after provider selection via inline keyboard
func handleTeamWithProvider(config *Config, cb *CallbackQuery, teamName, providerName string) {
	// Check if team session already exists
	for topicID, sessInfo := range config.TeamSessions {
		if sessInfo != nil {
			sessName := getSessionNameFromInfo(sessInfo)
			if sessName == teamName {
				if cb.Message != nil {
					editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
						fmt.Sprintf("⚠️ Team session '%s' already exists (topic: %d).\n\nUse /team without args in that topic to restart.", teamName, topicID))
				}
				return
			}
		}
	}

	// Resolve working directory using the same logic as /new
	// Supports: team_name, ~/path, /absolute/path
	workDir := resolveProjectPath(config, teamName)

	// Create directory if it doesn't exist
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	// Create SessionInfo for team session
	sessInfo := &SessionInfo{
		Path:         workDir,
		SessionName:  teamName,
		ProviderName: providerName,
		Type:         session.SessionKindTeam,
		LayoutName:   "team-3pane",
		Panes:        make(map[session.PaneRole]*PaneInfo),
	}

	// Initialize panes
	sessInfo.Panes[session.RolePlanner] = &PaneInfo{Role: session.RolePlanner}
	sessInfo.Panes[session.RoleExecutor] = &PaneInfo{Role: session.RoleExecutor}
	sessInfo.Panes[session.RoleReviewer] = &PaneInfo{Role: session.RoleReviewer}

	// Create the 3-pane tmux layout using TeamRuntime
	runtime := session.GetRuntime(session.SessionKindTeam)
	if runtime == nil {
		if cb.Message != nil {
			editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				"❌ Team runtime not available. Check your installation.")
		}
		return
	}

	if err := runtime.EnsureLayout(sessInfo, workDir); err != nil {
		if cb.Message != nil {
			editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create team layout: %v", err))
		}
		return
	}

	// Create Telegram topic
	topicID, err := createForumTopic(config, teamName, providerName, "")
	if err != nil {
		// Cleanup: kill the tmux window we just created (sanitize name)
		sanitizedName := strings.ReplaceAll(teamName, ".", "__")
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+sanitizedName).Run()
		if cb.Message != nil {
			editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create topic: %v", err))
		}
		return
	}

	// Update sessInfo with the topic ID
	sessInfo.TopicID = topicID

	// Save to config
	config.SetTeamSession(topicID, sessInfo)
	if err := saveConfig(config); err != nil {
		// Cleanup: delete topic and kill window (sanitize name)
		sanitizedName := strings.ReplaceAll(teamName, ".", "__")
		deleteForumTopic(config, topicID)
		exec.Command("tmux", "kill-window", "-t", "ccc-team:"+sanitizedName).Run()
		if cb.Message != nil {
			editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to save config: %v", err))
		}
		return
	}

	// Ensure hooks are installed
	if err := ensureHooksForSession(config, teamName, sessInfo); err != nil {
		listenLog("[/team] Failed to install hooks for %s: %v", teamName, err)
	}

	// Start Claude in each pane
	if err := runtime.StartClaude(sessInfo, workDir); err != nil {
		listenLog("[/team] Failed to start Claude for %s: %v", teamName, err)
		// Non-fatal - Claude can be started manually
	}

	resultMsg := fmt.Sprintf("✅ Team session '%s' created!\n\n", teamName)
	resultMsg += fmt.Sprintf("📂 Path: %s\n", workDir)
	resultMsg += fmt.Sprintf("🤖 Provider: %s\n", providerName)
	resultMsg += fmt.Sprintf("💬 Topic ID: %d\n\n", topicID)
	resultMsg += "📱 Send messages:\n"
	resultMsg += "  /planner <msg>   - Send to planner\n"
	resultMsg += "  /executor <msg>  - Send to executor\n"
	resultMsg += "  /reviewer <msg>  - Send to reviewer\n"
	resultMsg += "  <msg> (no cmd)   - Send to executor (default)"

	if cb.Message != nil {
		editMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
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
		// Determine auth token (auto-load from env if empty)
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
			// Fallback: if Sonnet is not configured but Opus is, use Opus as default
			envCmd += fmt.Sprintf("export ANTHROPIC_MODEL='%s'; ", provider.OpusModel)
		}
		if provider.HaikuModel != "" {
			envCmd += fmt.Sprintf("export ANTHROPIC_DEFAULT_HAIKU_MODEL='%s'; ", provider.HaikuModel)
		}
		if provider.SubagentModel != "" {
			envCmd += fmt.Sprintf("export CLAUDE_CODE_SUBAGENT_MODEL='%s'; ", provider.SubagentModel)
		}

		// Config dir with ~ expansion
		if provider.ConfigDir != "" {
			configDir := provider.ConfigDir
			if strings.HasPrefix(configDir, "~/") {
				configDir = home + configDir[1:]
			} else if configDir == "~" {
				configDir = home
			}
			envCmd += fmt.Sprintf("export CLAUDE_CONFIG_DIR='%s'; ", configDir)
		}

		// API timeout from config
		if provider.ApiTimeout > 0 {
			envCmd += fmt.Sprintf("export API_TIMEOUT_MS=%d; ", provider.ApiTimeout)
		}
	}

	time.Sleep(500 * time.Millisecond)
	// Send env vars + claude command to tmux
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
