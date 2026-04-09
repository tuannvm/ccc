package main

import (
	"fmt"
	"os"

	"github.com/tuannvm/ccc/pkg/auth"
	"github.com/tuannvm/ccc/pkg/cli"
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/diagnostics"
	"github.com/tuannvm/ccc/pkg/hooks"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
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
		if err := listenpkg.StartSession(false); err != nil {
			os.Exit(1)
		}
		return
	}

	// Check for -c flag (continue) as first arg
	if os.Args[1] == "-c" {
		if err := listenpkg.StartSession(true); err != nil {
			os.Exit(1)
		}
		return
	}

	cb := newHandlerCallbacks()

	switch os.Args[1] {
	case "run":
		if err := tmux.RunWithArgs(os.Args[2:], tmux.WorktreeAutoGenerate, ensureHooksForSession); err != nil {
			os.Exit(1)
		}
		return
	case "setup":
		if err := setuppkg.SetupFromArgs(os.Args[2:]); err != nil {
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
		if err := listenpkg.Run(version); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-permission":
		if err := hooks.HandlePermissionHook(cb); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-question":
		if err := hooks.HandlePermissionHook(cb); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-stop":
		if err := hooks.HandleStopHook(cb); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-stop-retry":
		hooks.HandleStopRetryFromArgs(os.Args[2:], handleStopRetry)

	case "hook-post-tool":
		if err := hooks.HandlePostToolHook(cb); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-user-prompt":
		if err := hooks.HandleUserPromptHook(cb); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "hook-notification":
		if err := hooks.HandleNotificationHook(cb); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "install":
		service.InstallAll()

	case "install-hooks":
		if err := installHooksToCurrentDir(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "send":
		if err := relay.HandleSendFileFromArgs(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "team":
		if err := teampkg.RunFromArgs(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "start":
		if err := listenpkg.StartDetachedFromArgs(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "relay":
		relay.RunRelayServerFromArgs(os.Args[2:])

	default:
		if err := notifypkg.HandleDefaultCommand(os.Args[1:], listenpkg.StartSessionInCurrentDirAuto); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func printHelp() {
	cli.PrintHelp(version)
}
