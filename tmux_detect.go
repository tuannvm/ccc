package main

import (
	"github.com/tuannvm/ccc/pkg/tmux"
)

const (
	// cccSessionName is the main tmux session for all ccc work
	cccSessionName = "ccc"
	// cccWindowPrefix is the prefix for ccc windows (we use session/project name as window name)
	cccWindowPrefix = ""
)

var (
	// Wrapper variables for backward compatibility with other root files
	// These are kept in sync with pkg/tmux variables
	tmuxPath   string
	cccPath    string
	claudePath string
)

func initPaths() {
	tmux.InitPaths()
	// Sync local wrappers with package variables
	tmuxPath = tmux.TmuxPath
	cccPath = tmux.CCCPath
	claudePath = tmux.ClaudePath
}

// tmuxTargetByID returns the window ID if available, otherwise falls back to name lookup
func tmuxTargetByID(windowID string, windowName string) string {
	return tmux.TargetByID(windowID, windowName)
}

// tmuxTargetByName finds a window target by name (fallback)
func tmuxTargetByName(windowName string) string {
	return tmux.TargetByName(windowName)
}
