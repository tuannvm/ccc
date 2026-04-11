package tmux

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// SessionExists checks if the main "ccc" tmux session exists without creating it
// Returns true if session exists, false otherwise
func SessionExists() bool {
	cmd := exec.Command(TmuxPath, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if scanner.Text() == SessionName {
			return true
		}
	}
	// Check for scanner errors (though unlikely with tmux output)
	if err := scanner.Err(); err != nil {
		return false
	}
	return false
}

// EnsureSession ensures the main "ccc" tmux session exists
// Returns the session name "ccc"
func EnsureSession() (string, error) {
	// Check if the ccc session exists
	cmd := exec.Command(TmuxPath, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// No sessions at all, create ccc session
		c := exec.Command(TmuxPath, "new-session", "-d", "-s", SessionName)
		if err := c.Run(); err != nil {
			return "", err
		}
		exec.Command(TmuxPath, "set-option", "-t", SessionName, "mouse", "on").Run()
		return SessionName, nil
	}

	// Check if ccc session exists
	hasCccSession := false
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if scanner.Text() == SessionName {
			hasCccSession = true
			break
		}
	}

	// Create ccc session if it doesn't exist
	if !hasCccSession {
		c := exec.Command(TmuxPath, "new-session", "-d", "-s", SessionName)
		if err := c.Run(); err != nil {
			return "", err
		}
		exec.Command(TmuxPath, "set-option", "-t", SessionName, "mouse", "on").Run()
	}

	return SessionName, nil
}

// EnsureProjectWindow ensures a window exists for the project within the ccc session
// Returns the target string "ccc:TommyClaw" (session:window format)
func EnsureProjectWindow(sessionName string) (string, error) {
	sess, err := EnsureSession()
	if err != nil {
		return "", err
	}

	// Make the session name tmux-safe (dots are interpreted as separators)
	windowName := SafeName(sessionName)

	// Check if a window with this name already exists in the ccc session
	cmd := exec.Command(TmuxPath, "list-windows", "-t", sess, "-F", "#{window_name}")
	out, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			if scanner.Text() == windowName {
				// Window exists, return its target
				return sess + ":" + windowName, nil
			}
		}
	}

	// Window doesn't exist, create it
	// Note: We use sess + ":" to target the session for window creation
	// Without the colon, tmux interprets it as a window target which can cause conflicts
	cmd = exec.Command(TmuxPath, "new-window", "-t", sess+":", "-n", windowName)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create window: %w", err)
	}

	return sess + ":" + windowName, nil
}

// FindExistingWindow finds an existing window without creating it
// Returns the target string "ccc:TommyClaw" if window exists, empty string otherwise
func FindExistingWindow(sessionName string) (string, error) {
	sess, err := EnsureSession()
	if err != nil {
		return "", err
	}

	// Make the session name tmux-safe (dots are interpreted as separators)
	windowName := SafeName(sessionName)

	// Check if a window with this name already exists in the ccc session
	cmd := exec.Command(TmuxPath, "list-windows", "-t", sess, "-F", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if scanner.Text() == windowName {
			// Window exists, return its target
			return sess + ":" + windowName, nil
		}
	}

	// Window doesn't exist, return empty (no error)
	return "", nil
}

// GetWindowTarget returns the target for a project's window in the ccc session
// Takes a session name (e.g., "TommyClaw") and returns "ccc:TommyClaw"
func GetWindowTarget(sessionName string) (string, error) {
	return EnsureProjectWindow(sessionName)
}

// GetCurrentSessionName returns the session name currently displayed in the ccc session
// Returns empty string if unable to determine
func GetCurrentSessionName() string {
	sess, err := EnsureSession()
	if err != nil {
		return ""
	}

	// Get the current window name in the ccc session
	cmd := exec.Command(TmuxPath, "display-message", "-t", sess, "-p", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Window name is the project name (e.g., "TommyClaw", "Ghostty")
	// We no longer add provider prefixes to avoid lookup issues
	return strings.TrimSpace(string(out))
}

// TargetByID returns the window ID if available, otherwise falls back to name lookup
func TargetByID(windowID string, windowName string) string {
	if windowID != "" {
		return windowID
	}
	return TargetByName(windowName)
}

// TargetByName finds a window target by name (fallback)
func TargetByName(windowName string) string {
	cmd := exec.Command(TmuxPath, "list-windows", "-a", "-F", "#{window_id}\t#{window_name}")
	out, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), "\t", 2)
			if len(parts) == 2 && parts[1] == windowName {
				return parts[0] // return window ID
			}
		}
	}
	return SessionName + ":" + windowName
}

// SafeName makes a session name safe for tmux (dots become separators)
func SafeName(name string) string {
	return strings.ReplaceAll(name, ".", "__")
}
