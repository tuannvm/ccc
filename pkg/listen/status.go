package listen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/diagnostics"
	"github.com/tuannvm/ccc/pkg/lookup"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
	updatepkg "github.com/tuannvm/ccc/pkg/update"
)

// HandleStatusCommand is the compact management hub for session operations.
func HandleStatusCommand(cfg *configpkg.Config, chatID, threadID int64, text string, isGroup bool, updateOffset int) {
	fresh, err := configpkg.Load()
	if err != nil || fresh == nil {
		telegram.SendMessage(cfg, chatID, threadID, "ccc status\nconfig: unavailable")
		return
	}
	cfg = fresh
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/status"))
	if strings.HasPrefix(arg, "resume ") {
		HandleResumeCommand(cfg, chatID, threadID, "/resume "+strings.TrimSpace(strings.TrimPrefix(arg, "resume ")))
		return
	}

	switch arg {
	case "", "show":
		telegram.SendMessage(cfg, chatID, threadID, buildStatusMessage(cfg, threadID))
	case "restart":
		if cfg.IsTeamSession(threadID) {
			HandleTeamCreateCommand(cfg, chatID, threadID, "/team")
			return
		}
		HandleContinueCommand(cfg, chatID, threadID)
	case "stop":
		HandleStopCommand(cfg, chatID, threadID, isGroup)
	case "resume":
		HandleResumeCommand(cfg, chatID, threadID, "/resume")
	case "delete":
		HandleDeleteCommand(cfg, chatID, threadID)
	case "cleanup":
		HandleCleanupCommand(cfg, chatID, threadID)
	case "update":
		updatepkg.ApplyUpdate(cfg, chatID, threadID, updateOffset)
	case "service", "restart-service":
		HandleRestartCommand(cfg, chatID, threadID)
	case "system", "stats":
		telegram.SendMessage(cfg, chatID, threadID, diagnostics.GetSystemStats())
	default:
		telegram.SendMessage(cfg, chatID, threadID, "status commands: restart, stop, resume, delete, cleanup, update, service, system")
	}
}

func buildStatusMessage(cfg *configpkg.Config, threadID int64) string {
	if cfg == nil {
		return "ccc status\nconfig: unavailable"
	}

	if threadID > 0 {
		if cfg.IsTeamSession(threadID) {
			if sessInfo, ok := cfg.GetTeamSession(threadID); ok && sessInfo != nil {
				return buildTeamStatus(cfg, sessInfo)
			}
		}

		if sessName := lookup.GetSessionByTopic(cfg, threadID); sessName != "" {
			if info := cfg.Sessions[sessName]; info != nil {
				return buildSessionStatus(cfg, sessName, info)
			}
		}
	}

	var lines []string
	lines = append(lines, "ccc status")
	lines = append(lines, providerSummary(cfg, nil))
	lines = append(lines, fmt.Sprintf("sessions: %d", len(cfg.Sessions)))
	lines = append(lines, fmt.Sprintf("team sessions: %d", len(cfg.TeamSessions)))
	lines = append(lines, "")
	lines = append(lines, "daily: /new, /provider, /worktree, /status")
	return strings.Join(lines, "\n")
}

func buildSessionStatus(cfg *configpkg.Config, sessionName string, info *configpkg.SessionInfo) string {
	state := "stopped"
	if target, err := tmux.FindExistingWindow(sessionName); err == nil && target != "" {
		if tmux.WindowHasClaudeRunning(target, "") {
			state = "running"
		} else {
			state = "ready"
		}
	}

	workDir := lookup.GetSessionWorkDir(cfg, sessionName, info)
	if workDir == "" {
		workDir = info.Path
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("session: %s", sessionName))
	lines = append(lines, providerSummary(cfg, info))
	lines = append(lines, fmt.Sprintf("state: %s", state))
	if workDir != "" {
		lines = append(lines, fmt.Sprintf("path: %s", displayPath(workDir)))
	}
	if info.ClaudeSessionID != "" {
		lines = append(lines, fmt.Sprintf("conversation: %s", shortSessionID(info.ClaudeSessionID)))
	}
	if info.IsWorktree {
		lines = append(lines, fmt.Sprintf("worktree: %s", info.WorktreeName))
		if info.BaseSession != "" {
			lines = append(lines, fmt.Sprintf("base: %s", info.BaseSession))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "actions: /status restart, /status stop, /status resume, /status delete")
	return strings.Join(lines, "\n")
}

func buildTeamStatus(cfg *configpkg.Config, info *configpkg.SessionInfo) string {
	name := getSessionNameFromInfo(info)
	state := "stopped"
	target := "ccc-team:" + tmux.SafeName(name)
	if err := exec.Command(tmux.TmuxPath, "display-message", "-t", target, "-p", "#{window_name}").Run(); err == nil {
		state = "ready"
	}
	if _, err := os.Stat(info.Path); os.IsNotExist(err) {
		state = "missing path"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("team: %s", name))
	lines = append(lines, providerSummary(cfg, info))
	lines = append(lines, fmt.Sprintf("state: %s", state))
	if info.Path != "" {
		lines = append(lines, fmt.Sprintf("path: %s", displayPath(info.Path)))
	}
	lines = append(lines, fmt.Sprintf("layout: %s", info.GetLayoutName()))
	lines = append(lines, "")
	lines = append(lines, "actions: /status restart")
	return strings.Join(lines, "\n")
}
