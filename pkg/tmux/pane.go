package tmux

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// WindowHasClaudeRunning checks if the tmux window has a functional Claude/ccc process running
// Returns false if window doesn't exist or only has a shell (zsh/bash) without Claude
// Handles npm-installed Claude which shows as 'node' or 'nodejs' process
// Checks only the ACTIVE pane to avoid false positives in split-pane windows
func WindowHasClaudeRunning(windowID string, windowName string) bool {
	// First find the window
	target := TargetByID(windowID, windowName)
	if target == "" {
		return false
	}
	return TargetHasClaudeRunning(target)
}

// WindowHasAgentRunning checks if the active pane has the selected backend running.
func WindowHasAgentRunning(windowID string, windowName string, providerName string) bool {
	target := TargetByID(windowID, windowName)
	if target == "" {
		return false
	}
	if isCodexProviderName(providerName) {
		return TargetHasCodexRunning(target)
	}
	return TargetHasClaudeRunning(target)
}

func isCodexProviderName(name string) bool {
	return strings.EqualFold(name, "codex")
}

// TargetHasClaudeRunning checks if a tmux target (pane or window) has Claude running
// This is the shared implementation used by both WindowHasClaudeRunning and currentPaneHasClaudeRunning
// Uses hybrid detection: process name check + process tree check + prompt character check
func TargetHasClaudeRunning(target string) bool {

	// Get pane IDs, active flag, and commands together to check only the active pane
	cmd := exec.Command(TmuxPath, "list-panes", "-t", target, "-F", "#{pane_active}\t#{pane_id}\t#{pane_current_command}")
	out, err := cmd.Output()
	if err != nil {
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
	var activePaneID string
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
		paneCmd = strings.TrimSpace(paneCmd)

		// Process-based detection: check if this pane has claude running
		if strings.HasPrefix(paneCmd, "claude") {
			return true
		}
		// Check for npm-installed Claude (shows as 'node' or 'nodejs')
		if paneCmd == "node" || paneCmd == "nodejs" {
			// Verify it's actually Claude by examining the process command line
			if PaneIsClaudeProcess(paneID) {
				return true
			}
		}
		// Special case: "ccc" process means the wrapper is running
		// This could mean Claude is starting OR it's already running
		// We need prompt-based detection to tell the difference
		if paneCmd == "ccc" || paneCmd == "ccc run" {
			// Check if Claude prompt is present at the END of the buffer - means Claude is actually running
			if PaneHasActiveClaudePrompt(paneID) {
				return true
			}
			// ccc without active prompt - check for workspace trust dialog (Claude Code 2.1.84+)
			// The trust dialog blocks Claude from showing its input prompt
			if AutoAcceptTrustDialog(paneID) {
				// Dialog was auto-accepted, but Claude is still loading
				return false // retry on next poll
			}
			// ccc without active prompt means it's still starting up
		}
		// Special case: shell process (zsh/bash) - check if it has Claude as a child process
		// This handles cases where pane_current_command shows the shell but Claude is running under it
		if shells[paneCmd] {
			// Check if this shell has Claude/node children
			if PaneHasClaudeChild(paneID) {
				return true
			}
			// Shell without Claude children - check for workspace trust/consent dialog
			// The dialog may be visible while Claude is starting up
			if AutoAcceptTrustDialog(paneID) {
				// Dialog was auto-accepted, but Claude is still loading
				return false // retry on next poll
			}
		}
	}

	// Process-based detection failed, try prompt-based detection as final fallback
	// We use strict detection here to avoid false positives from shell prompts
	// Only accept the prompt if we have Claude-specific context (via PaneHasActiveClaudePrompt)
	if activePaneID != "" && PaneHasActiveClaudePrompt(activePaneID) {
		return true
	}

	// If we reach here, the active pane doesn't have Claude running
	return false
}

// TargetHasCodexRunning checks if a tmux target has Codex CLI running.
func TargetHasCodexRunning(target string) bool {
	cmd := exec.Command(TmuxPath, "list-panes", "-t", target, "-F", "#{pane_active}\t#{pane_id}\t#{pane_current_command}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	shells := map[string]bool{
		"sh": true, "bash": true, "zsh": true, "fish": true,
		"dash": true, "nu": true, "elvish": true, "xonsh": true,
		"tcsh": true, "csh": true, "ksh": true,
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var activePaneID string
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		isActive, paneID, paneCmd := parts[0], parts[1], strings.TrimSpace(parts[2])
		if isActive != "1" {
			continue
		}
		activePaneID = paneID
		if paneCmd == "codex" {
			return true
		}
		if paneCmd == "node" || paneCmd == "nodejs" {
			if PaneIsCodexProcess(paneID) {
				return true
			}
		}
		if paneCmd == "ccc" || paneCmd == "ccc run" {
			if PaneHasActiveCodexPrompt(paneID) {
				return true
			}
			if AutoAcceptTrustDialog(paneID) {
				return false
			}
		}
		if shells[paneCmd] {
			if PaneHasCodexChild(paneID) {
				return true
			}
			if AutoAcceptTrustDialog(paneID) {
				return false
			}
		}
	}
	return activePaneID != "" && PaneHasActiveCodexPrompt(activePaneID)
}

// PaneHasActiveCodexPrompt checks recent pane content for Codex's interactive prompt.
func PaneHasActiveCodexPrompt(paneTarget string) bool {
	cmd := exec.Command(TmuxPath, "capture-pane", "-t", paneTarget, "-p", "-J", "-S", "-15")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return hasActiveCodexPrompt(string(out))
}

func hasActiveCodexPrompt(content string) bool {
	lowerContent := strings.ToLower(content)
	if !strings.Contains(lowerContent, "codex") && !strings.Contains(lowerContent, "openai") {
		return false
	}
	lines := strings.Split(strings.TrimSpace(content), "\n")
	nonEmptySeen := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		nonEmptySeen++
		if strings.Contains(line, "›") {
			return true
		}
		if nonEmptySeen >= 5 {
			break
		}
	}
	return false
}

func PaneHasCodexChild(paneID string) bool {
	cmd := exec.Command(TmuxPath, "display-message", "-t", paneID, "-p", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	panePid := strings.TrimSpace(string(out))
	if panePid == "" || panePid == "0" {
		return false
	}
	psOut, err := exec.Command("ps", "-o", "pid,command", "--ppid", panePid, "--no-headers").Output()
	if err != nil {
		allPsOut, psErr := exec.Command("ps", "-ax", "-o", "pid,ppid,command").Output()
		if psErr != nil {
			return false
		}
		psOut = filterChildProcesses(allPsOut, panePid)
	}
	return psOutputHasCodex(psOut)
}

func PaneIsCodexProcess(paneID string) bool {
	cmd := exec.Command(TmuxPath, "display-message", "-t", paneID, "-p", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	shellPid := strings.TrimSpace(string(out))
	if shellPid == "" || shellPid == "0" {
		return false
	}
	psOut, err := exec.Command("ps", "-o", "pid,command", "--ppid", shellPid, "--no-headers").Output()
	if err != nil {
		allPsOut, psErr := exec.Command("ps", "-ax", "-o", "pid,ppid,command").Output()
		if psErr != nil {
			return PaneHasActiveCodexPrompt(paneID)
		}
		psOut = filterChildProcesses(allPsOut, shellPid)
	}
	return psOutputHasCodex(psOut)
}

func psOutputHasCodex(psOut []byte) bool {
	for _, line := range strings.Split(string(psOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		cmdline := strings.ToLower(strings.TrimSpace(parts[1]))
		if strings.Contains(cmdline, "codex") && !strings.Contains(cmdline, "ccc") {
			return true
		}
	}
	return false
}

// PaneHasClaudePrompt checks if the tmux pane contains Claude's prompt character (❯)
// This is used to verify that a node/nodejs process is actually Claude Code
// The target can be a pane ID (%0) or window:pane format (session:window.pane)
// NOTE: This only checks for the prompt anywhere in the buffer. For detecting ACTIVE
// sessions, use PaneHasActiveClaudePrompt() instead to avoid false positives.
func PaneHasClaudePrompt(paneTarget string) bool {
	// Capture the pane buffer and check for Claude's prompt
	// Use -e for escape sequences and -J to join wrapped lines
	// Do NOT use -C as it escapes non-ASCII bytes, breaking Unicode prompt detection
	cmd := exec.Command(TmuxPath, "capture-pane", "-t", paneTarget, "-p", "-e", "-J")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	content := string(out)
	// Claude Code shows "❯" when ready for input
	// Also check for "How can I help?" which appears in the welcome message
	return strings.Contains(content, "❯") || strings.Contains(content, "How can I help")
}

// PaneHasActiveClaudePrompt checks if the tmux pane has Claude's prompt at the END of the buffer
// This indicates Claude is currently active and waiting for input (not just historical content)
// The target can be a pane ID (%0) or window:pane format (session:window.pane)
// Uses strict detection with context requirement to avoid false positives from shell prompts
func PaneHasActiveClaudePrompt(paneTarget string) bool {
	// Capture the last few lines of the pane buffer to check for active prompt
	// Use -J to join wrapped lines, -S -15 for last 15 lines
	// Do NOT use -e: ANSI escape codes make empty lines appear non-empty,
	// which consumes the scan budget before reaching the actual prompt line
	cmd := exec.Command(TmuxPath, "capture-pane", "-t", paneTarget, "-p", "-J", "-S", "-15")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	content := string(out)
	// Check if a recent non-empty line contains Claude's prompt
	// Scan up to 3 non-empty lines from the bottom to handle Claude's status bar
	// which appears after the prompt line (model info, costs, etc.)
	lines := strings.Split(strings.TrimSpace(content), "\n")

	// Pre-compute Claude context check (shared across all lines)
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

	// Find the prompt in the last few non-empty lines
	// Scan up to 5 non-empty lines to skip past Claude's status bar,
	// separator lines, and other decorative elements that appear after the prompt
	nonEmptySeen := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			nonEmptySeen++
			if strings.Contains(line, "❯") && hasClaudeContext {
				return true
			}
			if nonEmptySeen >= 5 {
				break
			}
		}
	}
	return false
}

// PaneHasClaudeChild checks if the pane's process (typically a shell) has Claude as a child process
// This is used when pane_current_command shows a shell (zsh/bash) but Claude might be running under it
// Returns true if the pane has a child process that is Claude (claude binary or node with claude/cli)
func PaneHasClaudeChild(paneID string) bool {
	// Get the pane's PID using tmux
	cmd := exec.Command(TmuxPath, "display-message", "-t", paneID, "-p", "#{pane_pid}")
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
			return true
		}
		// Check for node process with claude/cli
		if (strings.HasPrefix(cmdline, "node ") || strings.HasPrefix(cmdline, "nodejs ")) &&
			(strings.Contains(cmdline, "/claude") || strings.Contains(cmdline, "/@anthropic-ai/")) {
			return true
		}
	}

	return false
}

// PaneIsClaudeProcess checks if the pane's foreground process is actually the Claude CLI
// This finds the foreground node process (child of shell) and examines its command line
// Returns true if the pane is running a node process with claude/cli in its command line
func PaneIsClaudeProcess(paneID string) bool {
	// Get the pane's PID (shell PID) using tmux
	cmd := exec.Command(TmuxPath, "display-message", "-t", paneID, "-p", "#{pane_pid}")
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
			return PaneHasClaudePrompt(paneID)
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
		isClaude := isNode && (strings.Contains(cmdline, "/claude ") || // "node .../claude" (global bin)
			strings.Contains(cmdline, "/claude.js ") || // direct script
			strings.Contains(cmdline, "/@anthropic-ai/") || // npm package
			strings.HasSuffix(cmdline, "/claude") || // ends with /claude
			strings.HasSuffix(cmdline, "/claude.js")) // ends with /claude.js

		if isClaude {
			return true
		}
	}

	// No node process found as child of shell, or not Claude
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

// WindowHasShellRunning checks if the target tmux window has a shell running
// Returns true if the ACTIVE pane has a shell, which means the window is ready for input
// This is scoped to the active pane to avoid misrouting commands in split-pane windows
// Supports common shells: zsh, bash, sh, fish, dash, nu, elvish, xonsh, tcsh, csh, ksh
func WindowHasShellRunning(windowID string, windowName string) bool {
	target := TargetByID(windowID, windowName)
	if target == "" {
		return false
	}

	// Get only the ACTIVE pane's current command
	// Using -t target without -F format gets us the active pane by default
	cmd := exec.Command(TmuxPath, "list-panes", "-t", target, "-F", "#{pane_active}\t#{pane_current_command}")
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

// DetectConsentDialog checks if the content matches a consent/trust dialog pattern.
// Uses generic pattern detection to handle UI variations across versions.
func DetectConsentDialog(content string) bool {
	_, ok := ConsentDialogChoice(content)
	return ok
}

// ConsentDialogChoice returns the numeric option ccc should select for known
// trust/consent dialogs. Most trust prompts should proceed, but Codex's external
// agent migration prompt should be skipped so startup does not block.
func ConsentDialogChoice(content string) (string, bool) {
	lowerContent := strings.ToLower(content)

	if strings.Contains(lowerContent, "external agent config detected") &&
		strings.Contains(lowerContent, "migrate hooks") &&
		strings.Contains(lowerContent, "skip for now") {
		return "2", true
	}

	// Claude Code 2.1.119 shows a workspace safety screen headed by
	// "Accessing workspace:" before the numbered choices. Match this first so
	// the detector still works when the visible capture clips the options.
	if strings.Contains(lowerContent, "accessing workspace:") &&
		strings.Contains(lowerContent, "quick safety check") &&
		strings.Contains(lowerContent, "claude code'll be able to read") {
		return "1", true
	}

	// Check for numbered options - looks for patterns like "1.", "2)", "[1]", etc.
	optionPattern := regexp.MustCompile(`[1-9][\.\)\]]`)
	lines := strings.Split(content, "\n")
	foundDigits := make(map[int]bool)
	for _, line := range lines {
		matches := optionPattern.FindAllStringIndex(line, -1)
		for _, match := range matches {
			for i := match[0]; i < match[1]; i++ {
				if line[i] >= '1' && line[i] <= '9' {
					foundDigits[int(line[i]-'0')] = true
					break
				}
			}
		}
	}
	hasNumberedOptions := len(foundDigits) >= 2

	trustKeywords := []string{"accessing workspace", "trust", "safety check", "confirm", "allow", "proceed", "project you created", "you trust"}
	exitKeywords := []string{"exit", "decline", "cancel", "skip", "deny"}

	hasTrustKeyword := false
	hasExitKeyword := false
	for _, kw := range trustKeywords {
		if strings.Contains(lowerContent, strings.ToLower(kw)) {
			hasTrustKeyword = true
			break
		}
	}
	for _, kw := range exitKeywords {
		if strings.Contains(lowerContent, strings.ToLower(kw)) {
			hasExitKeyword = true
			break
		}
	}

	// Active Claude context - more specific patterns that indicate real Claude session
	hasActiveClaudeContext := strings.Contains(content, "How can I help") ||
		strings.Contains(content, "I can help") ||
		strings.Contains(content, "Bash:") ||
		strings.Contains(content, "function:") ||
		strings.Contains(content, "result:")

	hasPrompt := strings.Contains(content, "❯")
	isConsentDialog := hasNumberedOptions && hasTrustKeyword && hasExitKeyword &&
		(!hasPrompt || !hasActiveClaudeContext)

	if isConsentDialog {
		return "1", true
	}
	return "", false
}
