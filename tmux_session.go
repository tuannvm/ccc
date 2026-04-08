package main

import (
	"github.com/tuannvm/ccc/pkg/tmux"
)

// cccSessionExists checks if the main "ccc" tmux session exists without creating it
// Returns true if session exists, false otherwise
func cccSessionExists() bool {
	return tmux.SessionExists()
}

// ensureCccSession ensures the main "ccc" tmux session exists
// Returns the session name "ccc"
func ensureCccSession() (string, error) {
	return tmux.EnsureSession()
}

// ensureProjectWindow ensures a window exists for the project within the ccc session
// Returns the target string "ccc:TommyClaw" (session:window format)
func ensureProjectWindow(sessionName string) (string, error) {
	return tmux.EnsureProjectWindow(sessionName)
}

// findExistingWindow finds an existing window without creating it
// Returns the target string "ccc:TommyClaw" if window exists, empty string otherwise
func findExistingWindow(sessionName string) (string, error) {
	return tmux.FindExistingWindow(sessionName)
}

// getCccWindowTarget returns the target for a project's window in the ccc session
// Takes a session name (e.g., "TommyClaw") and returns "ccc:TommyClaw"
func getCccWindowTarget(sessionName string) (string, error) {
	return tmux.GetWindowTarget(sessionName)
}

// getCurrentSessionName returns the session name currently displayed in the ccc session
// Returns empty string if unable to determine
func getCurrentSessionName() string {
	return tmux.GetCurrentSessionName()
}

// tmuxSafeName converts a session name to a tmux-safe window name
func tmuxSafeName(name string) string {
	return tmux.SafeName(name)
}
