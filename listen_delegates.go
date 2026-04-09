package main

import (
	"github.com/tuannvm/ccc/pkg/lookup"
	"github.com/tuannvm/ccc/pkg/tmux"
	execpkg "github.com/tuannvm/ccc/pkg/exec"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
)

func handleWorktreeCommand(config *Config, chatID, threadID int64, text string) {
	listenpkg.HandleWorktreeCommand(config, chatID, threadID, text, WorktreeAutoGenerate)
}

func handlePrivateChat(config *Config, msg TelegramMessage, chatID, threadID int64) {
	listenpkg.HandlePrivateChat(config, msg, chatID, threadID, execpkg.RunClaudeOneShot)
}

func runClaudeRaw(continueSession bool, resumeSessionID string, providerOverride string, worktreeName string) error {
	return tmux.RunClaudeRaw(continueSession, resumeSessionID, providerOverride, worktreeName, WorktreeAutoGenerate, ensureHooksForSession)
}

func findSession(config *Config, cwd string, claudeSessionID string) (string, int64) {
	return lookup.FindSession(config, cwd, claudeSessionID)
}

func getSessionByTopic(config *Config, topicID int64) string {
	return lookup.GetSessionByTopic(config, topicID)
}

func findSessionByClaudeID(config *Config, claudeSessionID string) (string, int64) {
	return lookup.FindSessionByClaudeID(config, claudeSessionID)
}

func findSessionByCwd(config *Config, cwd string) (string, int64) {
	return lookup.FindSessionByCwd(config, cwd)
}
