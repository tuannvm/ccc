package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/pkg/auth"
)

// handleCallbackQuery processes callback queries from inline keyboard button presses
func handleCallbackQuery(config *Config, cb *CallbackQuery) {
	if cb == nil {
		return
	}

	// Authorization check: depends on multi-user mode
	if !auth.IsAuthorizedCallback(config, cb) {
		return
	}

	telegram.AnswerCallbackQuery(config, cb.ID)

	// Parse callback data
	parts := strings.Split(cb.Data, ":")
	if len(parts) == 0 {
		return
	}

	// Handle different callback types
	switch parts[0] {
	case "new":
		// Provider selection for /new command: new:session_name:provider_name
		if len(parts) == 3 {
			sessionName := parts[1]
			providerName := parts[2]
			handleNewWithProvider(config, cb, sessionName, providerName)
		}
		return

	case "team":
		// Provider selection for /team command: team:team_name:provider_name
		if len(parts) == 3 {
			teamName := parts[1]
			providerName := parts[2]
			handleTeamWithProvider(config, cb, teamName, providerName)
		}
		return

	case "provider":
		// Provider selection for /provider command: provider:session_name:provider_name
		if len(parts) == 3 {
			sessionName := parts[1]
			providerName := parts[2]
			handleProviderChange(config, cb, sessionName, providerName)
		}
		return

	default:
		// Legacy: AskUserQuestion callback - session:questionIndex:totalQuestions:optionIndex
		handleQuestionCallback(config, cb, parts)
	}
}

// handleQuestionCallback handles legacy question selection callbacks
func handleQuestionCallback(config *Config, cb *CallbackQuery, parts []string) {
	if len(parts) < 3 {
		return
	}

	sessionName := parts[0]
	questionIndex, _ := strconv.Atoi(parts[1])
	var totalQuestions, optionIndex int
	if len(parts) == 4 {
		totalQuestions, _ = strconv.Atoi(parts[2])
		optionIndex, _ = strconv.Atoi(parts[3])
	} else {
		// Legacy format: session:questionIndex:optionIndex
		optionIndex, _ = strconv.Atoi(parts[2])
	}

	// Edit message to show selection and remove buttons
	if cb.Message != nil {
		originalText := cb.Message.Text
		newText := fmt.Sprintf("%s\n\n✓ Selected option %d", originalText, optionIndex+1)
		telegram.EditMessageRemoveKeyboard(config, cb.Message.Chat.ID, cb.Message.MessageID, newText)
	}

	// Switch to the session and send arrow keys
	sessionInfo, exists := config.Sessions[sessionName]
	if !exists || sessionInfo == nil {
		return
	}

	workDir := getSessionWorkDir(config, sessionName, sessionInfo)

	// Get worktree name if this is a worktree session
	worktreeName := ""
	if sessionInfo.IsWorktree {
		worktreeName = sessionInfo.WorktreeName
	}

	// Use stored Claude session ID to resume existing conversation
	resumeSessionID := sessionInfo.ClaudeSessionID

	// Switch to the session (preserve session context for callbacks)
	// Since currentSession == sessionName, this will skip restart
	if err := tmux.SwitchSessionInWindow(sessionName, workDir, sessionInfo.ProviderName, resumeSessionID, worktreeName, true, true); err != nil {
		return
	}

	target, _ := tmux.GetWindowTarget(sessionName)
	// Send arrow down keys to select option, then Enter
	for range optionIndex {
		exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Down").Run()
		time.Sleep(50 * time.Millisecond)
	}
	exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Enter").Run()
	listenLog("[callback] Selected option %d for %s (question %d/%d)", optionIndex, sessionName, questionIndex+1, totalQuestions)

	// After the last question, send Enter to confirm "Submit answers"
	if totalQuestions > 0 && questionIndex == totalQuestions-1 {
		time.Sleep(300 * time.Millisecond)
		exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "Enter").Run()
		listenLog("[callback] Auto-submitted answers for %s", sessionName)
	}
}
