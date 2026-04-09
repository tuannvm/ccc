package main

import (
	"fmt"
	"os"

	"github.com/tuannvm/ccc/pkg/auth"
	"github.com/tuannvm/ccc/pkg/cli"
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/diagnostics"
	notifypkg "github.com/tuannvm/ccc/pkg/notify"
	"github.com/tuannvm/ccc/pkg/relay"
	"github.com/tuannvm/ccc/pkg/service"
	setuppkg "github.com/tuannvm/ccc/pkg/setup"
	"github.com/tuannvm/ccc/pkg/tmux"
	teampkg "github.com/tuannvm/ccc/pkg/team"
)

const version = "1.7.0"

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
		if err := tmux.RunWithArgs(os.Args[2:], tmux.WorktreeAutoGenerate, ensureHooksForSession); err != nil {
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
		if err := setuppkg.SetGroupAuto(); err != nil {
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
		handleStopRetryFromArgs(os.Args[2:])

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
		installAll()

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
		if len(os.Args) < 3 {
			teampkg.PrintUsage()
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
		relay.RunRelayServerFromArgs(os.Args[2:])

	default:
		if err := notifypkg.HandleDefaultCommand(os.Args[1:], startSessionInCurrentDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func printHelp() {
	cli.PrintHelp(version)
}

func installAll() {
	if err := installSkill(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	if err := service.InstallService(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}
