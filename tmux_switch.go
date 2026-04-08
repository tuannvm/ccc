package main

import (
	"github.com/tuannvm/ccc/pkg/tmux"
)

// switchSessionInWindow switches the context to the project's window in the ccc session
// Each project gets its own named window within the main "ccc" session
// If skipRestart is true and the requested session is already active, it will skip restarting
func switchSessionInWindow(sessionName string, workDir string, providerName string, sessionID string, worktreeName string, continueSession bool, skipRestart bool) error {
	return tmux.SwitchSessionInWindow(sessionName, workDir, providerName, sessionID, worktreeName, continueSession, skipRestart)
}
