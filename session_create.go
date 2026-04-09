package main

import (
	"github.com/tuannvm/ccc/pkg/lookup"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// generateUniqueSessionName delegates to lookup.GenerateUniqueSessionName
func generateUniqueSessionName(config *Config, cwd string, basename string) string {
	return lookup.GenerateUniqueSessionName(config, cwd, basename)
}

// attachToExistingSession delegates to listen.AttachToExistingSession
func attachToExistingSession(config *Config, sessionName string, sessionInfo *SessionInfo, message string) error {
	return listenpkg.AttachToExistingSession(config, sessionName, sessionInfo, message, tmux.AttachToSession)
}

// startLocalSession delegates to listen.StartLocalSession
func startLocalSession(config *Config, sessionName, workDir, message string) error {
	return listenpkg.StartLocalSession(config, sessionName, workDir, message, tmux.AttachToSession)
}

// startTelegramSession delegates to listen.StartTelegramSession
func startTelegramSession(config *Config, sessionName, workDir, message string) error {
	return listenpkg.StartTelegramSession(config, sessionName, workDir, message, tmux.AttachToSession)
}

// attachToTmuxSession delegates to tmux.AttachToSession
func attachToTmuxSession(sessionName string) error {
	return tmux.AttachToSession(sessionName)
}
