package listen

import (
	"fmt"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/diagnostics"
	execpkg "github.com/tuannvm/ccc/pkg/exec"
	"github.com/tuannvm/ccc/pkg/telegram"
	updatepkg "github.com/tuannvm/ccc/pkg/update"
)

// HandleShellCommand executes a shell command and sends output back via Telegram.
func HandleShellCommand(config *configpkg.Config, chatID, threadID int64, cmdStr string) {
	output, err := execpkg.RunShell(cmdStr)
	if err != nil {
		output = fmt.Sprintf("⚠️ %s\n\nExit: %v", output, err)
	}
	telegram.SendMessage(config, chatID, threadID, output)
}

// HandleStatsCommand sends system statistics via Telegram.
func HandleStatsCommand(config *configpkg.Config, chatID, threadID int64) {
	stats := diagnostics.GetSystemStats()
	telegram.SendMessage(config, chatID, threadID, stats)
}

// HandleVersionCommand sends the current version via Telegram.
func HandleVersionCommand(config *configpkg.Config, chatID, threadID int64, version string) {
	telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("ccc %s", version))
}

// HandleRestartCommand sends a restart notification and restarts the ccc service.
func HandleRestartCommand(config *configpkg.Config, chatID, threadID int64) {
	telegram.SendMessage(config, chatID, threadID, "🔄 Restarting ccc service...")
	updatepkg.RestartProcess("listen")
}
