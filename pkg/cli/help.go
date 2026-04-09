package cli

import "fmt"

// PrintHelp displays the CLI help text.
func PrintHelp(version string) {
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
    install                 Install skill and background service
    install-hooks           Install hooks in current project directory
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
