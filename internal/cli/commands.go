package cli

import (
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
	"github.com/kidandcat/ccc/internal/config"
	"github.com/kidandcat/ccc/internal/hooks"
	"github.com/kidandcat/ccc/internal/otp"
	"github.com/kidandcat/ccc/internal/session"
	"github.com/kidandcat/ccc/internal/telegram"
	"github.com/kidandcat/ccc/internal/tmux"
	"github.com/kidandcat/ccc/internal/whisper"
)

const Version = "1.7.0"

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
	logPath := filepath.Join(config.CacheDir(), "ccc.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		listenLogFile = f
	}
}

var authInProgress sync.Mutex
var authWaitingCode bool
var otpAttempts = make(map[string]int) // session -> failed attempts

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
func RunClaude(prompt string) (string, error) {
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

	if tmux.ClaudePath == "" {
		return "Error: claude binary not found", fmt.Errorf("claude not found")
	}
	cmd := exec.CommandContext(ctx, tmux.ClaudePath, "--dangerously-skip-permissions", "-p", prompt)
	cmd.Dir = workDir

	// Start with current environment
	cmd.Env = os.Environ()

	// Load config and apply provider settings
	cfg, err := config.LoadConfig()
	if err == nil {
		provider := config.GetProvider(cfg, "")
		cmd.Env = tmux.ApplyProviderEnv(cmd.Env, provider, cfg)
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
	lockPath := filepath.Join(config.CacheDir(), "ccc.lock")
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

func Setup(botToken string) error {
	fmt.Println("🚀 Claude Code Companion Setup")
	fmt.Println("==============================")
	fmt.Println()

	// Load existing config if present (preserve sessions, group, etc.)
	cfg, _ := config.LoadConfig()
	if cfg == nil {
		cfg = &config.Config{Sessions: make(map[string]*config.SessionInfo)}
	}
	cfg.BotToken = botToken

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
		resp, err := telegram.TelegramGet(botToken, fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", botToken, offset))
		if err != nil {
			return fmt.Errorf("failed to get updates: %w", err)
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.TelegramMaxResponseSize))
		resp.Body.Close()

		var updates telegram.TelegramUpdate
		if err := json.Unmarshal(body, &updates); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if !updates.OK {
			return fmt.Errorf("telegram API error - check your bot token")
		}

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			if update.Message.Chat.ID != 0 {
				cfg.ChatID = update.Message.Chat.ID
				if err := config.SaveConfig(cfg); err != nil {
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
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=5", cfg.BotToken, offset)
		resp, err := telegram.TelegramClientGet(client, cfg.BotToken, reqURL)
		if err != nil {
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.TelegramMaxResponseSize))
		resp.Body.Close()

		var updates telegram.TelegramUpdate
		json.Unmarshal(body, &updates)

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			chat := update.Message.Chat
			if chat.Type == "supergroup" {
				cfg.GroupID = chat.ID
				config.SaveConfig(cfg)
				fmt.Printf("✅ Group configured!\n\n")
				goto step3
			}
		}
	}
	fmt.Println("⏭️  Skipped (you can run 'ccc setgroup' later)")

step3:
	// Step 3: Install Claude hook and skill
	fmt.Println("Step 4/6: Installing Claude hook and skill...")
	if err := hooks.InstallHook(); err != nil {
		fmt.Printf("⚠️  Hook installation failed: %v\n", err)
		fmt.Println("   You can install it later with: ccc install")
	}
	if err := hooks.InstallSkill(); err != nil {
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
		msg, err := otp.SetupOTP(cfg)
		if err != nil {
			fmt.Printf("⚠️  OTP setup failed: %v\n", err)
		} else {
			fmt.Println()
			fmt.Println(msg)
			fmt.Println()
			fmt.Println("   Save this secret! You'll need it to approve remote permission requests.")
		}
	} else {
		cfg.OTPSecret = ""
		if err := config.SaveConfig(cfg); err != nil {
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
	if cfg.GroupID != 0 {
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

// HandleConfigCommand handles the config subcommand
func HandleConfigCommand(args []string) error {
	if len(args) < 3 {
		cfg, _ := config.LoadConfig()
		fmt.Println("Current configuration:")
		if cfg != nil {
			fmt.Printf("  Projects Dir: %s\n", cfg.ProjectsDir)
			fmt.Printf("  OAuth Token: %s\n", maskToken(cfg.OAuthToken))
		}
		fmt.Println("\nUsage:")
		fmt.Println("  ccc config projects-dir <path>  Set base directory for projects")
		fmt.Println("  ccc config oauth-token <token>  Set OAuth token")
		return nil
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	switch args[2] {
	case "projects-dir":
		if len(args) < 4 {
			return fmt.Errorf("usage: ccc config projects-dir <path>")
		}
		cfg.ProjectsDir = args[3]
		if err := config.SaveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("✅ Projects directory set to: %s\n", cfg.ProjectsDir)

	case "oauth-token":
		if len(args) < 4 {
			return fmt.Errorf("usage: ccc config oauth-token <token>")
		}
		cfg.OAuthToken = args[3]
		if err := config.SaveConfig(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("✅ OAuth token updated")

	default:
		return fmt.Errorf("unknown config command: %s", args[2])
	}

	return nil
}

func maskToken(token string) string {
	if token == "" {
		return "(not set)"
	}
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "***" + token[len(token)-4:]
}

func SetGroup(cfg *config.Config) error {
	fmt.Println("Send a message in the group where you want to use topics...")
	fmt.Println("(Make sure Topics are enabled in group settings)")

	offset := 0
	client := &http.Client{Timeout: 35 * time.Second}

	for {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", cfg.BotToken, offset)
		resp, err := telegram.TelegramClientGet(client, cfg.BotToken, reqURL)
		if err != nil {
			return err
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.TelegramMaxResponseSize))
		resp.Body.Close()

		var updates telegram.TelegramUpdate
		if err := json.Unmarshal(body, &updates); err != nil {
			continue
		}

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			chat := update.Message.Chat
			if chat.Type == "supergroup" && update.Message.From.ID == cfg.ChatID {
				cfg.GroupID = chat.ID
				if err := config.SaveConfig(cfg); err != nil {
					return err
				}
				fmt.Printf("Group set: %d\n", chat.ID)
				fmt.Println("You can now create sessions with: /new <name>")
				return nil
			}
		}
	}
}

func Doctor() {
	fmt.Println("🩺 ccc doctor")
	fmt.Println("=============")
	fmt.Println()

	allGood := true

	// Check tmux
	fmt.Print("tmux.............. ")
	if tmux.TmuxPath != "" {
		fmt.Printf("✅ %s\n", tmux.TmuxPath)
	} else {
		fmt.Println("❌ not found")
		fmt.Println("   Install: brew install tmux (macOS) or apt install tmux (Linux)")
		allGood = false
	}

	// Check claude
	fmt.Print("claude............ ")
	if tmux.ClaudePath != "" {
		fmt.Printf("✅ %s\n", tmux.ClaudePath)
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
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Println("❌ not found")
		fmt.Println("   Run: ccc setup <bot_token>")
		allGood = false
	} else {
		fmt.Printf("✅ %s\n", config.ConfigPath())

		// Check bot token
		fmt.Print("  bot_token....... ")
		if cfg.BotToken != "" {
			fmt.Println("✅ configured")
		} else {
			fmt.Println("❌ missing")
			allGood = false
		}

		// Check chat ID
		fmt.Print("  chat_id......... ")
		if cfg.ChatID != 0 {
			fmt.Printf("✅ %d\n", cfg.ChatID)
		} else {
			fmt.Println("❌ missing")
			allGood = false
		}

		// Check group ID (optional)
		fmt.Print("  group_id........ ")
		if cfg.GroupID != 0 {
			fmt.Printf("✅ %d\n", cfg.GroupID)
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
	whisper.DoctorCheckWhisper()

	// Check OAuth token
	fmt.Print("oauth token....... ")
	if cfg != nil && cfg.OAuthToken != "" {
		fmt.Println("✅ configured (in config)")
	} else if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") != "" {
		fmt.Println("✅ configured (from environment)")
	} else {
		fmt.Println("⚠️  not set (optional)")
	}

	// Check OTP (permission approval)
	fmt.Print("OTP (permissions). ")
	if cfg != nil && otp.IsOTPEnabled(cfg) {
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
func Send(message string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	if !cfg.Away {
		fmt.Println("Away mode off, skipping notification.")
		return nil
	}

	// Try to send to session topic if we're in a session directory
	if cfg.GroupID != 0 {
		cwd, _ := os.Getwd()
		for name, info := range cfg.Sessions {
			if info == nil {
				continue
			}
			// Match against saved path, subdirectories of saved path, or suffix
			if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") || strings.HasSuffix(cwd, "/"+name) {
				return telegram.SendMessage(cfg, cfg.GroupID, info.TopicID, message)
			}
		}
	}

	// Fallback to private chat
	return telegram.SendMessage(cfg, cfg.ChatID, 0, message)
}

// SendWithFallback sends a notification to the current session or falls back to private chat
func SendWithFallback(message string) error {
	return Send(message)
}

// HandleSendFile sends a file to the current session's Telegram topic
func HandleSendFile(filePath string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Try to send to session topic if we're in a session directory
	if cfg.GroupID != 0 {
		cwd, _ := os.Getwd()
		for name, info := range cfg.Sessions {
			if info == nil {
				continue
			}
			// Match against saved path, subdirectories of saved path, or suffix
			if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") || strings.HasSuffix(cwd, "/"+name) {
				return telegram.SendFile(cfg, cfg.GroupID, info.TopicID, filePath, "")
			}
		}
	}

	// Fallback to private chat
	return telegram.SendFile(cfg, cfg.ChatID, 0, filePath, "")
}

// Main listen loop
func Listen() error {
	// Small random delay to avoid race conditions when multiple instances start
	time.Sleep(time.Duration(os.Getpid()%500) * time.Millisecond)

	// Use a lock file to ensure only one instance runs
	lockPath := filepath.Join(config.CacheDir(), "ccc.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	defer lockFile.Close()

	// Try to acquire exclusive lock (non-blocking)
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fmt.Println("Another ccc listen instance is already running, exiting quietly")
		os.Exit(0) // Exit with 0 so launchd doesn't restart
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	// Write our PID to the lock file
	lockFile.Truncate(0)
	lockFile.Seek(0, 0)
	fmt.Fprintf(lockFile, "%d\n", os.Getpid())

	initListenLog()
	if listenLogFile != nil {
		defer listenLogFile.Close()
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	listenLog("Bot started (chat: %d, group: %d, sessions: %d)", cfg.ChatID, cfg.GroupID, len(cfg.Sessions))

	telegram.SetBotCommands(cfg.BotToken)

	// Recover undelivered Telegram messages from ledger
	for sessName, info := range cfg.Sessions {
		if info == nil || info.TopicID == 0 || cfg.GroupID == 0 {
			continue
		}
		undelivered := session.FindUndelivered(sessName, "telegram")
		for _, ur := range undelivered {
			if ur.Type == "assistant_text" || ur.Type == "notification" {
				telegram.SendMessage(cfg, cfg.GroupID, info.TopicID, fmt.Sprintf("*%s:*\n%s", sessName, ur.Text))
				session.UpdateDelivery(sessName, ur.ID, "telegram_delivered", true)
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
			cfg, err := config.LoadConfig()
			if err != nil || cfg == nil {
				continue
			}
			for sessName, info := range cfg.Sessions {
				if info == nil || info.TopicID == 0 || cfg.GroupID == 0 {
					continue
				}
				if flagInfo, err := os.Stat(hooks.ThinkingFlag(sessName)); err == nil {
					// Auto-expire after 10 minutes to handle missed stop hooks
					if time.Since(flagInfo.ModTime()) > 10*time.Minute {
						hooks.ClearThinking(sessName)
						continue
					}
					telegram.SendTypingAction(cfg, cfg.GroupID, info.TopicID)
				}
			}
		}
	}()

	for {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", cfg.BotToken, offset)
		resp, err := telegram.TelegramClientGet(client, cfg.BotToken, reqURL)
		if err != nil {
			listenLog("Network error: %v (retrying...)", err)
			time.Sleep(5 * time.Second)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.TelegramMaxResponseSize))
		resp.Body.Close()

		var updates telegram.TelegramUpdate
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
				// Only accept from authorized user
				if cb.From.ID != cfg.ChatID {
					continue
				}

				telegram.AnswerCallbackQuery(cfg, cb.ID)

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
						handleNewWithProvider(cfg, cb, sessionName, providerName)
					}
					continue

				case "provider":
					// Provider selection for /provider command: provider:session_name:provider_name
					if len(parts) == 3 {
						sessionName := parts[1]
						providerName := parts[2]
						handleProviderChange(cfg, cb, sessionName, providerName)
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
							telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID, newText)
						}

						// Switch to the session and send arrow keys
						sessionInfo, exists := cfg.Sessions[sessionName]
						if exists && sessionInfo != nil {
							workDir := session.GetSessionWorkDir(cfg, sessionName, sessionInfo)

							// Get worktree name if this is a worktree session
							worktreeName := ""
							if sessionInfo.IsWorktree {
								worktreeName = sessionInfo.WorktreeName
							}

							// Use stored Claude session ID to resume existing conversation
							resumeSessionID := sessionInfo.ClaudeSessionID

							// Switch to the session (preserve session context for callbacks)
							// Since currentSession == sessionName, this will skip restart
							if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
								target, _ := tmux.GetCccWindowTarget(sessionName)
								// Send arrow down keys to select option, then Enter
								for i := 0; i < optionIndex; i++ {
									exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Down").Run()
									time.Sleep(50 * time.Millisecond)
								}
								exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Enter").Run()
								listenLog("[callback] Selected option %d for %s (question %d/%d)", optionIndex, sessionName, questionIndex+1, totalQuestions)

								// After the last question, send Enter to confirm "Submit answers"
								if totalQuestions > 0 && questionIndex == totalQuestions-1 {
									time.Sleep(300 * time.Millisecond)
									exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Enter").Run()
									listenLog("[callback] Auto-submitted answers for %s", sessionName)
								}
							}
						}
					}
				}

				continue
			}

			msg := update.Message

			// Only accept from authorized user
			if msg.From.ID != cfg.ChatID {
				continue
			}

			chatID := msg.Chat.ID
			threadID := msg.MessageThreadID
			isGroup := msg.Chat.Type == "supergroup"

			// Handle voice messages
			if msg.Voice != nil && isGroup && threadID > 0 {
				sessionCfg, _ := config.LoadConfig()
				sessionName := session.GetSessionByTopic(sessionCfg, threadID)
				if sessionName != "" {
					sessionInfo := sessionCfg.Sessions[sessionName]
					if sessionInfo != nil {
						telegram.SendMessage(sessionCfg, chatID, threadID, "🎤 Transcribing...")
						// Download and transcribe
						audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("voice_%d.ogg", time.Now().UnixNano()))
						if err := telegram.DownloadTelegramFile(sessionCfg, msg.Voice.FileID, audioPath); err != nil {
							telegram.SendMessage(sessionCfg, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
						} else {
							transcription, err := whisper.TranscribeAudio(sessionCfg, audioPath)
							os.Remove(audioPath)
							if err != nil {
								telegram.SendMessage(sessionCfg, chatID, threadID, fmt.Sprintf("❌ Transcription failed: %v", err))
							} else if transcription != "" {
								listenLog("[voice] @%s: %s", msg.From.Username, transcription)
								telegram.SendMessage(sessionCfg, chatID, threadID, fmt.Sprintf("📝 %s", transcription))
								voiceText := "[Audio transcription, may contain errors]: " + transcription
								voiceLedgerID := fmt.Sprintf("tg:%d:voice", msg.MessageID)
								session.AppendMessage(&session.MessageRecord{
									ID: voiceLedgerID, Session: sessionName, Type: "user_prompt",
									Text: voiceText, Origin: "telegram",
									TerminalDelivered: false, TelegramDelivered: true,
								})
								// Switch to session and send
								workDir := session.GetSessionWorkDir(sessionCfg, sessionName, sessionInfo)
								worktreeName := ""
								if sessionInfo.IsWorktree {
									worktreeName = sessionInfo.WorktreeName
								}
								// Use stored Claude session ID to resume existing conversation
								resumeSessionID := sessionInfo.ClaudeSessionID
								if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
									target, _ := tmux.GetCccWindowTarget(sessionName)
									if err := tmux.SendToTmuxFromTelegram(target, tmux.TmuxSafeName(sessionName), voiceText); err == nil {
										session.UpdateDelivery(sessionName, voiceLedgerID, "terminal_delivered", true)
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
				sessionCfg, _ := config.LoadConfig()
				sessionName := session.GetSessionByTopic(sessionCfg, threadID)
				if sessionName != "" {
					sessionInfo := sessionCfg.Sessions[sessionName]
					if sessionInfo != nil {
						// Get largest photo (last in array)
						photo := msg.Photo[len(msg.Photo)-1]
						imgPath := filepath.Join(os.TempDir(), fmt.Sprintf("telegram_%d.jpg", time.Now().UnixNano()))
						if err := telegram.DownloadTelegramFile(sessionCfg, photo.FileID, imgPath); err != nil {
							telegram.SendMessage(sessionCfg, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
						} else {
							caption := msg.Caption
							if caption == "" {
								caption = "Analyze this image:"
							}
							prompt := fmt.Sprintf("%s %s", caption, imgPath)
							telegram.SendMessage(sessionCfg, chatID, threadID, fmt.Sprintf("📷 Image saved, sending to Claude..."))
							photoLedgerID := fmt.Sprintf("tg:%d:photo", msg.MessageID)
							session.AppendMessage(&session.MessageRecord{
								ID: photoLedgerID, Session: sessionName, Type: "user_prompt",
								Text: caption, Origin: "telegram",
								TerminalDelivered: false, TelegramDelivered: true,
							})
							// Switch to session and send
							workDir := session.GetSessionWorkDir(sessionCfg, sessionName, sessionInfo)
							worktreeName := ""
							if sessionInfo.IsWorktree {
								worktreeName = sessionInfo.WorktreeName
							}
							// Use stored Claude session ID to resume existing conversation
							resumeSessionID := sessionInfo.ClaudeSessionID
							if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
								target, _ := tmux.GetCccWindowTarget(sessionName)
								if err := tmux.SendToTmuxFromTelegramWithDelay(target, tmux.TmuxSafeName(sessionName), prompt, 2*time.Second); err == nil {
									session.UpdateDelivery(sessionName, photoLedgerID, "terminal_delivered", true)
								}
							}
						}
					}
				}
				continue
			}

			// Handle document messages
			if msg.Document != nil && isGroup && threadID > 0 {
				sessionCfg, _ := config.LoadConfig()
				sessionName := session.GetSessionByTopic(sessionCfg, threadID)
				if sessionName != "" {
					sessionInfo := sessionCfg.Sessions[sessionName]
					if sessionInfo != nil {
						destDir := sessionInfo.Path
						if destDir == "" {
							destDir = config.ResolveProjectPath(sessionCfg, sessionName)
						}
						destPath := filepath.Join(destDir, msg.Document.FileName)
						if err := telegram.DownloadTelegramFile(sessionCfg, msg.Document.FileID, destPath); err != nil {
							telegram.SendMessage(sessionCfg, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
						} else {
							caption := msg.Caption
							if caption == "" {
								caption = fmt.Sprintf("I sent you this file: %s", destPath)
							} else {
								caption = fmt.Sprintf("%s\n\nFile: %s", caption, destPath)
							}
							telegram.SendMessage(sessionCfg, chatID, threadID, fmt.Sprintf("📎 File saved: %s", destPath))
							docLedgerID := fmt.Sprintf("tg:%d:doc", msg.MessageID)
							session.AppendMessage(&session.MessageRecord{
								ID: docLedgerID, Session: sessionName, Type: "user_prompt",
								Text: caption, Origin: "telegram",
								TerminalDelivered: false, TelegramDelivered: true,
							})
							// Switch to session and send
							workDir := session.GetSessionWorkDir(sessionCfg, sessionName, sessionInfo)
							worktreeName := ""
							if sessionInfo.IsWorktree {
								worktreeName = sessionInfo.WorktreeName
							}
							// Use stored Claude session ID to resume existing conversation
							resumeSessionID := sessionInfo.ClaudeSessionID
							if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err == nil {
								target, _ := tmux.GetCccWindowTarget(sessionName)
								if err := tmux.SendToTmuxFromTelegram(target, tmux.TmuxSafeName(sessionName), caption); err == nil {
									session.UpdateDelivery(sessionName, docLedgerID, "terminal_delivered", true)
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

			listenLog("[%s] @%s: %s", msg.Chat.Type, msg.From.Username, text)

			// Handle OTP code responses (for permission approval)
			if otp.IsOTPEnabled(cfg) && !strings.HasPrefix(text, "/") {
				pendingSession := otp.FindPendingOTPSession()
				if pendingSession != "" {
					code := strings.TrimSpace(text)
					if otp.ValidateOTP(cfg.OTPSecret, code) {
						otp.WriteOTPResponse(pendingSession, true)
						delete(otpAttempts, pendingSession)
						telegram.SendMessage(cfg, chatID, threadID, "✅ Permission approved (valid for 5 min)")
					} else {
						otpAttempts[pendingSession]++
						remaining := 5 - otpAttempts[pendingSession]
						if remaining <= 0 {
							otp.WriteOTPResponse(pendingSession, false)
							delete(otpAttempts, pendingSession)
							telegram.SendMessage(cfg, chatID, threadID, "❌ Too many failed attempts - permission denied")
						} else {
							telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Invalid code — %d attempts remaining", remaining))
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
				telegram.SendMessage(cfg, chatID, threadID, output)
				continue
			}

			if text == "/update" {
				telegram.UpdateCCC(cfg, chatID, threadID, offset)
				continue
			}

			if text == "/restart" {
				telegram.SendMessage(cfg, chatID, threadID, "🔄 Restarting ccc service...")
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
				telegram.SendMessage(cfg, chatID, threadID, stats)
				continue
			}

			if text == "/Version" {
				telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("ccc %s", Version))
				continue
			}

			if text == "/auth" {
				go handleAuth(cfg, chatID, threadID)
				continue
			}

			// If auth is waiting for code, send it
			if authWaitingCode && !strings.HasPrefix(text, "/") {
				go handleAuthCode(cfg, chatID, threadID, text)
				continue
			}

			// /continue command - restart session preserving conversation history
			if text == "/continue" && isGroup && threadID > 0 {
				sessionCfg, _ := config.LoadConfig()
				sessName := session.GetSessionByTopic(sessionCfg, threadID)
				if sessName == "" {
					telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic. Use /new <name> to create one.")
					continue
				}
				// Use the stored path from config, fallback to resolveProjectPath
				sessionInfo := sessionCfg.Sessions[sessName]
				workDir := session.GetSessionWorkDir(sessionCfg, sessName, sessionInfo)
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

				if err := tmux.SwitchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
				} else {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("🔄 Session '%s' restarted with conversation history", sessName))
				}
				continue
			}

			// /delete command - delete session and thread
			if text == "/delete" && isGroup && threadID > 0 {
				sessionCfg, _ := config.LoadConfig()
				sessName := session.GetSessionByTopic(sessionCfg, threadID)
				if sessName == "" {
					telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic.")
					continue
				}

				// Check if this is the currently active session and stop Claude if so
				target, err := tmux.GetCccWindowTarget(sessName)
				if err == nil {
					// Get current window name to check if it matches the session being deleted
					cmd := exec.Command(tmux.TmuxPath, "display-message", "-t", target, "-p", "#{window_name}")
					out, err := cmd.Output()
					if err == nil {
						currentWindowName := strings.TrimSpace(string(out))
						// Window names are tmux-safe (dots replaced with underscores)
						expectedName := tmux.TmuxSafeName(sessName)
						if currentWindowName == expectedName {
							// This is the active session, send C-c to stop Claude
							exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "C-c").Run()
							time.Sleep(100 * time.Millisecond)
							// Also kill the window to clean up
							exec.Command(tmux.TmuxPath, "kill-window", "-t", target).Run()
						}
					}
				}

				// Remove from config
				topicID := sessionCfg.Sessions[sessName].TopicID
				delete(sessionCfg.Sessions, sessName)
				config.SaveConfig(sessionCfg)
				// Delete telegram thread
				if err := telegram.DeleteForumTopic(sessionCfg, topicID); err != nil {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session deleted but failed to delete thread: %v", err))
				}
				// No message needed - thread is gone
				continue
			}

			// /providers command - list available providers or change session provider
			if text == "/providers" || strings.HasPrefix(text, "/provider") {
				cfg, _ := config.LoadConfig()

				// If in a topic (session), show current provider + change keyboard
				if isGroup && threadID > 0 {
					sessName := session.GetSessionByTopic(cfg, threadID)
					if sessName != "" {
						sessionInfo := cfg.Sessions[sessName]

						// Show current provider and selection keyboard
						current := sessionInfo.ProviderName
						if current == "" {
							current = cfg.ActiveProvider
							if current == "" {
								current = "anthropic" // Default builtin provider
							}
						}

						// Show keyboard with all available providers
						var buttons [][]telegram.InlineKeyboardButton

						// Get all providers (builtin + configured)
						providerNames := config.GetProviderNames(cfg)
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

						msg := fmt.Sprintf("🤖 **%s**\n\nCurrent provider: %s\n\nSelect a new provider:", sessName, current)
						telegram.SendMessageWithKeyboard(cfg, chatID, threadID, msg, buttons)
						continue
					}
				}

				// Not in a topic - show all available providers
				var msg []string
				msg = append(msg, "📋 Available providers:")

				// Get all providers (builtin + configured)
				providerNames := config.GetProviderNames(cfg)
				for _, name := range providerNames {
					active := ""
					// Mark provider as active if it's the configured active provider
					// OR if no active provider is configured and this is "anthropic" (the default)
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
					msg = append(msg, "\nNo additional providers configured.\n\nConfigure providers in ~/.config/ccc/config.json.")
				}
				telegram.SendMessage(cfg, chatID, threadID, strings.Join(msg, "\n"))
				continue
			}

			// /resume command - manage Claude session IDs
			if strings.HasPrefix(text, "/resume") && isGroup && threadID > 0 {
				cfg, _ := config.LoadConfig()
				sessName := session.GetSessionByTopic(cfg, threadID)
				if sessName == "" {
					telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic.")
					continue
				}
				sessionInfo := cfg.Sessions[sessName]

				// Get work dir once for both listing and validation
				workDir := session.GetSessionWorkDir(cfg, sessName, sessionInfo)
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
						providerName = cfg.ActiveProvider
					}
					var transcriptDir string
					if providerName != "" && cfg.Providers != nil {
						if p := cfg.Providers[providerName]; p != nil && p.ConfigDir != "" {
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
						telegram.SendMessage(cfg, chatID, threadID, "📋 No previous Claude sessions found for this project.")
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
					telegram.SendMessage(cfg, chatID, threadID, strings.Join(msg, "\n"))
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
					providerName = cfg.ActiveProvider
				}
				var transcriptDir string
				if providerName != "" && cfg.Providers != nil {
					if p := cfg.Providers[providerName]; p != nil && p.ConfigDir != "" {
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
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Session not found: %s\n\nUse /resume to list available sessions.", arg))
					continue
				}

				// Update session's Claude session ID
				oldID := sessionInfo.ClaudeSessionID
				sessionInfo.ClaudeSessionID = arg
				config.SaveConfig(cfg)

				msg := fmt.Sprintf("✅ Switched to session: %s", arg)
				if oldID != "" && oldID != arg {
					shortOld := oldID
					if len(oldID) > 8 {
						shortOld = oldID[:8] + "..."
					}
					msg += fmt.Sprintf("\n\nPrevious: %s", shortOld)
				}
				msg += "\n\nRestarting session..."

				telegram.SendMessage(cfg, chatID, threadID, msg)

				// Restart the session
				if _, err := os.Stat(workDir); os.IsNotExist(err) {
					os.MkdirAll(workDir, 0755)
				}

				// Get worktree name if this is a worktree session
				worktreeName := ""
				if sessionInfo.IsWorktree {
					worktreeName = sessionInfo.WorktreeName
				}

				if err := tmux.SwitchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, arg, worktreeName, false, false); err != nil {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
				} else {
					// Update the stored session ID
					sessionInfo.ClaudeSessionID = arg
					config.SaveConfig(cfg)
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("🚀 Session '%s' resumed with Claude session %s", sessName, arg))
				}
				continue
			}

			// /cleanup command - delete tmux sessions and Telegram topics (NOT folders)
			if text == "/cleanup" {
				cfg, _ := config.LoadConfig()
				if len(cfg.Sessions) == 0 {
					telegram.SendMessage(cfg, chatID, threadID, "No sessions to clean up.")
					continue
				}

				// In multi-window architecture, we need to stop Claude in each project window
				// and close the windows before cleaning up

				var cleaned []string
				var errors []string

				for sessName, info := range cfg.Sessions {
					// Stop Claude process in the project window
					if target, err := tmux.GetCccWindowTarget(sessName); err == nil && target != "" {
						exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "C-c").Run()
						time.Sleep(100 * time.Millisecond)
						// Kill the window to clean up
						exec.Command(tmux.TmuxPath, "kill-window", "-t", target).Run()
					}

					// Delete telegram thread
					if info.TopicID > 0 && cfg.GroupID > 0 {
						if err := telegram.DeleteForumTopic(cfg, info.TopicID); err != nil {
							errors = append(errors, fmt.Sprintf("%s: %v", sessName, err))
						}
					}

					cleaned = append(cleaned, sessName)
				}

				// Clear all sessions from config
				cfg.Sessions = make(map[string]*config.SessionInfo)
				config.SaveConfig(cfg)

				msg := fmt.Sprintf("🧹 Cleaned %d sessions: %s", len(cleaned), strings.Join(cleaned, ", "))
				if len(errors) > 0 {
					msg += fmt.Sprintf("\n\n⚠️ Errors:\n%s", strings.Join(errors, "\n"))
				}
				telegram.SendMessage(cfg, chatID, threadID, msg)
				continue
			}

			// /new command - create/restart session
			if strings.HasPrefix(text, "/new") && isGroup {
				cfg, _ := config.LoadConfig()
				arg := strings.TrimSpace(strings.TrimPrefix(text, "/new"))

				// /new <name>[@provider] - create brand new session + topic
				if arg != "" {
					// Parse provider from argument: name@provider or name --provider provider
					sessionName := arg
					providerName := ""

					// Check for @provider syntax
					if idx := strings.Index(arg, "@"); idx > 0 {
						sessionName = arg[:idx]
						providerName = strings.TrimSpace(arg[idx+1:])
					} else if strings.Contains(arg, " --provider ") {
						// Check for --provider syntax
						parts := strings.SplitN(arg, " --provider ", 2)
						sessionName = strings.TrimSpace(parts[0])
						providerName = strings.TrimSpace(parts[1])
					}

					// Validate provider if specified using config.GetProvider()
					if providerName != "" {
						provider := config.GetProvider(cfg, providerName)
						if provider == nil {
							// List available providers
							available := config.GetProviderNames(cfg)
							msg := fmt.Sprintf("❌ Unknown provider '%s'\n\nAvailable providers: %s",
								providerName, strings.Join(available, ", "))
							telegram.SendMessage(cfg, chatID, threadID, msg)
							continue
						}
					}

					// Always show keyboard if no explicit provider selected
					if providerName == "" {
						// Check if session already exists
						existing, exists := cfg.Sessions[sessionName]
						if exists && existing != nil && existing.TopicID != 0 {
							telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
							continue
						}

						// Build provider selection keyboard using config.GetProviderNames()
						var buttons [][]telegram.InlineKeyboardButton

						// Get all providers (builtin + configured)
						providerNames := config.GetProviderNames(cfg)
						for _, name := range providerNames {
							label := name
							if cfg.ActiveProvider == name {
								label += " ⭐"
							}
							callbackData := fmt.Sprintf("new:%s:%s", sessionName, name)
							buttons = append(buttons, []telegram.InlineKeyboardButton{
								{Text: label, CallbackData: callbackData},
							})
						}

						msg := fmt.Sprintf("🤖 Select provider for '%s':", sessionName)
						telegram.SendMessageWithKeyboard(cfg, chatID, threadID, msg, buttons)
						continue
					}

					// Direct creation (single provider or provider specified)
					existing, exists := cfg.Sessions[sessionName]
					if exists && existing != nil && existing.TopicID != 0 {
						telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Session '%s' already exists. Use /new without args in that topic to restart.", sessionName))
						continue
					}
					// Use provider from arg or default to active provider
					if providerName == "" {
						providerName = cfg.ActiveProvider
					}
					topicID, err := telegram.CreateForumTopic(cfg, sessionName, providerName)
					if err != nil {
						telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
						continue
					}
					// Use pre-configured path if session was preset, otherwise resolve from name
					workDir := config.ResolveProjectPath(cfg, sessionName)
					if exists && existing != nil && existing.Path != "" {
						workDir = existing.Path
					}
					cfg.Sessions[sessionName] = &config.SessionInfo{
						TopicID:      topicID,
						Path:         workDir,
						ProviderName: providerName,
					}
					config.SaveConfig(cfg)

					// Ensure hooks are installed in the project directory
					if err := hooks.EnsureHooksForSession(cfg, sessionName, cfg.Sessions[sessionName]); err != nil {
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
					if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
						telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("❌ Failed to start session: %v", err))
					} else {
						telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("🚀 Session '%s' started!%s\n\nSend messages here to interact with Claude.", sessionName, providerMsg))
					}
					continue
				}

				// Without args - restart session in current topic
				if threadID > 0 {
					sessionName := session.GetSessionByTopic(cfg, threadID)
					if sessionName == "" {
						telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic. Use /new <name> to create one.")
						continue
					}
					sessionInfo := cfg.Sessions[sessionName]
					workDir := session.GetSessionWorkDir(cfg, sessionName, sessionInfo)
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
					if err := hooks.EnsureHooksForSession(cfg, sessionName, sessionInfo); err != nil {
						listenLog("[/new] Failed to verify/install hooks for %s: %v", sessionName, err)
					}

					if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
						telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
					} else {
						telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("🚀 Session '%s' restarted", sessionName))
					}
				} else {
					telegram.SendMessage(cfg, chatID, threadID, "Usage: /new <name> to create a new session")
				}
				continue
			}

			// /worktree command - create worktree session from existing session
			if strings.HasPrefix(text, "/worktree") && isGroup {
				args := strings.TrimSpace(strings.TrimPrefix(text, "/worktree"))
				parts := strings.Fields(args)

				if len(parts) < 2 {
					telegram.SendMessage(cfg, chatID, threadID, "Usage: /worktree <session_name> <worktree_name>\n\nCreates a new worktree session from an existing session's repository.")
					continue
				}

				baseSessionName := parts[0]
				worktreeName := parts[1]

				// Check if base session exists
				baseSession, exists := cfg.Sessions[baseSessionName]
				if !exists || baseSession == nil {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Base session '%s' not found. Use /new to create it first.", baseSessionName))
					continue
				}

				// Check if base session path is a git repository
				basePath := baseSession.Path
				if basePath == "" {
					basePath = config.ResolveProjectPath(cfg, baseSessionName)
				}
				// Use git to check if path is inside a work tree (handles worktrees and nested dirs)
				gitCmd := exec.Command("git", "-C", basePath, "rev-parse", "--is-inside-work-tree")
				if out, err := gitCmd.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Base session path is not a git repository: %s", basePath))
					continue
				}

				// Create unique session name for worktree: base_worktree
				worktreeSessionName := baseSessionName + "_" + worktreeName

				// Check if worktree session already exists
				if _, exists := cfg.Sessions[worktreeSessionName]; exists {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("⚠️ Worktree session '%s' already exists. Use /new in that topic to restart.", worktreeSessionName))
					continue
				}

				// Get provider from base session or active provider
				providerName := baseSession.ProviderName
				if providerName == "" {
					providerName = cfg.ActiveProvider
				}

				// Create Telegram topic
				topicID, err := telegram.CreateForumTopic(cfg, worktreeSessionName, providerName)
				if err != nil {
					telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to create topic: %v", err))
					continue
				}

				// Create session info with worktree metadata
				// Store the actual worktree path for unique identification
				worktreePath := filepath.Join(basePath, ".claude", "worktrees", worktreeName)
				cfg.Sessions[worktreeSessionName] = &config.SessionInfo{
					TopicID:      topicID,
					Path:         worktreePath, // Use actual worktree path for unique session resolution
					ProviderName: providerName,
					IsWorktree:   true,
					WorktreeName: worktreeName,
					BaseSession:  baseSessionName,
				}
				config.SaveConfig(cfg)

				// Ensure hooks are installed in the base project directory
				if err := hooks.EnsureHooksForSession(cfg, worktreeSessionName, cfg.Sessions[worktreeSessionName]); err != nil {
					listenLog("[/worktree] Failed to install hooks for %s: %v", worktreeSessionName, err)
				}

				// Switch to the worktree session in the single ccc window
				// Use basePath for cd (worktree dir is created by Claude Code with --worktree flag)
				if err := tmux.SwitchSessionInWindow(worktreeSessionName, basePath, providerName, "", worktreeName, false, false); err != nil {
					telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("❌ Failed to start session: %v", err))
					continue
				}

				providerMsg := ""
				if providerName != "" {
					providerMsg = fmt.Sprintf("\n🤖 Provider: %s", providerName)
				}
				telegram.SendMessage(cfg, cfg.GroupID, topicID, fmt.Sprintf("🌳 Worktree session '%s' started!\nBase: %s\nWorktree: %s%s\n\nSend messages here to interact with Claude.", worktreeSessionName, baseSessionName, worktreeName, providerMsg))
				continue
			}

			// Check if message is in a topic (interactive session)
			if isGroup && threadID > 0 {
				// Reload config to get latest sessions
				cfg, _ := config.LoadConfig()
				sessName := session.GetSessionByTopic(cfg, threadID)
				if sessName != "" {
					var target string
					var err error

					// Only switch if the requested session is different from current
					// Compare using tmux-safe names since window names are sanitized
					currentSession := tmux.GetCurrentSessionName()
					needsSwitch := currentSession != tmux.TmuxSafeName(sessName)

					if needsSwitch {
						// Switch to the correct session in the single ccc window
						sessionInfo := cfg.Sessions[sessName]
						workDir := session.GetSessionWorkDir(cfg, sessName, sessionInfo)
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
						if err := hooks.EnsureHooksForSession(cfg, sessName, sessionInfo); err != nil {
							listenLog("[sendToTmux] Failed to verify/install hooks for %s: %v", sessName, err)
						}

						// Switch to the session in the single ccc window (always restart when switching sessions)
						if err := tmux.SwitchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, false); err != nil {
							telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
							continue
						}

						// Get the target for the project window
						target, err = tmux.GetCccWindowTarget(sessName)
						if err != nil {
							telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to get ccc window: %v", err))
							continue
						}

						// Wait for Claude to be ready after switching
						time.Sleep(1 * time.Second)

						// Replay any undelivered terminal messages for this session
						undelivered := session.FindUndelivered(sessName, "terminal")
						for _, ur := range undelivered {
							if ur.Type == "user_prompt" && ur.Origin == "telegram" {
								if err := tmux.SendToTmuxFromTelegram(target, tmux.TmuxSafeName(sessName), ur.Text); err == nil {
									session.UpdateDelivery(sessName, ur.ID, "terminal_delivered", true)
								}
								time.Sleep(500 * time.Millisecond)
							}
						}

						listenLog("sendToTmux: target=%s session=%s (switched from %s)", target, sessName, currentSession)
					} else {
						// Already in the correct session, just get the target
						target, err = tmux.GetCccWindowTarget(sessName)
						if err != nil {
							telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to get ccc window: %v", err))
							continue
						}
						listenLog("sendToTmux: target=%s session=%s (already active)", target, sessName)
					}

					// Record in ledger before sending
					ledgerID := fmt.Sprintf("tg:%d", update.UpdateID)
					session.AppendMessage(&session.MessageRecord{
						ID:                ledgerID,
						Session:           sessName,
						Type:              "user_prompt",
						Text:              text,
						Origin:            "telegram",
						TerminalDelivered: false,
						TelegramDelivered: true,
					})

					if err := tmux.SendToTmuxFromTelegram(target, tmux.TmuxSafeName(sessName), text); err != nil {
						listenLog("sendToTmux FAILED: target=%s err=%v", target, err)
						telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to send: %v", err))
					} else {
						session.UpdateDelivery(sessName, ledgerID, "terminal_delivered", true)
					}
				} else {
					telegram.SendMessage(cfg, chatID, threadID, "⚠️ No session linked to this topic. Use /new <name> to create one.")
				}
				continue
			}

			// Private chat: run one-shot Claude
			if !isGroup {
				telegram.SendMessage(cfg, chatID, threadID, "🤖 Running Claude...")

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
							telegram.SendMessage(cfg, cid, 0, fmt.Sprintf("💥 Panic: %v", r))
						}
					}()
					output, err := RunClaude(p)
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
		}
	}
}

// handleNewWithProvider creates a session after provider selection via inline keyboard
func handleNewWithProvider(cfg *config.Config, cb *telegram.CallbackQuery, sessionName, providerName string) {
	// Check if session already exists
	existing, exists := cfg.Sessions[sessionName]
	if exists && existing != nil && existing.TopicID != 0 {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("⚠️ Session '%s' already exists.\n\nUse /new without args in that topic to restart.", sessionName))
		}
		return
	}

	// Create topic
	topicID, err := telegram.CreateForumTopic(cfg, sessionName, providerName)
	if err != nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Failed to create topic: %v", err))
		}
		return
	}

	// Resolve work directory
	workDir := config.ResolveProjectPath(cfg, sessionName)
	if exists && existing != nil && existing.Path != "" {
		workDir = existing.Path
	}

	// Create session
	cfg.Sessions[sessionName] = &config.SessionInfo{
		TopicID:      topicID,
		Path:         workDir,
		ProviderName: providerName,
	}
	config.SaveConfig(cfg)

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}

	// Switch to the new session in the single ccc window
	resultMsg := fmt.Sprintf("🚀 Session '%s' started!\n🤖 Provider: %s\n\nSend messages here to interact with Claude.", sessionName, providerName)
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, providerName, "", "", false, false); err != nil {
		resultMsg = fmt.Sprintf("❌ Failed to start session: %v", err)
	}

	if cb.Message != nil {
		telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

// handleProviderChange changes provider for an existing session via inline keyboard
func handleProviderChange(cfg *config.Config, cb *telegram.CallbackQuery, sessionName, providerName string) {
	// Check if session exists
	session, exists := cfg.Sessions[sessionName]
	if !exists || session == nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Session '%s' not found.", sessionName))
		}
		return
	}

	// Validate provider using config.GetProvider()
	// getProvider returns nil for unknown providers
	provider := config.GetProvider(cfg, providerName)
	if provider == nil {
		if cb.Message != nil {
			telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID,
				fmt.Sprintf("❌ Provider '%s' not found. Available: %v", providerName, config.GetProviderNames(cfg)))
		}
		return
	}

	// Update session provider
	session.ProviderName = providerName
	config.SaveConfig(cfg)

	resultMsg := fmt.Sprintf("✅ Provider changed to %s for session '%s'\n\nRestart with /new to apply the new provider.", provider.Name(), sessionName)
	if cb.Message != nil {
		telegram.EditMessageRemoveKeyboard(cfg, cb.Message.Chat.ID, cb.Message.MessageID, resultMsg)
	}
}

func PrintHelp() {
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
    -v, --Version           Show Version

For more info: https://github.com/kidandcat/ccc
`, Version)
}

const authTmuxSession = "claude-auth"

func handleAuth(cfg *config.Config, chatID, threadID int64) {
	if !authInProgress.TryLock() {
		telegram.SendMessage(cfg, chatID, threadID, "⚠️ Auth already in progress")
		return
	}

	telegram.SendMessage(cfg, chatID, threadID, "🔐 Starting Claude auth...")

	tmux.KillTmuxSession(authTmuxSession)
	time.Sleep(500 * time.Millisecond)

	home, _ := os.UserHomeDir()
	if err := exec.Command(tmux.TmuxPath, "new-session", "-d", "-s", authTmuxSession, "-c", home).Run(); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to create tmux session: %v", err))
		authInProgress.Unlock()
		return
	}

	// Build environment string for tmux from provider config
	envCmd := ""
	provider := config.GetActiveProvider(cfg)
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
			telegram.SendMessage(cfg, chatID, threadID, "✅ Claude is already authenticated!")
			tmux.KillTmuxSession(authTmuxSession)
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
		telegram.SendMessage(cfg, chatID, threadID, "❌ Could not find OAuth URL. Try again.")
		tmux.KillTmuxSession(authTmuxSession)
		authInProgress.Unlock()
		return
	}

	authWaitingCode = true
	telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("🔗 Open this URL and authorize:\n\n%s\n\nThen paste the code here.", oauthURL))
}

func handleAuthCode(cfg *config.Config, chatID, threadID int64, code string) {
	authWaitingCode = false
	code = strings.TrimSpace(code)

	telegram.SendMessage(cfg, chatID, threadID, "🔄 Sending code to Claude...")

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
			telegram.SendMessage(cfg, chatID, threadID, "✅ Auth successful! Claude is ready.")
			tmux.KillTmuxSession(authTmuxSession)
			authInProgress.Unlock()
			return
		}
	}

	out, _ := exec.Command(tmux.TmuxPath, "capture-pane", "-t", authTmuxSession, "-p").Output()
	pane := string(out)
	if strings.Contains(pane, "Login successful") || strings.Contains(pane, "❯") {
		telegram.SendMessage(cfg, chatID, threadID, "✅ Auth successful!")
	} else {
		telegram.SendMessage(cfg, chatID, threadID, "⚠️ Auth may have failed. Check VPS manually.")
	}

	tmux.KillTmuxSession(authTmuxSession)
	authInProgress.Unlock()
}

// installService installs the ccc background service (launchd on macOS, systemd on Linux)
func installService() error {
	home, _ := os.UserHomeDir()
	cccPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	if _, err := os.Stat("/Library"); err == nil {
		// macOS - create launchd plist
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ccc.plist")
		plistContent := fmt.Sprintf(`<?xml Version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist Version="1.0">
<dict>
    <key>Label</key>
    <string>com.ccc</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>listen</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>`, cccPath)
		if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
			return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
		}
		if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
			return fmt.Errorf("failed to write plist: %w", err)
		}
		fmt.Println("✅ launchd service installed")
	} else {
		// Linux - create systemd service
		servicePath := filepath.Join(home, ".config", "systemd", "user", "ccc.service")
		serviceContent := fmt.Sprintf(`[Unit]
Description=Claude Code Companion Listener
After=network.target

[Service]
ExecStart=%s listen
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, cccPath)
		if err := os.MkdirAll(filepath.Dir(servicePath), 0755); err != nil {
			return fmt.Errorf("failed to create systemd directory: %w", err)
		}
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
			return fmt.Errorf("failed to write service file: %w", err)
		}
		// Reload systemd and enable service
		exec.Command("systemctl", "--user", "daemon-reload").Run()
		exec.Command("systemctl", "--user", "enable", "ccc").Run()
		fmt.Println("✅ systemd service installed")
	}
	return nil
}
