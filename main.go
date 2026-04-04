package main

import (
	"fmt"
	"os"
	"strings"
)

const version = "1.7.0"

const WorktreeAutoGenerate = "AUTO" // Special value for auto-generating worktree names

func init() {
	initPaths()
}

func main() {
	// Handle flags
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printHelp()
			return
		case "-v", "--version", "version":
			fmt.Printf("ccc version %s\n", version)
			return
		}
	}

	if len(os.Args) < 2 {
		// No args: start/attach tmux session with topic
		if err := startSession(false); err != nil {
			os.Exit(1)
		}
		return
	}

	// Check for -c flag (continue) as first arg
	if os.Args[1] == "-c" {
		if err := startSession(true); err != nil {
			os.Exit(1)
		}
		return
	}

	switch os.Args[1] {
	case "run":
		// Run claude directly (used inside tmux sessions)
		// Flags: -c (continue), --resume <session-id>, --provider <name>, --worktree [name]
		continueSession := false
		var resumeSessionID string
		var providerOverride string
		var worktreeName string
		args := os.Args[2:]
		for i := 0; i < len(args); i++ {
			if args[i] == "-c" {
				continueSession = true
			} else if args[i] == "--resume" && i+1 < len(args) {
				resumeSessionID = args[i+1]
				i++
			} else if args[i] == "--provider" && i+1 < len(args) {
				providerOverride = args[i+1]
				i++
			} else if args[i] == "--worktree" {
				// --worktree with optional value
				// If next arg exists and doesn't start with "-", use it as name
				// Otherwise, use WorktreeAutoGenerate for auto-generation
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					worktreeName = args[i+1]
					i++
				} else {
					worktreeName = WorktreeAutoGenerate
				}
			}
		}
		if err := runClaudeRaw(continueSession, resumeSessionID, providerOverride, worktreeName); err != nil {
			os.Exit(1)
		}
		return
	case "setup":
		if len(os.Args) < 3 {
			fmt.Println("Usage: ccc setup <bot_token>")
			os.Exit(1)
		}
		if err := setup(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "doctor":
		doctor()

	case "config":
		config, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(os.Args) < 3 {
			// Show current config
			fmt.Printf("projects_dir: %s\n", getProjectsDir(config))
			if config.OAuthToken != "" {
				fmt.Println("oauth_token: configured")
			} else {
				fmt.Println("oauth_token: not set")
			}
			if config.TranscriptionLang != "" {
				fmt.Printf("transcription_lang: %s\n", config.TranscriptionLang)
			} else {
				fmt.Println("transcription_lang: not set (auto-detect)")
			}
			if isOTPEnabled(config) {
				fmt.Println("otp: enabled")
			} else {
				fmt.Println("otp: disabled (enable with: ccc setup <bot_token>)")
			}
			fmt.Println("\nUsage: ccc config <key> <value>")
			fmt.Println("  ccc config projects-dir ~/Projects")
			fmt.Println("  ccc config oauth-token <token>")
			fmt.Println("  ccc config transcription-lang es")
			os.Exit(0)
		}
		key := os.Args[2]
		if len(os.Args) < 4 {
			// Show specific key
			switch key {
			case "projects-dir":
				fmt.Println(getProjectsDir(config))
			case "oauth-token":
				if config.OAuthToken != "" {
					fmt.Println("configured")
				} else {
					fmt.Println("not set")
				}
			case "bot-token":
				if config.BotToken != "" {
					fmt.Println("configured")
				} else {
					fmt.Println("not set")
				}
			case "transcription-lang":
				if config.TranscriptionLang != "" {
					fmt.Println(config.TranscriptionLang)
				} else {
					fmt.Println("not set (auto-detect)")
				}
			case "otp":
				if isOTPEnabled(config) {
					fmt.Println("enabled")
				} else {
					fmt.Println("disabled")
				}
			default:
				fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
				os.Exit(1)
			}
			os.Exit(0)
		}
		value := os.Args[3]
		switch key {
		case "projects-dir":
			config.ProjectsDir = value
			if err := saveConfig(config); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ projects_dir set to: %s\n", getProjectsDir(config))
		case "oauth-token":
			config.OAuthToken = value
			if err := saveConfig(config); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ OAuth token saved")
		case "bot-token":
			config.BotToken = value
			if err := saveConfig(config); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ Bot token saved")
		case "transcription-lang":
			config.TranscriptionLang = value
			if err := saveConfig(config); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ Transcription language set to: %s\n", value)
		case "otp":
			fmt.Fprintf(os.Stderr, "Permission mode can only be changed via: ccc setup <bot_token>\n")
			os.Exit(1)
		default:
			fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
			os.Exit(1)
		}

	case "setgroup":
		config, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := setGroup(config); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "listen":
		if err := listen(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-permission":
		if err := handlePermissionHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-question":
		// Legacy: redirect to permission hook
		if err := handlePermissionHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-stop":
		if err := handleStopHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-stop-retry":
		// Background process: retry transcript read 3x at 2s intervals
		// Args: sessName topicID transcriptPath
		if len(os.Args) < 5 {
			os.Exit(1)
		}
		var tid int64
		fmt.Sscan(os.Args[3], &tid)
		handleStopRetry(os.Args[2], tid, os.Args[4])

	case "hook-post-tool":
		if err := handlePostToolHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-user-prompt":
		if err := handleUserPromptHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-notification":
		if err := handleNotificationHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-session-start":
		if err := handleSessionStartHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "install":
		if err := installHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		if err := installSkill(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		if err := installService(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}

	case "uninstall":
		if err := uninstallHook(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not uninstall hooks: %v\n", err)
		}
		uninstallSkill()
		fmt.Println("✅ CCC uninstalled")

	case "cleanup-hooks":
		if err := cleanupGlobalHooks(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "install-hooks":
		if err := installHooksToCurrentDir(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "send":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: ccc send <file>\n")
			os.Exit(1)
		}
		if err := handleSendFile(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "team":
		// Team session commands: ccc team new|list|attach|start|stop|delete
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: ccc team <command> [args...]\n")
			fmt.Fprintf(os.Stderr, "\nCommands:\n")
			fmt.Fprintf(os.Stderr, "  new <name> --topic <id>     Create a new team session (3 panes)\n")
			fmt.Fprintf(os.Stderr, "  list                        List all team sessions\n")
			fmt.Fprintf(os.Stderr, "  attach <name> [--role <r>]  Attach to a team session\n")
			fmt.Fprintf(os.Stderr, "  start <name>                Start Claude in a team session\n")
			fmt.Fprintf(os.Stderr, "  stop <name>                 Stop a team session\n")
			fmt.Fprintf(os.Stderr, "  delete <name>               Delete a team session\n")
			os.Exit(1)
		}
		teamCmd := NewTeamCommands()
		if err := teamCmd.Run(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "start":
		// start <name> <work-dir> <prompt>
		// Creates a Telegram topic, tmux session with Claude, and sends the prompt (detached)
		if len(os.Args) < 5 {
			fmt.Fprintf(os.Stderr, "Usage: ccc start <session-name> <work-dir> <prompt>\n")
			os.Exit(1)
		}
		if err := startDetached(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "relay":
		port := "8080"
		if len(os.Args) >= 3 {
			port = os.Args[2]
		}
		runRelayServer(port)

	default:
		message := strings.Join(os.Args[1:], " ")

		// If a message is provided, try to send it as a notification first (preserves old behavior)
		if message != "" {
			config, err := loadConfig()
			if err == nil && config.Away {
				// Away mode is on: send as notification to existing session
				sendErr := send(message)
				if sendErr == nil {
					// Message sent successfully (to topic, private chat, or skipped because Away mode off)
					return
				}
				// Send failed - determine if this is a config/setup error or transient error
				// Config/setup errors should fall through to session creation
				// Transient errors should exit immediately
				errMsg := strings.ToLower(sendErr.Error())
				isConfigError := strings.Contains(errMsg, "not configured") ||
					strings.Contains(errMsg, "chat not found") ||
					strings.Contains(errMsg, "unauthorized") ||
					strings.Contains(errMsg, "forbidden") ||
					strings.Contains(errMsg, "bad request")

				if isConfigError {
					// Config/setup error - fall through to session creation with helpful message
					fmt.Fprintf(os.Stderr, "Note: %v\n", sendErr)
				} else {
					// Transient error (network, rate limit, etc.) - report it and don't fall through
					fmt.Fprintf(os.Stderr, "Error: %v\n", sendErr)
					os.Exit(1)
				}
			}
		}

		// Default behavior: start/attach to session in current directory
		if err := startSessionInCurrentDir(message); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}
