package main

import (
	"github.com/tuannvm/ccc/pkg/lookup"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/session"
)

func getSessionWorkDir(config *Config, sessionName string, sessionInfo *SessionInfo) string {
	return lookup.GetSessionWorkDir(config, sessionName, sessionInfo)
}

func findSessionForPath(config *Config, cwd string) (string, *SessionInfo) {
	return lookup.FindSessionForPath(config, cwd)
}

func startSession(continueSession bool) error {
	return listenpkg.StartSession(continueSession, tmux.AttachToSession)
}

func startDetached(name string, workDir string, prompt string) error {
	return listenpkg.StartDetached(name, workDir, prompt)
}

func startSessionInCurrentDir(message string) error {
	return listenpkg.StartSessionInCurrentDirAuto(message, tmux.AttachToSession,
		lookup.FindSessionForPath, lookup.GenerateUniqueSessionName)
}

func inferRoleFromTranscriptPath(transcriptPath string) session.PaneRole {
	return session.InferRoleFromTranscriptPath(transcriptPath)
}

func persistClaudeSessionID(config *Config, sessName string, claudeSessionID string, transcriptPath string) {
	lookup.PersistClaudeSessionID(config, sessName, claudeSessionID, transcriptPath)
}
