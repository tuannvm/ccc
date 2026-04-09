package main

import (
	"time"

	"github.com/tuannvm/ccc/pkg/lookup"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/session"
)

// tmuxSafeName is in tmux_session.go (wrapper for tmux.SafeName)

// getSessionWorkDir delegates to lookup.GetSessionWorkDir
func getSessionWorkDir(config *Config, sessionName string, sessionInfo *SessionInfo) string {
	return lookup.GetSessionWorkDir(config, sessionName, sessionInfo)
}

// findSessionForPath delegates to lookup.FindSessionForPath
func findSessionForPath(config *Config, cwd string) (string, *SessionInfo) {
	return lookup.FindSessionForPath(config, cwd)
}

func getWorktreeNames(basePath string) map[string]bool {
	return lookup.GetWorktreeNames(basePath)
}

func waitForNewWorktree(basePath string, existingNames map[string]bool, timeout time.Duration) string {
	return lookup.WaitForNewWorktree(basePath, existingNames, timeout)
}

// startSession delegates to listen.StartSession
func startSession(continueSession bool) error {
	return listenpkg.StartSession(continueSession, tmux.AttachToSession)
}

// startDetached delegates to listen.StartDetached
func startDetached(name string, workDir string, prompt string) error {
	return listenpkg.StartDetached(name, workDir, prompt)
}

// startSessionInCurrentDir loads config and delegates to listen.StartSessionInCurrentDir
func startSessionInCurrentDir(message string) error {
	return listenpkg.StartSessionInCurrentDirAuto(message, tmux.AttachToSession,
		lookup.FindSessionForPath, lookup.GenerateUniqueSessionName)
}

// --- Session persist delegates ---

func inferRoleFromTranscriptPath(transcriptPath string) session.PaneRole {
	return session.InferRoleFromTranscriptPath(transcriptPath)
}

func inferRoleFromTmuxPane(sessionName string) session.PaneRole {
	return lookup.InferRoleFromTmuxPane(sessionName)
}

func persistClaudeSessionID(config *Config, sessName string, claudeSessionID string, transcriptPath string) {
	lookup.PersistClaudeSessionID(config, sessName, claudeSessionID, transcriptPath)
}
