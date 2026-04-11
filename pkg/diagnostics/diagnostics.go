package diagnostics

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/auth"
	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/pkg/transcribe"
)

// GetSystemStats returns machine stats (works on Linux and macOS)
func GetSystemStats() string {
	var sb strings.Builder
	hostname, _ := os.Hostname()
	sb.WriteString(fmt.Sprintf("🖥 %s\n\n", hostname))

	// Uptime
	if out, err := exec.Command("uptime").Output(); err == nil {
		sb.WriteString(fmt.Sprintf("⏱ %s\n", strings.TrimSpace(string(out))))
	}

	// CPU
	if out, err := exec.Command("nproc").Output(); err == nil {
		sb.WriteString(fmt.Sprintf("🧠 CPUs: %s", strings.TrimSpace(string(out))))
	} else if out, err := exec.Command("sysctl", "-n", "hw.ncpu").Output(); err == nil {
		sb.WriteString(fmt.Sprintf("🧠 CPUs: %s", strings.TrimSpace(string(out))))
	}
	if load, err := exec.Command("bash", "-c", "cat /proc/loadavg 2>/dev/null || sysctl -n vm.loadavg 2>/dev/null").Output(); err == nil {
		loadStr := strings.TrimSpace(string(load))
		if loadStr != "" {
			sb.WriteString(fmt.Sprintf(" (load: %s)", loadStr))
		}
	}
	sb.WriteString("\n")

	// Memory
	if out, err := exec.Command("bash", "-c", "free -h --si 2>/dev/null | awk '/^Mem:/{print $3\"/\"$2}' || vm_stat | head -5").Output(); err == nil {
		sb.WriteString(fmt.Sprintf("💾 RAM: %s\n", strings.TrimSpace(string(out))))
	}

	// Disk
	home, _ := os.UserHomeDir()
	if out, err := exec.Command("df", "-h", home).Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 5 {
				sb.WriteString(fmt.Sprintf("💿 Disk: %s used of %s (%s available)\n", fields[2], fields[1], fields[3]))
			}
		}
	}

	// Top processes
	if out, err := exec.Command("bash", "-c", "ps aux --sort=-%mem 2>/dev/null | head -6 || ps aux -m | head -6").Output(); err == nil {
		sb.WriteString("\n📊 Top processes:\n")
		lines := strings.Split(string(out), "\n")
		for i, line := range lines {
			if i > 5 {
				break
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if i > 0 {
				fields := strings.Fields(line)
				if len(fields) >= 11 {
					sb.WriteString(fmt.Sprintf("  %-8s %5s%% %5s%% %s\n", fields[0], fields[2], fields[3], fields[10]))
				}
			} else {
				sb.WriteString(fmt.Sprintf("  %s\n", line))
			}
		}
	}

	// Active tmux sessions
	if tmux.TmuxPath != "" {
		if out, err := exec.Command(tmux.TmuxPath, "list-sessions", "-F", "#{session_name} #{session_windows}w").Output(); err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			sb.WriteString(fmt.Sprintf("\n🐚 tmux sessions (%d):\n", len(lines)))
			for _, line := range lines {
				if line != "" {
					sb.WriteString(fmt.Sprintf("  • %s\n", line))
				}
			}
		}
	}

	// Active CCC sessions
	config, err := configpkg.Load()
	if err == nil && config != nil && len(config.Sessions) > 0 {
		sb.WriteString(fmt.Sprintf("\n📂 CCC sessions (%d):\n", len(config.Sessions)))
		for name, info := range config.Sessions {
			if info == nil {
				continue
			}
			status := "stopped"
			if info.Panes != nil {
				for _, pane := range info.Panes {
					if pane != nil && pane.ClaudeSessionID != "" {
						status = "active"
						break
					}
				}
			}
			sb.WriteString(fmt.Sprintf("  • %s (%s)\n", name, status))
		}
	}

	// Load averages
	sb.WriteString("\n📈 Load averages: ")
	if out, err := exec.Command("bash", "-c", "cat /proc/loadavg 2>/dev/null | awk '{print $1, $2, $3}' || sysctl -n vm.loadavg 2>/dev/null | awk '{print $2, $3, $4}'").Output(); err == nil {
		sb.WriteString(string(out))
	} else {
		sb.WriteString("(unavailable)\n")
	}

	return sb.String()
}

// Doctor checks all dependencies and configuration
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
	config, err := configpkg.Load()
	if err != nil {
		fmt.Println("❌ not found")
		fmt.Println("   Run: ccc setup <bot_token>")
		allGood = false
	} else {
		fmt.Printf("✅ %s\n", configpkg.GetConfigPath())

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
		var settings map[string]any
		if json.Unmarshal(data, &settings) == nil {
			if hooks, ok := settings["hooks"].(map[string]any); ok {
				var installed []string
				if stop, has := hooks["Stop"].([]any); has && len(stop) > 0 {
					installed = append(installed, "Stop")
				}
				if pre, has := hooks["PreToolUse"].([]any); has && len(pre) > 0 {
					installed = append(installed, "PreToolUse")
				}
				if notif, has := hooks["Notification"].([]any); has && len(notif) > 0 {
					installed = append(installed, "Notification")
				}
				if len(installed) == 3 {
					fmt.Printf("✅ installed (%s)\n", strings.Join(installed, ", "))
				} else if len(installed) > 0 {
					fmt.Printf("⚠️  partial (%s) - run: ccc install\n", strings.Join(installed, ", "))
					allGood = false
				} else {
					fmt.Println("❌ not installed (run: ccc install)")
					allGood = false
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
	transcribe.DoctorCheckWhisper()

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
	if config != nil && auth.IsOTPEnabled(config) {
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
