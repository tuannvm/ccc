package cli

import "fmt"

// PrintHelp displays the CLI help text.
func PrintHelp(version string) {
	fmt.Printf(`ccc - Claude Code Companion v%s

Your companion for Claude Code - control sessions remotely via Telegram and tmux.

USAGE:
    ccc                     Start/attach tmux session in current directory
    ccc <message>           Send notification (if away mode is on)

COMMANDS:
    setup <token>           Complete setup (bot, hook, service - all in one!)
    doctor                  Check all dependencies and configuration
    config                  Show/set configuration values
    config projects-dir <path>  Set base directory for projects
    config oauth-token <token>  Set OAuth token
    status                  Show the session mapped to this directory
    status all              List Telegram-backed sessions and local bridges
    status attach <session> Attach/resume a known CCC session by name
    status restart          Continue/restart the current directory session
    provider [name]         View or change current session provider
    setgroup                Configure Telegram group for topics (if skipped during setup)
    listen                  Start the Telegram bot listener manually
    install                 Install skill and background service
    install-hooks           Install hooks in current project directory
    uninstall               Remove CCC skill
    cleanup-hooks           Clean up old global hooks from pre-1.2 installations
    send <file>             Send file to current session's Telegram topic
    relay [port]            Start relay server for large files (default: 8080)
    run                     Run selected backend directly (used by tmux sessions)

TELEGRAM COMMANDS:
    /new <name>             Start or restart a session
    /new <name>@provider    Start with a specific provider
    /provider [name]        View or change current session provider
    /worktree [name]        Create a worktree session
    /status                 Show current session state
    /status restart         Restart with current conversation
    /status stop            Interrupt current Claude execution
    /status resume          List available Claude conversations
    /status delete          Delete current session and topic

LEGACY TELEGRAM ALIASES:
    /continue, /resume, /stop, /delete, /providers, /team, /update, /restart

OTP (permission approval):
    When OTP is enabled (via 'ccc setup'), Claude's permission requests
    are forwarded to Telegram. Reply with your OTP code to approve.

FLAGS:
    -h, --help              Show this help
    -v, --version           Show version

For more info: https://github.com/tuannvm/ccc
`, version)
}
