package main

import (
	"fmt"
	"os"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/relay"
	notifypkg "github.com/tuannvm/ccc/pkg/notify"
	"github.com/tuannvm/ccc/pkg/tmux"
	
	"github.com/tuannvm/ccc/pkg/auth"
	"github.com/tuannvm/ccc/pkg/cli"
	
	"github.com/tuannvm/ccc/pkg/diagnostics"
	
	"github.com/tuannvm/ccc/pkg/service"
	
)

const version = "1.7.0"

const WorktreeAutoGenerate = "AUTO" // Special value for auto-generating worktree names

func init() {
	tmux.InitPaths()
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
		ra := tmux.ParseRunArgs(os.Args[2:], WorktreeAutoGenerate)
		if err := runClaudeRaw(ra.ContinueSession, ra.ResumeSessionID, ra.ProviderOverride, ra.WorktreeName); err != nil {
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
		diagnostics.Doctor()

	case "config":
		configpkg.HandleConfigCommand(os.Args[2:], auth.IsOTPEnabled)

	case "setgroup":
		config, err := configpkg.Load()
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

	case "install":
		if err := installSkill(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		if err := service.InstallService(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
		if err := relay.HandleSendFile(os.Args[2]); err != nil {
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
		relay.RunRelayServer(port)

	default:
		message := strings.Join(os.Args[1:], " ")

		// If a message is provided, try to send it as a notification first
		if message != "" && notifypkg.TryNotifyIfAway(message) {
			return
		}

		// Default behavior: start/attach to session in current directory
		if err := startSessionInCurrentDir(message); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func printHelp() {
	cli.PrintHelp(version)
}
