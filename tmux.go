package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// cccSessionName is the main tmux session for all ccc work
	cccSessionName = "ccc"
	// cccWindowPrefix is the prefix for ccc windows (we use session/project name as window name)
	cccWindowPrefix = ""
)

var (
	tmuxPath   string
	cccPath    string
	claudePath string
)

func initPaths() {
	// Find tmux binary
	if path, err := exec.LookPath("tmux"); err == nil {
		tmuxPath = path
	} else {
		// Fallback paths for common installations
		for _, p := range []string{"/opt/homebrew/bin/tmux", "/usr/local/bin/tmux", "/usr/bin/tmux"} {
			if _, err := os.Stat(p); err == nil {
				tmuxPath = p
				break
			}
		}
	}

	// Find ccc binary - prefer ~/bin/ccc (canonical install path),
	// then PATH, then current executable as last resort
	home, _ := os.UserHomeDir()
	binCcc := home + "/bin/ccc"
	if _, err := os.Stat(binCcc); err == nil {
		cccPath = binCcc
	} else if path, err := exec.LookPath("ccc"); err == nil {
		cccPath = path
	} else if exe, err := os.Executable(); err == nil {
		cccPath = exe
	}

	// Find claude binary - first try PATH, then fallback paths
	if path, err := exec.LookPath("claude"); err == nil {
		claudePath = path
	} else {
		home, _ := os.UserHomeDir()
		claudePaths := []string{
			home + "/.local/bin/claude",
			"/usr/local/bin/claude",
		}
		for _, p := range claudePaths {
			if _, err := os.Stat(p); err == nil {
				claudePath = p
				break
			}
		}
	}
}

// getTargetSession returns an existing tmux session name, or creates one if none exist
func getTargetSession() (string, error) {
	// Try to find any existing session
	cmd := exec.Command(tmuxPath, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			name := scanner.Text()
			if name != "" {
				return name, nil
			}
		}
	}
	// No sessions exist, create one
	c := exec.Command(tmuxPath, "new-session", "-d", "-s", cccSessionName)
	if err := c.Run(); err != nil {
		return "", err
	}
	exec.Command(tmuxPath, "set-option", "-t", cccSessionName, "mouse", "on").Run()
	return cccSessionName, nil
}

// tmuxTargetByID returns the window ID if available, otherwise falls back to name lookup
func tmuxTargetByID(windowID string, windowName string) string {
	if windowID != "" {
		return windowID
	}
	return tmuxTargetByName(windowName)
}

// tmuxTargetByName finds a window target by name (fallback)
func tmuxTargetByName(windowName string) string {
	cmd := exec.Command(tmuxPath, "list-windows", "-a", "-F", "#{window_id}\t#{window_name}")
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
	return cccSessionName + ":" + windowName
}

// tmuxWindowHasClaudeRunning checks if the tmux window has a functional Claude/ccc process running
// Returns false if window doesn't exist or only has a shell (zsh/bash) without Claude
// Handles npm-installed Claude which shows as 'node' or 'nodejs' process
// Checks only the ACTIVE pane to avoid false positives in split-pane windows
func tmuxWindowHasClaudeRunning(windowID string, windowName string) bool {
	// First find the window
	target := tmuxTargetByID(windowID, windowName)
	if target == "" {
		listenLog("tmuxWindowHasClaudeRunning: no target found for windowID=%s name=%s", windowID, windowName)
		return false
	}
	return tmuxTargetHasClaudeRunning(target)
}

// tmuxTargetHasClaudeRunning checks if a tmux target (pane or window) has Claude running
// This is the shared implementation used by both tmuxWindowHasClaudeRunning and currentPaneHasClaudeRunning
// Uses hybrid detection: process name check + process tree check + prompt character check
func tmuxTargetHasClaudeRunning(target string) bool {

	// Get pane IDs, active flag, and commands together to check only the active pane
	cmd := exec.Command(tmuxPath, "list-panes", "-t", target, "-F", "#{pane_active}\t#{pane_id}\t#{pane_current_command}")
	out, err := cmd.Output()
	if err != nil {
		listenLog("tmuxTargetHasClaudeRunning: list-panes failed for target=%s: %v", target, err)
		return false
	}

	// Common shell names
	shells := map[string]bool{
		"sh": true, "bash": true, "zsh": true, "fish": true,
		"dash": true, "nu": true, "elvish": true, "xonsh": true,
		"tcsh": true, "csh": true, "ksh": true,
	}

	// Check only the ACTIVE pane's command
	panesOutput := strings.TrimSpace(string(out))
	lines := strings.Split(panesOutput, "\n")
	var activePaneID, activePaneCmd string
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		isActive, paneID, paneCmd := parts[0], parts[1], parts[2]
		if isActive != "1" {
			continue // Skip non-active panes
		}
		activePaneID = paneID
		activePaneCmd = strings.TrimSpace(paneCmd)

		// Process-based detection: check if this pane has claude running
		if strings.HasPrefix(paneCmd, "claude") {
			listenLog("tmuxTargetHasClaudeRunning: Claude IS running (cmd=%s) in active pane=%s target=%s", paneCmd, paneID, target)
			return true
		}
		// Check for npm-installed Claude (shows as 'node' or 'nodejs')
		if paneCmd == "node" || paneCmd == "nodejs" {
			// Verify it's actually Claude by examining the process command line
			if tmuxPaneIsClaudeProcess(paneID) {
				listenLog("tmuxTargetHasClaudeRunning: Claude (npm) IS running (cmd=%s) in active pane=%s target=%s", paneCmd, paneID, target)
				return true
			}
			listenLog("tmuxTargetHasClaudeRunning: node found but not Claude process in pane=%s target=%s", paneID, target)
		}
		// Special case: "ccc" process means the wrapper is running
		// This could mean Claude is starting OR it's already running
		// We need prompt-based detection to tell the difference
		if paneCmd == "ccc" || paneCmd == "ccc run" {
			listenLog("tmuxTargetHasClaudeRunning: ccc process detected, checking active prompt in pane=%s target=%s", paneID, target)
			// Check if Claude prompt is present at the END of the buffer - means Claude is actually running
			if tmuxPaneHasActiveClaudePrompt(paneID) {
				listenLog("tmuxTargetHasClaudeRunning: Claude IS running (ccc+active prompt detected) in active pane=%s target=%s", paneID, target)
				return true
			}
			// ccc without active prompt means it's still starting up
			listenLog("tmuxTargetHasClaudeRunning: ccc running but no active Claude prompt yet in pane=%s target=%s", paneID, target)
		}
		// Special case: shell process (zsh/bash) - check if it has Claude as a child process
		// This handles cases where pane_current_command shows the shell but Claude is running under it
		if shells[paneCmd] {
			listenLog("tmuxTargetHasClaudeRunning: shell detected (cmd=%s) in pane=%s target=%s, checking for Claude children", paneCmd, paneID, target)
			// Check if this shell has Claude/node children
			if tmuxPaneHasClaudeChild(paneID) {
				listenLog("tmuxTargetHasClaudeRunning: Claude IS running as child of shell in pane=%s target=%s", paneID, target)
				return true
			}
		}
	}

	// Process-based detection failed, try prompt-based detection as final fallback
	// We use strict detection here to avoid false positives from shell prompts
	// Only accept the prompt if we have Claude-specific context (via tmuxPaneHasActiveClaudePrompt)
	if activePaneID != "" && tmuxPaneHasActiveClaudePrompt(activePaneID) {
		listenLog("tmuxTargetHasClaudeRunning: Claude IS running (prompt fallback detected) in active pane=%s target=%s", activePaneID, target)
		return true
	}

	// If we reach here, the active pane doesn't have Claude running
	listenLog("tmuxTargetHasClaudeRunning: Claude NOT running in active pane (cmd=%s) in target=%s - will auto-restart", activePaneCmd, target)
	return false
}

// tmuxPaneHasClaudePrompt checks if the tmux pane contains Claude's prompt character (❯)
// This is used to verify that a node/nodejs process is actually Claude Code
// The target can be a pane ID (%0) or window:pane format (session:window.pane)
// NOTE: This only checks for the prompt anywhere in the buffer. For detecting ACTIVE
// sessions, use tmuxPaneHasActiveClaudePrompt() instead to avoid false positives.
func tmuxPaneHasClaudePrompt(paneTarget string) bool {
	// Capture the pane buffer and check for Claude's prompt
	// Use -e for escape sequences and -J to join wrapped lines
	// Do NOT use -C as it escapes non-ASCII bytes, breaking Unicode prompt detection
	cmd := exec.Command(tmuxPath, "capture-pane", "-t", paneTarget, "-p", "-e", "-J")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	content := string(out)
	// Claude Code shows "❯" when ready for input
	// Also check for "How can I help?" which appears in the welcome message
	return strings.Contains(content, "❯") || strings.Contains(content, "How can I help")
}

// tmuxPaneHasActiveClaudePrompt checks if the tmux pane has Claude's prompt at the END of the buffer
// This indicates Claude is currently active and waiting for input (not just historical content)
// The target can be a pane ID (%0) or window:pane format (session:window.pane)
// Uses strict detection with context requirement to avoid false positives from shell prompts
func tmuxPaneHasActiveClaudePrompt(paneTarget string) bool {
	// Capture the last few lines of the pane buffer to check for active prompt
	// Use -e for escape sequences and -J to join wrapped lines
	// -S -15 captures last 15 lines (enough to see prompt + recent context)
	cmd := exec.Command(tmuxPath, "capture-pane", "-t", paneTarget, "-p", "-e", "-J", "-S", "-15")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	content := string(out)
	// Check if the last non-empty line contains Claude's prompt
	lines := strings.Split(strings.TrimSpace(content), "\n")

	// Find the last non-empty line
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			// Check for Claude's prompt character (❯)
			if strings.Contains(line, "❯") {
				// To avoid false positives from shell prompts (e.g., Powerlevel10k),
				// we require Claude-specific context in the recent buffer
				// Claude-specific indicators:
				// - "How can I help?" in welcome message
				// - "Claude" or "Anthropic" in output
				// - Tool use blocks (e.g., "function:", "result:", "Bash:")
				hasClaudeContext := false
				lowerContent := strings.ToLower(content)
				if strings.Contains(content, "How can I help") ||
					strings.Contains(lowerContent, "claude") ||
					strings.Contains(lowerContent, "anthropic") ||
					strings.Contains(content, "Bash:") ||
					strings.Contains(content, "function:") ||
					strings.Contains(content, "result:") {
					hasClaudeContext = true
				}

				// Only accept the prompt if we have Claude-specific context
				// This avoids false positives from shell prompts that use ❯
				if hasClaudeContext {
					listenLog("tmuxPaneHasActiveClaudePrompt: found Claude prompt with context in pane=%s: %q", paneTarget, line)
					return true
				}
				// Has ❯ but no Claude context - likely a shell prompt
				listenLog("tmuxPaneHasActiveClaudePrompt: found ❯ but no Claude context in pane=%s: %q", paneTarget, line)
			}
			// If we found a non-empty line without the prompt, Claude is not active
			break
		}
	}
	return false
}

// tmuxPaneHasClaudeChild checks if the pane's process (typically a shell) has Claude as a child process
// This is used when pane_current_command shows a shell (zsh/bash) but Claude might be running under it
// Returns true if the pane has a child process that is Claude (claude binary or node with claude/cli)
func tmuxPaneHasClaudeChild(paneID string) bool {
	// Get the pane's PID using tmux
	cmd := exec.Command(tmuxPath, "display-message", "-t", paneID, "-p", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	panePid := strings.TrimSpace(string(out))
	if panePid == "" || panePid == "0" {
		return false
	}

	// Find child processes of the pane
	// Try GNU ps syntax first (Linux with --ppid)
	psOut, err := exec.Command("ps", "-o", "pid,command", "--ppid", panePid, "--no-headers").Output()
	if err != nil {
		// GNU ps failed, try getting all processes and filter in Go (works cross-platform)
		allPsOut, psErr := exec.Command("ps", "-ax", "-o", "pid,ppid,command").Output()
		if psErr != nil {
			return false
		}
		psOut = filterChildProcesses(allPsOut, panePid)
	}

	if len(psOut) == 0 {
		// No children found
		return false
	}

	// Check if any child is Claude
	lines := strings.Split(string(psOut), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Split on first whitespace to get PID and command
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		cmdline := strings.TrimSpace(parts[1])

		// Check for claude binary
		if strings.Contains(cmdline, "claude") && !strings.Contains(cmdline, "ccc") {
			listenLog("tmuxPaneHasClaudeChild: found claude child in pane=%s: %q", paneID, cmdline)
			return true
		}
		// Check for node process with claude/cli
		if (strings.HasPrefix(cmdline, "node ") || strings.HasPrefix(cmdline, "nodejs ")) &&
			(strings.Contains(cmdline, "/claude") || strings.Contains(cmdline, "/@anthropic-ai/")) {
			listenLog("tmuxPaneHasClaudeChild: found node/claude child in pane=%s: %q", paneID, cmdline)
			return true
		}
	}

	return false
}

// tmuxPaneIsClaudeProcess checks if the pane's foreground process is actually the Claude CLI
// This finds the foreground node process (child of shell) and examines its command line
// Returns true if the pane is running a node process with claude/cli in its command line
func tmuxPaneIsClaudeProcess(paneID string) bool {
	// Get the pane's PID (shell PID) using tmux
	cmd := exec.Command(tmuxPath, "display-message", "-t", paneID, "-p", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	shellPid := strings.TrimSpace(string(out))
	if shellPid == "" || shellPid == "0" {
		return false
	}

	// Find child processes of the shell that are running node
	// Try GNU ps syntax first (Linux with --ppid)
	psOut, err := exec.Command("ps", "-o", "pid,command", "--ppid", shellPid, "--no-headers").Output()
	if err != nil {
		// GNU ps failed, try getting all processes and filter in Go (works cross-platform)
		// Use -ax on BSD/macOS to get all processes
		allPsOut, psErr := exec.Command("ps", "-ax", "-o", "pid,ppid,command").Output()
		if psErr != nil {
			// ps completely failed, fall back to prompt check
			listenLog("tmuxPaneIsClaudeProcess: ps failed for shellPid=%s, falling back to prompt check", shellPid)
			return tmuxPaneHasClaudePrompt(paneID)
		}

		// Parse all processes and find children of shell
		psOut = filterChildProcesses(allPsOut, shellPid)
	}

	// Parse ps output to find node processes
	// Format: "PID command" (no header due to --no-headers or filtering)
	lines := strings.Split(strings.TrimSpace(string(psOut)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split on first whitespace to get PID and command
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		pid := parts[0]
		cmdline := strings.TrimSpace(parts[1])

		// Check if this is a node process running Claude
		// Match against various node invocation styles:
		// - "node /path/to/claude" (npm global)
		// - "nodejs /path/to/claude" (some systems)
		// - "/usr/bin/node /path/to/claude" (full path)
		// - "node /path/to/@anthropic-ai/cli/..." (npm package)
		//
		// IMPORTANT: Be specific to avoid false positives from unrelated processes
		// that happen to contain "claude" in their path/name. Use path separators
		// to ensure we're matching actual Claude CLI entrypoints.
		isNode := strings.HasPrefix(cmdline, "node ") ||
			strings.HasPrefix(cmdline, "nodejs ") ||
			strings.Contains(cmdline, "/node ") ||
			strings.Contains(cmdline, "/nodejs ")

		// Check for Claude CLI specific patterns:
		// 1. "/claude" or "/claude.js" as a path component (not just substring)
		// 2. "@anthropic-ai/" npm package namespace
		// 3. Known Claude entrypoint patterns
		isClaude := isNode && (
			strings.Contains(cmdline, "/claude ") ||           // "node .../claude" (global bin)
			strings.Contains(cmdline, "/claude.js ") ||        // direct script
			strings.Contains(cmdline, "/@anthropic-ai/") ||    // npm package
			strings.HasSuffix(cmdline, "/claude") ||           // ends with /claude
			strings.HasSuffix(cmdline, "/claude.js"))          // ends with /claude.js

		if isClaude {
			listenLog("tmuxPaneIsClaudeProcess: paneID=%s shellPid=%s nodePid=%s cmdline=%q isClaude=true", paneID, shellPid, pid, cmdline)
			return true
		}
	}

	// No node process found as child of shell, or not Claude
	listenLog("tmuxPaneIsClaudeProcess: paneID=%s shellPid=%s no Claude node process found", paneID, shellPid)
	return false
}

// filterChildProcesses parses ps output and returns lines where PPID matches parentPid
// ps output format: "PID PPID COMMAND" (may have header line)
func filterChildProcesses(psOutput []byte, parentPid string) []byte {
	lines := strings.Split(string(psOutput), "\n")
	var result []string

	shellPidInt, err := strconv.Atoi(parentPid)
	if err != nil {
		// If we can't parse the PID, return empty
		return []byte{}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and header (lines starting with "PID" or "  PID")
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}

		// Split into fields: PID PPID COMMAND...
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Check if PPID (second field) matches parentPid
		if len(fields[1]) > 0 {
			if ppid, err := strconv.Atoi(fields[1]); err == nil && ppid == shellPidInt {
				// This is a child process, return "PID COMMAND" format
				result = append(result, fields[0]+" "+strings.Join(fields[2:], " "))
			}
		}
	}

	return []byte(strings.Join(result, "\n"))
}

// tmuxWindowHasShellRunning checks if the target tmux window has a shell running
// Returns true if the ACTIVE pane has a shell, which means the window is ready for input
// This is scoped to the active pane to avoid misrouting commands in split-pane windows
// Supports common shells: zsh, bash, sh, fish, dash, nu, elvish, xonsh, tcsh, csh, ksh
func tmuxWindowHasShellRunning(windowID string, windowName string) bool {
	target := tmuxTargetByID(windowID, windowName)
	if target == "" {
		return false
	}

	// Get only the ACTIVE pane's current command
	// Using -t target without -F format gets us the active pane by default
	cmd := exec.Command(tmuxPath, "list-panes", "-t", target, "-F", "#{pane_active}\t#{pane_current_command}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	// Common shell names to recognize
	shells := map[string]bool{
		"sh": true, "bash": true, "zsh": true, "fish": true,
		"dash": true, "nu": true, "elvish": true, "xonsh": true,
		"tcsh": true, "csh": true, "ksh": true,
	}

	// Check only the active pane's command
	panesOutput := strings.TrimSpace(string(out))
	lines := strings.Split(panesOutput, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		isActive, paneCmd := parts[0], parts[1]
		if isActive == "1" {
			paneCmd = strings.TrimSpace(paneCmd)
			// Check if the active pane has a recognized shell running
			if shells[paneCmd] {
				return true
			}
			return false
		}
	}
	return false
}

func tmuxWindowExistsByID(windowID string, windowName string) bool {
	if windowID != "" {
		// Check by ID directly
		cmd := exec.Command(tmuxPath, "list-windows", "-a", "-F", "#{window_id}")
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			if scanner.Text() == windowID {
				return true
			}
		}
		return false
	}
	// Fallback: search by name
	cmd := exec.Command(tmuxPath, "list-windows", "-a", "-F", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if scanner.Text() == windowName {
			return true
		}
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

// switchSessionInWindow switches the context to the project's window in the ccc session
// Each project gets its own named window within the main "ccc" session
// If skipRestart is true and the requested session is already active, it will skip restarting
func switchSessionInWindow(sessionName string, workDir string, providerName string, sessionID string, worktreeName string, continueSession bool, skipRestart bool) error {
	// Ensure the project window exists in the ccc session (e.g., "ccc:TommyClaw")
	target, err := ensureProjectWindow(sessionName)
	if err != nil {
		return err
	}

	// Check if we should skip restarting
	// Only skip if: 1) skipRestart is true, AND 2) the target window already has Claude/shell running
	shouldRestart := true
	if skipRestart {
		// Check if the target window already has Claude or a shell running
		// A shell means the window is ready for input and we can send commands directly
		if tmuxWindowHasClaudeRunning(target, "") || tmuxWindowHasShellRunning(target, "") {
			// Target window already has Claude or shell running - skip respawn
			// We can send commands directly without restarting
			shouldRestart = false
		} else {
			// When skipRestart=true but we don't detect Claude or shell, be extra cautious
			// This handles false negatives in detection where Claude is actually running
			// Check for Claude prompt in the pane content as a fallback
			if tmuxPaneHasActiveClaudePrompt(target) {
				listenLog("skipRestart=true: Claude prompt detected in pane content (fallback detection)")
				shouldRestart = false
			}
		}
	}

	// Build the ccc run command with all flags
	// Use ccc run instead of claude directly to ensure provider env setup
	runCmd := cccPath + " run"

	// Determine if we should continue an existing session
	// Only add -c flag if Claude is actually running OR we have a specific session ID
	// This prevents "No conversation found to continue" errors on new sessions
	if sessionID != "" {
		// Explicit session ID to resume - use --resume flag
		runCmd += " --resume " + shellQuote(sessionID)
	} else if continueSession {
		// Check if Claude is actually running before adding -c flag
		if tmuxWindowHasClaudeRunning(target, "") {
			runCmd += " -c"
			listenLog("Claude is running, will continue existing session")
		} else {
			listenLog("continueSession=true but Claude not running, will start new session instead")
		}
	}
	// If no sessionID and not continueSession (or Claude not running), start fresh (no flags)

	// Always pass provider flag if specified
	// This ensures provider-agnostic behavior - no special case for "anthropic"
	if providerName != "" {
		runCmd += " --provider " + shellQuote(providerName)
	}
	if worktreeName != "" {
		runCmd += " --worktree " + shellQuote(worktreeName)
	}

	// Send commands to switch session context
	if shouldRestart {
		// Strategy: Always use respawn-pane for clean pane restart when shouldRestart is true
		// This ensures we have a clean shell regardless of what's currently running
		// (Claude, vim, less, or any other foreground process)
		// respawn-pane kills the process and restarts the shell atomically

		listenLog("Respawning pane for clean session restart")
		if err := exec.Command(tmuxPath, "respawn-pane", "-t", target, "-k").Run(); err != nil {
			return fmt.Errorf("failed to respawn pane: %w", err)
		}

		// Poll for pane restart completion with bounded timeout
		// Shell startup can take longer on slower systems or under load
		deadline := time.Now().Add(5 * time.Second)
		respawnComplete := false
		for time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
			if !tmuxWindowHasClaudeRunning(target, "") {
				respawnComplete = true
				listenLog("Pane respawn complete, shell is ready")
				break
			}
		}

		if !respawnComplete && tmuxWindowHasClaudeRunning(target, "") {
			return fmt.Errorf("pane respawn timed out - still shows Claude running after 5 seconds")
		}

		// Verify we have a shell running now
		if tmuxWindowHasClaudeRunning(target, "") {
			return fmt.Errorf("pane still shows Claude running after respawn - cannot proceed safely")
		}

		// Change to work directory and start claude via ccc run (as one command)
		fullCmd := "cd " + shellQuote(workDir) + " && " + runCmd
		if err := exec.Command(tmuxPath, "send-keys", "-t", target, fullCmd, "C-m").Run(); err != nil {
			return fmt.Errorf("failed to send command: %w", err)
		}
	} else {
		// Not restarting - check what's running in the target window
		if tmuxWindowHasClaudeRunning(target, "") {
			// Claude is already running in this window - don't send any command
			// The user can continue their existing session
			listenLog("Claude already running in target window, skipping command send")
		} else if tmuxWindowHasShellRunning(target, "") {
			// Shell is running - decide whether to start Claude
			// When skipRestart=true, the caller indicates the session should already be usable
			// This means Claude might be running but not properly detected (false negative)
			// In this case, we should NOT send a restart command to avoid disrupting the session
			if skipRestart {
				listenLog("Shell detected with skipRestart=true - not sending restart command to preserve session state")
			} else {
				// Shell is running but no Claude - start Claude
				fullCmd := "cd " + shellQuote(workDir) + " && " + runCmd
				if err := exec.Command(tmuxPath, "send-keys", "-t", target, fullCmd, "C-m").Run(); err != nil {
					return fmt.Errorf("failed to send command: %w", err)
				}
			}
		} else {
			// Pane is empty or has unknown process
			// When skipRestart=true, be conservative and don't respawn
			// The session might be in a transient state (Claude starting, tool running)
			if skipRestart {
				listenLog("Pane has unknown state but skipRestart=true - not respawning to preserve session state")
			} else {
				// Respawn to get clean state
				listenLog("Pane has unknown state, respawning for clean start")
				if err := exec.Command(tmuxPath, "respawn-pane", "-t", target, "-k").Run(); err != nil {
					return fmt.Errorf("failed to respawn pane: %w", err)
				}

				// Wait for respawn and send command
				time.Sleep(500 * time.Millisecond)
				fullCmd := "cd " + shellQuote(workDir) + " && " + runCmd
				if err := exec.Command(tmuxPath, "send-keys", "-t", target, fullCmd, "C-m").Run(); err != nil {
					return fmt.Errorf("failed to send command: %w", err)
				}
			}
		}
	}

	// Select the window to make it active (this is important when switching between projects)
	// Only attempt selection if there might be an attached client - ignore errors in headless mode
	exec.Command(tmuxPath, "select-window", "-t", target).Run()
	// We ignore the error from select-window because:
	// 1. In headless/non-interactive mode, there's no client to switch
	// 2. The window is still created and commands are sent successfully
	// 3. When the user later attaches, they'll see the correct window

	// Set window title for display purposes, but keep the base name stable
	// We use the 'window-status-format' to show provider info without renaming the window
	// This ensures ensureProjectWindow can always find the window by its original name
	if providerName != "" && len(providerName) > 0 {
		// Store provider info in a user option for display purposes
		prefix := strings.ToUpper(string(providerName[0]))
		exec.Command(tmuxPath, "set-window-option", "-t", target, "@ccc-provider", prefix).Run()
	}

	return nil
}

// shellQuote safely quotes a string for shell command arguments
func shellQuote(s string) string {
	// Replace single quotes with '\'' and wrap in single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func createTmuxWindow(windowName string, workDir string, continueSession bool, providerName string, sessionID string, worktreeName string) (string, error) {
	// Build the command to run inside the window
	cccCmd := cccPath + " run"
	if sessionID != "" {
		// Resume specific session by ID
		cccCmd += " --resume " + shellQuote(sessionID)
	} else if continueSession {
		cccCmd += " -c"
	}
	if providerName != "" {
		cccCmd += " --provider " + shellQuote(providerName)
	}
	if worktreeName != "" {
		cccCmd += " --worktree " + shellQuote(worktreeName)
	}

	// Get an existing session or create one
	sess, err := getTargetSession()
	if err != nil {
		return "", err
	}

	// Create new window, -P -F prints the window ID
	args := []string{"new-window", "-P", "-F", "#{window_id}", "-t", sess + ":", "-n", windowName, "-c", workDir}
	cmd := exec.Command(tmuxPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	windowID := strings.TrimSpace(string(out))

	// Send the command to the window via send-keys using window ID
	time.Sleep(200 * time.Millisecond)
	exec.Command(tmuxPath, "send-keys", "-t", windowID, cccCmd, "C-m").Run()

	return windowID, nil
}

// applyProviderEnv applies provider-specific environment variables to cmd.Env
// Returns the modified environment slice
// This version uses the Provider interface for provider-agnostic design
func applyProviderEnv(baseEnv []string, provider Provider, config *Config) []string {
	if provider == nil {
		return baseEnv
	}

	env := baseEnv

	// Get provider variables and track which ones we'll actually set
	providerVars := provider.EnvVars(config)

	// For ConfiguredProvider with auth, we need to check if auth_env_var expands to non-empty
	// If it expands to empty, we should preserve ambient credentials instead
	shouldUnsetAuth := false
	if !provider.IsBuiltin() {
		for _, v := range providerVars {
			if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=$") {
				// This is auth_env_var - check if it expands to non-empty
				envVarName := strings.TrimPrefix(v, "ANTHROPIC_AUTH_TOKEN=$")
				if envVal := os.Getenv(envVarName); envVal != "" {
					shouldUnsetAuth = true
				}
				// If env var is empty, we DON'T unset existing auth - preserve ambient
				break
			} else if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=") && !strings.Contains(v, "$") {
				// Direct token value (no $ prefix)
				shouldUnsetAuth = true
				break
			}
		}
	}

	// Unset auth vars only if we're actually replacing them
	if shouldUnsetAuth {
		env = unsetEnvVars(env, []string{
			"ANTHROPIC_API_KEY",
			"CLAUDE_API_KEY",
			"ANTHROPIC_AUTH_TOKEN",
		})
		// Also unset model vars when using a configured provider with auth
		env = unsetEnvVars(env, []string{
			"ANTHROPIC_BASE_URL",
			"ANTHROPIC_MODEL",
			"ANTHROPIC_DEFAULT_OPUS_MODEL",
			"ANTHROPIC_DEFAULT_SONNET_MODEL",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL",
			"CLAUDE_CODE_SUBAGENT_MODEL",
		})
	}

	// Add provider-specific environment variables
	// For ConfiguredProvider, this includes expanded auth_env_var values and api_timeout
	for _, v := range providerVars {
		// Expand $VAR references for ConfiguredProvider auth_env_var
		if strings.HasPrefix(v, "ANTHROPIC_AUTH_TOKEN=$") {
			envVarName := strings.TrimPrefix(v, "ANTHROPIC_AUTH_TOKEN=$")
			if envVal := os.Getenv(envVarName); envVal != "" {
				env = append(env, "ANTHROPIC_AUTH_TOKEN="+envVal)
			}
			// If env var is empty, we skip it (preserving ambient credentials)
		} else {
			env = append(env, v)
		}
	}

	// Common settings for all providers
	// Note: TMPDIR is set for all providers including builtin, as before
	env = append(env, []string{
		"TMPDIR=/tmp/claude",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DISABLE_COST_WARNINGS=1",
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	}...)

	return env
}

// applyProviderEnvLegacy applies provider-specific environment variables using the old ProviderConfig struct
// This is a transitional function for backward compatibility
func applyProviderEnvLegacy(baseEnv []string, provider *ProviderConfig, config *Config) []string {
	if provider == nil {
		return baseEnv
	}

	home, _ := os.UserHomeDir()

	// Build environment with provider settings
	// First, unset any Anthropic-related vars to avoid conflicts
	env := baseEnv
	env = unsetEnvVars(env, []string{
		"ANTHROPIC_API_KEY",
		"CLAUDE_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"CLAUDE_CODE_SUBAGENT_MODEL",
	})

	// Determine auth token (auto-load from env if empty)
	authToken := provider.AuthToken
	if authToken == "" && provider.AuthEnvVar != "" {
		authToken = os.Getenv(provider.AuthEnvVar)
	}
	// If still no token, preserve existing Anthropic credentials from environment
	if authToken == "" {
		if existing := os.Getenv("ANTHROPIC_API_KEY"); existing != "" {
			authToken = existing
		} else if existing := os.Getenv("ANTHROPIC_AUTH_TOKEN"); existing != "" {
			authToken = existing
		} else if existing := os.Getenv("CLAUDE_API_KEY"); existing != "" {
			authToken = existing
		}
	}
	if authToken != "" {
		env = append(env, "ANTHROPIC_AUTH_TOKEN="+authToken)
	}

	// Base URL
	if provider.BaseURL != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+provider.BaseURL)
	}

	// Models
	if provider.OpusModel != "" {
		env = append(env, "ANTHROPIC_DEFAULT_OPUS_MODEL="+provider.OpusModel)
	}
	if provider.SonnetModel != "" {
		env = append(env, "ANTHROPIC_DEFAULT_SONNET_MODEL="+provider.SonnetModel)
		env = append(env, "ANTHROPIC_MODEL="+provider.SonnetModel)
	} else if provider.OpusModel != "" {
		// Fallback: if Sonnet is not configured but Opus is, use Opus as default
		env = append(env, "ANTHROPIC_MODEL="+provider.OpusModel)
	}
	if provider.HaikuModel != "" {
		env = append(env, "ANTHROPIC_DEFAULT_HAIKU_MODEL="+provider.HaikuModel)
	}
	if provider.SubagentModel != "" {
		env = append(env, "CLAUDE_CODE_SUBAGENT_MODEL="+provider.SubagentModel)
	}

	// Config dir with ~ expansion
	if provider.ConfigDir != "" {
		configDir := provider.ConfigDir
		if strings.HasPrefix(configDir, "~/") {
			configDir = home + configDir[1:]
		} else if configDir == "~" {
			configDir = home
		}
		env = append(env, "CLAUDE_CONFIG_DIR="+configDir)
	}

	// Common settings for all providers (use values from config)
	if provider.ApiTimeout > 0 {
		env = append(env, fmt.Sprintf("API_TIMEOUT_MS=%d", provider.ApiTimeout))
	}
	env = append(env, []string{
		"TMPDIR=/tmp/claude",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DISABLE_COST_WARNINGS=1",
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	}...)

	return env
}

// unsetEnvVars removes specified environment variables from env slice
func unsetEnvVars(env []string, keys []string) []string {
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	var result []string
	for _, e := range env {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			result = append(result, e)
			continue
		}
		key := e[:idx]
		if !keyMap[key] {
			result = append(result, e)
		}
	}
	return result
}

// runClaudeRaw runs claude directly (used inside tmux sessions)
// providerOverride, if non-empty, specifies which provider to use instead of active_provider
// resumeSessionID, if non-empty, resumes a specific session by ID
// worktreeName, if non-empty, creates/uses a git worktree session
func runClaudeRaw(continueSession bool, resumeSessionID string, providerOverride string, worktreeName string) error {
	if claudePath == "" {
		return fmt.Errorf("claude binary not found")
	}

	// Clean stale Telegram flag from previous sessions.
	// Use window_name to identify the session
	if winName, err := exec.Command(tmuxPath, "display-message", "-p", "#{window_name}").Output(); err == nil {
		name := strings.TrimSpace(string(winName))
		if name != "" {
			os.Remove(telegramActiveFlag(name))
		}
	}

	var args []string
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	} else if continueSession {
		args = append(args, "-c")
	}
	if worktreeName != "" {
		args = append(args, "--worktree", worktreeName)
	}

	// Build the claude command with all args
	// Execute claude directly to ensure provider env vars are not overridden by shell rc files
	cmd := exec.Command(claudePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start with current environment
	cmd.Env = os.Environ()

	// Log the command for debugging
	cwd, _ := os.Getwd()
	configDirCode := os.Getenv("CLAUDE_CODE_CONFIG_DIR")
	configDirZai := os.Getenv("CLAUDE_CONFIG_DIR")
	homeDir := os.Getenv("HOME")
	listenLog("runClaudeRaw: claude=%s args=%v cwd=%s config_code_dir=%q config_dir=%q home=%q", claudePath, args, cwd, configDirCode, configDirZai, homeDir)

	// Load config and apply provider settings
	config, err := loadConfig()
	if err == nil {
		// CRITICAL: Ensure hooks are installed in the current project directory
		// Hooks are essential for ccc functionality (Telegram, OTP, tool tracking)
		// We use cwd (current working directory) as the project path
		sessionName := filepath.Base(cwd)
		sessionInfo := &SessionInfo{Path: cwd}
		if config.Sessions != nil {
			if existing := config.Sessions[sessionName]; existing != nil {
				sessionInfo = existing
			}
		}
		if err := ensureHooksForSession(config, sessionName, sessionInfo); err != nil {
			listenLog("Warning: Failed to ensure hooks in %s: %v", cwd, err)
		}

		// Determine which provider to use using the Provider interface
		// getProvider returns nil only for unknown providers
		provider := getProvider(config, providerOverride)

		// Validate provider - getProvider returns nil for unknown providers
		if providerOverride != "" && provider == nil {
			return fmt.Errorf("unknown provider: %s (available providers: %v)", providerOverride, getProviderNames(config))
		}

		// Apply provider env in the following cases:
		// 1. When NOT resuming (resumeSessionID == "") - start new session with provider env
		// 2. When resuming WITH explicit provider override - user specified which provider to use
		// Skip provider env only when resuming WITHOUT explicit override (preserve original session env)
		shouldApplyProviderEnv := (resumeSessionID == "") || (providerOverride != "")

		// Ensure provider settings have trusted directories configured
		// This prevents "Do you trust the files in this folder?" prompts
		// Works with both BuiltinProvider and ConfiguredProvider
		if err := ensureProviderSettings(provider); err != nil {
			listenLog("Failed to update provider settings: %v", err)
		}

		if shouldApplyProviderEnv {
			cmd.Env = applyProviderEnv(cmd.Env, provider, config)
			listenLog("Applying provider env: providerOverride=%q resumeSessionID=%q provider=%q", providerOverride, resumeSessionID, provider.Name())
		} else {
			listenLog("Preserving original session environment for resumeSessionID=%q", resumeSessionID)
		}

		// Ensure OAuth token is available from config if not already in environment
		if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" && config.OAuthToken != "" {
			cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+config.OAuthToken)
		}
	}

	return cmd.Run()
}

// waitForClaude polls the tmux pane until Claude Code's input prompt appears
func waitForClaude(target string, timeout time.Duration) error {
	// Poll faster for short timeouts (message sending), slower for startup
	interval := 100 * time.Millisecond
	if timeout > 10*time.Second {
		interval = 500 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command(tmuxPath, "capture-pane", "-t", target, "-p")
		out, err := cmd.Output()
		if err == nil {
			content := string(out)
			// Claude Code shows "❯" when ready for input
			if strings.Contains(content, "❯") {
				return nil
			}
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("timeout waiting for Claude to start")
}

// sendToTmuxFromTelegram sets the Telegram active flag before sending,
// so the permission hook knows this input came from Telegram and requires OTP.
// windowNameFromTarget extracts the window name from a "session:window" target
func windowNameFromTarget(target string) string {
	if idx := strings.LastIndex(target, ":"); idx >= 0 {
		return target[idx+1:]
	}
	return target
}

func sendToTmuxFromTelegram(target string, windowName string, text string) error {
	os.WriteFile(telegramActiveFlag(windowName), []byte("1"), 0600)
	return sendToTmux(target, text)
}

func sendToTmuxFromTelegramWithDelay(target string, windowName string, text string, delay time.Duration) error {
	os.WriteFile(telegramActiveFlag(windowName), []byte("1"), 0600)
	return sendToTmuxWithDelay(target, text, delay)
}

func sendToTmux(target string, text string) error {
	// Calculate delay based on text length
	// Base: 50ms + 0.5ms per character, capped at 5 seconds
	baseDelay := 50 * time.Millisecond
	charDelay := time.Duration(len(text)) * 500 * time.Microsecond // 0.5ms per char
	delay := baseDelay + charDelay
	if delay > 5*time.Second {
		delay = 5 * time.Second
	}
	return sendToTmuxWithDelay(target, text, delay)
}

func sendToTmuxWithDelay(target string, text string, delay time.Duration) error {
	// Normalize line endings to LF only (handles CRLF from Telegram/Windows)
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// Handle empty or whitespace-only text - nothing to send
	if strings.TrimSpace(text) == "" {
		listenLog("sendToTmuxWithDelay: empty or whitespace-only text, skipping")
		return nil
	}

	// Check if text contains newlines - use bracketed paste mode for multi-line text
	// This prevents newlines from being interpreted as Enter key presses
	hasNewlines := strings.Contains(text, "\n")

	if hasNewlines {
		// Use bracketed paste mode for multi-line text
		// This wraps the text in escape sequences that tell the terminal
		// the content is a paste operation, so newlines should not execute
		listenLog("sendToTmuxWithDelay: using bracketed paste for multi-line text (%d chars)", len(text))

		// Calculate adaptive delay based on text length
		// More text needs more time for the terminal to process
		pasteDelay := time.Duration(len(text)/10) * time.Millisecond
		if pasteDelay < 20*time.Millisecond {
			pasteDelay = 20 * time.Millisecond
		}
		if pasteDelay > 200*time.Millisecond {
			pasteDelay = 200 * time.Millisecond
		}

		// Send bracketed paste start sequence: ESC [ 2 0 0 ~
		if err := exec.Command(tmuxPath, "send-keys", "-t", target, "-l", "\x1b[200~").Run(); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)

		// Send the actual text content
		cmd := exec.Command(tmuxPath, "send-keys", "-t", target, "-l", text)
		if err := cmd.Run(); err != nil {
			return err
		}
		// Use adaptive delay based on text length
		time.Sleep(pasteDelay)

		// Send bracketed paste end sequence: ESC [ 2 0 1 ~
		if err := exec.Command(tmuxPath, "send-keys", "-t", target, "-l", "\x1b[201~").Run(); err != nil {
			return err
		}
		// Brief delay before checking buffer
		time.Sleep(20 * time.Millisecond)
	} else {
		// Single-line text: use original simple approach
		listenLog("sendToTmuxWithDelay: single-line text (%d chars)", len(text))
		cmd := exec.Command(tmuxPath, "send-keys", "-t", target, "-l", text)
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	// Apply the specified delay as minimum wait time before checking buffer
	// This allows callers to request additional settling time if needed
	minDelay := delay
	if minDelay > 0 {
		time.Sleep(minDelay)
	}

	// Wait for the text to be fully processed by the TUI before sending Enter
	// This prevents Enter from being interpreted as a newline in the input buffer
	// We poll the pane buffer to verify the text appears, with a bounded timeout
	textAppeared := waitForTextInPane(target, text, 5*time.Second)
	if !textAppeared {
		listenLog("sendToTmuxWithDelay: text did not appear in pane after timeout, sending Enter anyway")
	}

	// Send Enter to execute the prompt
	// Single Enter is sufficient for normal prompt execution
	// The TUI will process the command and display results
	if err := exec.Command(tmuxPath, "send-keys", "-t", target, "C-m").Run(); err != nil {
		return err
	}

	return nil
}

// waitForTextInPane polls the tmux pane buffer until the expected text appears
// Returns true if text appears within timeout, false otherwise
// Checks for text AFTER the last prompt marker to avoid false positives on historical content
// For multi-line text, uses the last non-empty line for more reliable detection
func waitForTextInPane(target string, expectedText string, timeout time.Duration) bool {
	// Poll the pane buffer to verify text appears
	// This works for all text lengths and avoids timing races
	deadline := time.Now().Add(timeout)
	checkInterval := 50 * time.Millisecond

	// Normalize line endings for consistent searching
	expectedText = strings.ReplaceAll(expectedText, "\r\n", "\n")

	// Determine the best search text for verification
	searchText := expectedText

	// For multi-line text, extract the last non-empty line for more reliable detection
	// This handles bracketed paste mode where newlines are preserved
	if strings.Contains(expectedText, "\n") {
		lines := strings.Split(expectedText, "\n")
		// Find the last non-empty line
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) != "" {
				searchText = lines[i]
				break
			}
		}
		// Handle edge case: all lines are empty (only whitespace/newlines)
		if strings.TrimSpace(searchText) == "" {
			// For empty-only text, just check for any newlines appearing after prompt
			listenLog("waitForTextInPane: all lines empty, searching for any content")
			searchText = "\n"
		} else if len(searchText) < 5 {
			// Last line is very short (e.g., "}", ")") - could cause false positives
			// Fall back to last 50 chars of the full text for more unique match
			if len(expectedText) > 50 {
				searchText = expectedText[len(expectedText)-50:]
			} else {
				// Use full text if it's not long enough
				searchText = expectedText
			}
			listenLog("waitForTextInPane: last line too short, using tail of text: %q", searchText)
		} else if searchText == expectedText && len(searchText) > 100 {
			// Fallback: couldn't find non-empty line and text is long
			searchText = searchText[len(searchText)-100:]
		}
		listenLog("waitForTextInPane: multi-line text, using search text: %q", searchText)
		// For multi-line text, keep the search strategy - don't override with full expectedText
	} else if len(searchText) > 100 {
		// Single-line: take last 100 chars for verification (more reliable than full text)
		searchText = searchText[len(searchText)-100:]
		// For very short single-line text, search for the full text
		if len(searchText) < 10 {
			searchText = expectedText
		}
	} else if len(searchText) < 10 {
		// Single-line and very short: search for the full text
		searchText = expectedText
	}

	for time.Now().Before(deadline) {
		// Use -e for escape sequences and -J to join wrapped lines
		// Do NOT use -C as it escapes non-ASCII bytes, breaking Unicode prompt detection
		cmd := exec.Command(tmuxPath, "capture-pane", "-t", target, "-p", "-e", "-J")
		out, err := cmd.Output()
		if err == nil {
			content := string(out)
			// Check for text AFTER the last prompt marker to ensure we're checking newly pasted content
			// This avoids false positives when the same text was sent previously
			if lastPromptIndex := strings.LastIndex(content, "❯"); lastPromptIndex >= 0 {
				// Only check content after the last prompt
				contentAfterPrompt := content[lastPromptIndex:]
				if strings.Contains(contentAfterPrompt, searchText) {
					return true
				}
			} else {
				// No prompt found, check entire buffer (for fresh panes)
				if strings.Contains(content, searchText) {
					return true
				}
			}
		}
		time.Sleep(checkInterval)
	}

	return false
}

func killTmuxWindow(windowID string, windowName string) error {
	target := tmuxTargetByID(windowID, windowName)
	cmd := exec.Command(tmuxPath, "kill-window", "-t", target)
	return cmd.Run()
}

func listTmuxWindows() ([]string, error) {
	cmd := exec.Command(tmuxPath, "list-windows", "-a", "-F", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var windows []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		name := scanner.Text()
		windows = append(windows, name)
	}
	return windows, nil
}

// ========== Pane Management ==========

// splitPane creates a new pane in the specified session's window
// Returns the new pane index string ("0", "1", etc.)
func splitPane(sessionName string, direction string) (string, error) {
	target, err := getCccWindowTarget(sessionName)
	if err != nil {
		return "", err
	}

	flag := "-h" // default horizontal (left/right)
	if direction == "vertical" {
		flag = "-v" // vertical (top/bottom)
	}

	// -P -F "#{pane_index}" prints the new pane index
	cmd := exec.Command(tmuxPath, "split-window", "-P", "-F", "#{pane_index}", "-t", target, flag)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to split pane: %w", err)
	}

	paneIndex := strings.TrimSpace(string(out))
	listenLog("splitPane: created pane %s in window %s (direction: %s)", paneIndex, target, direction)
	return paneIndex, nil
}

// getPanesInWindow returns all pane indices in the specified session's window
func getPanesInWindow(sessionName string) ([]string, error) {
	target, err := getCccWindowTarget(sessionName)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(tmuxPath, "list-panes", "-t", target, "-F", "#{pane_index}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var panes []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			panes = append(panes, line)
		}
	}
	return panes, nil
}

// killPane closes the specified pane in a session's window
func killPane(sessionName string, paneIndex string) error {
	target, err := getCccWindowTarget(sessionName)
	if err != nil {
		return err
	}

	paneTarget := fmt.Sprintf("%s.%s", target, paneIndex)
	if err := exec.Command(tmuxPath, "kill-pane", "-t", paneTarget).Run(); err != nil {
		return fmt.Errorf("failed to kill pane %s: %w", paneIndex, err)
	}

	listenLog("killPane: closed pane %s in window %s", paneIndex, target)
	return nil
}

// selectPane makes the specified pane active in the session's window
func selectPane(sessionName string, paneIndex string) error {
	target, err := getCccWindowTarget(sessionName)
	if err != nil {
		return err
	}

	paneTarget := fmt.Sprintf("%s.%s", target, paneIndex)
	if err := exec.Command(tmuxPath, "select-pane", "-t", paneTarget).Run(); err != nil {
		return fmt.Errorf("failed to select pane %s: %w", paneIndex, err)
	}

	listenLog("selectPane: activated pane %s in window %s", paneIndex, target)
	return nil
}

// getActivePane returns the currently active pane index in the session's window
func getActivePane(sessionName string) (string, error) {
	target, err := getCccWindowTarget(sessionName)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(tmuxPath, "list-panes", "-t", target, "-F", "#{pane_active}\t#{pane_index}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[0] == "1" {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("no active pane found in %s", target)
}

// switchToPane switches context to a specific pane — respawns it and starts Claude
// Similar to switchSessionInWindow but targets a specific pane index
func switchToPane(sessionName string, paneIndex string, workDir string, providerName string, sessionID string) error {
	target, err := getCccWindowTarget(sessionName)
	if err != nil {
		return err
	}

	paneTarget := fmt.Sprintf("%s.%s", target, paneIndex)

	// Check if Claude is already running in this pane
	if tmuxTargetHasClaudeRunning(paneTarget) {
		listenLog("switchToPane: Claude already running in pane %s", paneIndex)
		return selectPane(sessionName, paneIndex)
	}

	// Build the ccc run command
	runCmd := cccPath + " run"
	if sessionID != "" {
		runCmd += " --resume " + shellQuote(sessionID)
	}
	if providerName != "" {
		runCmd += " --provider " + shellQuote(providerName)
	}

	// Respawn the pane
	if err := exec.Command(tmuxPath, "respawn-pane", "-t", paneTarget, "-k").Run(); err != nil {
		return fmt.Errorf("failed to respawn pane: %w", err)
	}

	// Wait for respawn
	time.Sleep(500 * time.Millisecond)

	// Send command to start Claude
	fullCmd := "cd " + shellQuote(workDir) + " && " + runCmd
	if err := exec.Command(tmuxPath, "send-keys", "-t", paneTarget, fullCmd, "C-m").Run(); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	// Select the pane to make it active
	return selectPane(sessionName, paneIndex)
}

// syncPanes reconciles the config's pane map with actual tmux pane state
// Removes stale entries (panes in config but not in tmux)
// Adds empty shells for new panes (in tmux but not in config)
func syncPanes(config *Config, sessionName string) error {
	info := config.Sessions[sessionName]
	if info == nil {
		return nil
	}

	// Get actual pane state from tmux
	actualPanes, err := getPanesInWindow(sessionName)
	if err != nil {
		// Window might not exist yet — skip sync
		return nil
	}

	actualSet := make(map[string]bool)
	for _, p := range actualPanes {
		actualSet[p] = true
	}

	// Initialize panes map if needed
	if info.Panes == nil {
		info.Panes = make(map[string]*PaneInfo)
	}

	// Remove stale config entries
	for paneIndex := range info.Panes {
		if !actualSet[paneIndex] {
			delete(info.Panes, paneIndex)
			listenLog("syncPanes: removed stale pane %s from config", paneIndex)
		}
	}

	// Add new panes found in tmux
	for _, paneIndex := range actualPanes {
		if _, exists := info.Panes[paneIndex]; !exists {
			info.Panes[paneIndex] = &PaneInfo{PaneIndex: paneIndex}
			listenLog("syncPanes: added new pane %s to config", paneIndex)
		}
	}

	// Validate ActivePane exists
	if info.ActivePane != "" {
		if !actualSet[info.ActivePane] {
			// Active pane no longer exists, reset to "0"
			info.ActivePane = "0"
			listenLog("syncPanes: ActivePane was invalid, reset to 0")
		}
	}

	return nil
}

// resolvePaneTarget returns the tmux target for a session, accounting for panes
// If the session has panes and ActivePane is set, returns pane-specific target
// Otherwise returns window-level target (backward compatible)
func resolvePaneTarget(config *Config, sessionName string) (string, error) {
	info := config.Sessions[sessionName]
	if info == nil || info.Panes == nil || info.ActivePane == "" {
		return getCccWindowTarget(sessionName)
	}

	target, err := getCccWindowTarget(sessionName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s", target, info.ActivePane), nil
}

// killTmuxSession kills an entire tmux session (used for temporary sessions like auth)
func killTmuxSession(name string) error {
	cmd := exec.Command(tmuxPath, "kill-session", "-t", name)
	return cmd.Run()
}
