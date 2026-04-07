package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// cccSessionExists checks if the main "ccc" tmux session exists without creating it
// Returns true if session exists, false otherwise
func cccSessionExists() bool {
	cmd := exec.Command(tmuxPath, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if scanner.Text() == cccSessionName {
			return true
		}
	}
	// Check for scanner errors (though unlikely with tmux output)
	if err := scanner.Err(); err != nil {
		return false
	}
	return false
}

// ensureCccSession ensures the main "ccc" tmux session exists
// Returns the session name "ccc"
func ensureCccSession() (string, error) {
	// Check if the ccc session exists
	cmd := exec.Command(tmuxPath, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// No sessions at all, create ccc session
		c := exec.Command(tmuxPath, "new-session", "-d", "-s", cccSessionName)
		if err := c.Run(); err != nil {
			return "", err
		}
		exec.Command(tmuxPath, "set-option", "-t", cccSessionName, "mouse", "on").Run()
		return cccSessionName, nil
	}

	// Check if ccc session exists
	hasCccSession := false
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if scanner.Text() == cccSessionName {
			hasCccSession = true
			break
		}
	}

	// Create ccc session if it doesn't exist
	if !hasCccSession {
		c := exec.Command(tmuxPath, "new-session", "-d", "-s", cccSessionName)
		if err := c.Run(); err != nil {
			return "", err
		}
		exec.Command(tmuxPath, "set-option", "-t", cccSessionName, "mouse", "on").Run()
	}

	return cccSessionName, nil
}

// ensureProjectWindow ensures a window exists for the project within the ccc session
// Returns the target string "ccc:TommyClaw" (session:window format)
func ensureProjectWindow(sessionName string) (string, error) {
	sess, err := ensureCccSession()
	if err != nil {
		return "", err
	}

	// Make the session name tmux-safe (dots are interpreted as separators)
	windowName := tmuxSafeName(sessionName)

	// Check if a window with this name already exists in the ccc session
	cmd := exec.Command(tmuxPath, "list-windows", "-t", sess, "-F", "#{window_name}")
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
	cmd = exec.Command(tmuxPath, "new-window", "-t", sess + ":", "-n", windowName)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create window: %w", err)
	}

	return sess + ":" + windowName, nil
}

// findExistingWindow finds an existing window without creating it
// Returns the target string "ccc:TommyClaw" if window exists, empty string otherwise
func findExistingWindow(sessionName string) (string, error) {
	sess, err := ensureCccSession()
	if err != nil {
		return "", err
	}

	// Make the session name tmux-safe (dots are interpreted as separators)
	windowName := tmuxSafeName(sessionName)

	// Check if a window with this name already exists in the ccc session
	cmd := exec.Command(tmuxPath, "list-windows", "-t", sess, "-F", "#{window_name}")
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

// getCccWindowTarget returns the target for a project's window in the ccc session
// Takes a session name (e.g., "TommyClaw") and returns "ccc:TommyClaw"
func getCccWindowTarget(sessionName string) (string, error) {
	return ensureProjectWindow(sessionName)
}

// getCurrentSessionName returns the session name currently displayed in the ccc session
// Returns empty string if unable to determine
func getCurrentSessionName() string {
	sess, err := ensureCccSession()
	if err != nil {
		return ""
	}

	// Get the current window name in the ccc session
	cmd := exec.Command(tmuxPath, "display-message", "-t", sess, "-p", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Window name is the project name (e.g., "TommyClaw", "Ghostty")
	// We no longer add provider prefixes to avoid lookup issues
	return strings.TrimSpace(string(out))
}
