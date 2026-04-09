package main

import (
	"github.com/tuannvm/ccc/pkg/lookup"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
	"github.com/tuannvm/ccc/pkg/tmux"
)

func generateUniqueSessionName(config *Config, cwd string, basename string) string {
	return lookup.GenerateUniqueSessionName(config, cwd, basename)
}

func attachToExistingSession(config *Config, sessionName string, sessionInfo *SessionInfo, message string) error {
	return listenpkg.AttachToExistingSession(config, sessionName, sessionInfo, message, tmux.AttachToSession)
}

func startLocalSession(config *Config, sessionName, workDir, message string) error {
	return listenpkg.StartLocalSession(config, sessionName, workDir, message, tmux.AttachToSession)
}

func startTelegramSession(config *Config, sessionName, workDir, message string) error {
	return listenpkg.StartTelegramSession(config, sessionName, workDir, message, tmux.AttachToSession)
}

func attachToTmuxSession(sessionName string) error {
	return tmux.AttachToSession(sessionName)
}
