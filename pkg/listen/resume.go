package listen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/lookup"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// HandleResumeCommand handles the /resume command - manage Claude session IDs
func HandleResumeCommand(cfg *configpkg.Config, chatID, threadID int64, text string) {
	cfg, _ = configpkg.Load()
	sessName := lookup.GetSessionByTopic(cfg, threadID)
	if sessName == "" {
		telegram.SendMessage(cfg, chatID, threadID, "❌ No session mapped to this topic.")
		return
	}
	sessionInfo := cfg.Sessions[sessName]
	workDir := lookup.GetSessionWorkDir(cfg, sessName, sessionInfo)
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/resume"))
	if arg == "" {
		// List available Claude session IDs for this project
		home, _ := os.UserHomeDir()
		var pathComponent string
		if filepath.IsAbs(workDir) {
			pathComponent = strings.ReplaceAll(workDir, "/", "-")
			if strings.HasPrefix(pathComponent, "/") {
				pathComponent = "-" + pathComponent[1:]
			}
		} else {
			pathComponent = workDir
		}

		transcriptDir := configpkg.ResolveTranscriptDir(cfg, sessionInfo, home, pathComponent)
		if transcriptDir == "" {
			telegram.SendMessage(cfg, chatID, threadID, "📋 No previous Claude sessions found for this project.")
			return
		}

		var sessions []string
		entries, _ := os.ReadDir(transcriptDir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
				sessions = append(sessions, sessionID)
			}
		}

		if len(sessions) == 0 {
			telegram.SendMessage(cfg, chatID, threadID, "📋 No previous Claude sessions found for this project.")
			return
		}

		var msg []string
		msg = append(msg, "📋 Available Claude sessions for this project:")
		currentID := sessionInfo.ClaudeSessionID
		if currentID != "" {
			msg = append(msg, fmt.Sprintf("  • %s (current)", currentID))
		}
		for i := len(sessions) - 1; i >= 0; i-- {
			sessionID := sessions[i]
			if sessionID != currentID {
				msg = append(msg, fmt.Sprintf("  • %s", sessionID))
			}
		}
		msg = append(msg, "", fmt.Sprintf("Usage: /resume <session_id> to switch sessions"))
		telegram.SendMessage(cfg, chatID, threadID, strings.Join(msg, "\n"))
		return
	}

	// Resume specific session
	home, _ := os.UserHomeDir()
	var pathComponent string
	if filepath.IsAbs(workDir) {
		pathComponent = strings.ReplaceAll(workDir, "/", "-")
		if strings.HasPrefix(pathComponent, "/") {
			pathComponent = "-" + pathComponent[1:]
		}
	} else {
		pathComponent = workDir
	}

	transcriptDir := configpkg.ResolveTranscriptDir(cfg, sessionInfo, home, pathComponent)
	transcriptPath := filepath.Join(transcriptDir, arg+".jsonl")

	if _, err := os.Stat(transcriptPath); os.IsNotExist(err) {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Session not found: %s\n\nUse /resume to list available sessions.", arg))
		return
	}

	oldID := sessionInfo.ClaudeSessionID
	sessionInfo.ClaudeSessionID = arg
	configpkg.Save(cfg)

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

	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		os.MkdirAll(workDir, 0755)
	}
	worktreeName := ""
	if sessionInfo.IsWorktree {
		worktreeName = sessionInfo.WorktreeName
	}

	if err := tmux.SwitchSessionInWindow(sessName, workDir, sessionInfo.ProviderName, arg, worktreeName, false, false); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("❌ Failed to switch session: %v", err))
	} else {
		sessionInfo.ClaudeSessionID = arg
		configpkg.Save(cfg)
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("🚀 Session '%s' resumed with Claude session %s", sessName, arg))
	}
}
