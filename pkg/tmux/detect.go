package tmux

import (
	"os"
	"os/exec"
)

const (
	// SessionName is the main tmux session for all ccc work
	SessionName = "ccc"
	// WindowPrefix is the prefix for ccc windows (we use session/project name as window name)
	WindowPrefix = ""
)

var (
	// TmuxPath is the path to the tmux binary
	TmuxPath string
	// CCCPath is the path to the ccc binary
	CCCPath string
	// ClaudePath is the path to the claude binary
	ClaudePath string
	// CodexPath is the path to the codex binary
	CodexPath string
)

// InitPaths initializes the paths to tmux, ccc, and claude binaries
func InitPaths() {
	// Find tmux binary
	if path, err := exec.LookPath("tmux"); err == nil {
		TmuxPath = path
	} else {
		// Fallback paths for common installations
		for _, p := range []string{"/opt/homebrew/bin/tmux", "/usr/local/bin/tmux", "/usr/bin/tmux"} {
			if _, err := os.Stat(p); err == nil {
				TmuxPath = p
				break
			}
		}
	}

	// Find ccc binary - prefer ~/bin/ccc (canonical install path),
	// then PATH, then current executable as last resort
	home, _ := os.UserHomeDir()
	binCcc := home + "/bin/ccc"
	if _, err := os.Stat(binCcc); err == nil {
		CCCPath = binCcc
	} else if path, err := exec.LookPath("ccc"); err == nil {
		CCCPath = path
	} else if exe, err := os.Executable(); err == nil {
		CCCPath = exe
	}

	// Find claude binary - first try PATH, then fallback paths
	if path, err := exec.LookPath("claude"); err == nil {
		ClaudePath = path
	} else {
		home, _ := os.UserHomeDir()
		claudePaths := []string{
			home + "/.local/bin/claude",
			"/usr/local/bin/claude",
		}
		for _, p := range claudePaths {
			if _, err := os.Stat(p); err == nil {
				ClaudePath = p
				break
			}
		}
	}

	// Find codex binary - first try PATH, then fallback paths
	if path, err := exec.LookPath("codex"); err == nil {
		CodexPath = path
	} else {
		home, _ := os.UserHomeDir()
		codexPaths := []string{
			home + "/.npm-global/bin/codex",
			home + "/.local/bin/codex",
			"/opt/homebrew/bin/codex",
			"/usr/local/bin/codex",
		}
		for _, p := range codexPaths {
			if _, err := os.Stat(p); err == nil {
				CodexPath = p
				break
			}
		}
	}
}
