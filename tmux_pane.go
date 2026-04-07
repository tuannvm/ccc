package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// detectConsentDialog checks if the content matches a consent/trust dialog pattern.
// Uses generic pattern detection to handle UI variations across versions.
func detectConsentDialog(content string) bool {
	lowerContent := strings.ToLower(content)

	// Generic consent dialog detection using multiple heuristics:
	// 1. Has numbered options (1/2, [1]/[2], etc.) - characteristic of selection dialogs
	// 2. Has trust/safety keywords AND exit/decline keywords - consent dialog structure
	// 3. NOT showing Claude's active input prompt (indicated by "❯" with Claude context)

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

	trustKeywords := []string{"trust", "safety check", "confirm", "allow", "proceed", "project you created", "you trust"}
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

	return isConsentDialog
}

// captureVisiblePane captures only the visible portion of a tmux pane to avoid
// matching stale dialog text from scrollback. Returns the captured content as string.
func captureVisiblePane(target string) string {
	// Get the pane height to limit capture to visible window only
	heightCmd := exec.Command(tmuxPath, "display-message", "-p", "-t", target, "#{pane_height}")
	heightOut, err := heightCmd.Output()
	if err != nil {
		// Fallback to unbounded capture if height query fails
		cmd := exec.Command(tmuxPath, "capture-pane", "-t", target, "-p")
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return string(out)
	}

	height, err := strconv.Atoi(strings.TrimSpace(string(heightOut)))
	if err != nil || height <= 0 {
		// Fallback to unbounded capture if height is invalid
		cmd := exec.Command(tmuxPath, "capture-pane", "-t", target, "-p")
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return string(out)
	}

	// Capture only the visible window using -S -0 -E <height-1>
	// -S -0: start from line 0 (top visible line, not scrollback)
	// -E <height-1>: capture up to but not including the line after the visible pane
	// (tmux line numbering is zero-based, -E bound is inclusive)
	cmd := exec.Command(tmuxPath, "capture-pane", "-t", target, "-p", "-S", "-0", "-E", fmt.Sprintf("%d", height-1))
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// autoAcceptTrustDialog checks if a workspace trust/consent dialog is visible
// and auto-accepts it by sending Enter. Returns true if dialog was detected and accepted.
// Uses generic pattern detection instead of exact strings to handle UI variations.
// Uses bounded capture to avoid matching stale dialog text from scrollback.
func autoAcceptTrustDialog(target string) bool {
	content := captureVisiblePane(target)
	if content == "" {
		return false
	}

	if detectConsentDialog(content) {
		listenLog("autoAcceptTrustDialog[%s]: detected consent dialog (pattern match), auto-accepting", target)
		if err := exec.Command(tmuxPath, "send-keys", "-t", target, "Enter").Run(); err != nil {
			listenLog("autoAcceptTrustDialog[%s]: failed to send Enter: %v", target, err)
			return false
		}
		return true
	}
	return false
}

// waitForClaude polls the tmux pane until Claude Code's input prompt appears.
// It also handles the workspace trust dialog introduced in Claude Code 2.1.84+
// by auto-accepting it when detected.
// Note: Only -p (print) mode skips this dialog; --dangerously-skip-permissions does not.
func waitForClaude(target string, timeout time.Duration) error {
	// Poll faster for short timeouts (message sending), slower for startup
	interval := 100 * time.Millisecond
	if timeout > 10*time.Second {
		interval = 500 * time.Millisecond
	}
	const trustDialogDismissDelay = 1 * time.Second
	trustDialogHandled := false
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Handle workspace trust/consent dialog (Claude Code 2.1.84+)
		// Uses bounded capture to avoid matching stale dialog text
		if !trustDialogHandled && autoAcceptTrustDialog(target) {
			trustDialogHandled = true
			time.Sleep(trustDialogDismissDelay)
			continue
		}

		// Check for Claude prompt using bounded capture
		content := captureVisiblePane(target)
		if strings.Contains(content, "❯") {
			return nil
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
	delay = min(delay, 5*time.Second)
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
		pasteDelay = max(pasteDelay, 20*time.Millisecond)
		pasteDelay = min(pasteDelay, 200*time.Millisecond)

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

	// Dismiss autocomplete popup that bracketed paste may trigger
	time.Sleep(100 * time.Millisecond)
	if err := exec.Command(tmuxPath, "send-keys", "-t", target, "Escape").Run(); err != nil {
		listenLog("sendToTmuxWithDelay: failed to send Escape: %v", err)
	}

	// Send Enter to execute the prompt
	// Claude Code 2.1.84+ requires double Enter to submit
	time.Sleep(50 * time.Millisecond)
	if err := exec.Command(tmuxPath, "send-keys", "-t", target, "Enter").Run(); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	if err := exec.Command(tmuxPath, "send-keys", "-t", target, "Enter").Run(); err != nil {
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

// killTmuxSession kills an entire tmux session (used for temporary sessions like auth)
func killTmuxSession(name string) error {
	cmd := exec.Command(tmuxPath, "kill-session", "-t", name)
	return cmd.Run()
}
