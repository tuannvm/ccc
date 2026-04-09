package tmux

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/tuannvm/ccc/pkg/shell"
	"time"
)

// WorktreeAutoGenerate is the sentinel value for auto-generating worktree names
const WorktreeAutoGenerate = "AUTO"

// SwitchSessionInWindow switches the context to the project's window in the ccc session
// Each project gets its own named window within the main "ccc" session
// If skipRestart is true and the requested session is already active, it will skip restarting
func SwitchSessionInWindow(sessionName string, workDir string, providerName string, sessionID string, worktreeName string, continueSession bool, skipRestart bool) error {
	// Ensure the project window exists in the ccc session (e.g., "ccc:TommyClaw")
	target, err := EnsureProjectWindow(sessionName)
	if err != nil {
		return err
	}

	// Check if we should skip restarting
	// Only skip if: 1) skipRestart is true, AND 2) the target window already has Claude/shell running
	shouldRestart := true
	if skipRestart {
		// Check if the target window already has Claude or a shell running
		// A shell means the window is ready for input and we can send commands directly
		if WindowHasClaudeRunning(target, "") || WindowHasShellRunning(target, "") {
			// Target window already has Claude or shell running - skip respawn
			// We can send commands directly without restarting
			shouldRestart = false
		} else {
			// When skipRestart=true but we don't detect Claude or shell, be extra cautious
			// This handles false negatives in detection where Claude is actually running
			// Check for Claude prompt in the pane content as a fallback
			if PaneHasActiveClaudePrompt(target) {
				fmt.Printf("skipRestart=true: Claude prompt detected in pane content (fallback detection)\n")
				shouldRestart = false
			}
		}
	}

	// Build the ccc run command with all flags
	// Use ccc run instead of claude directly to ensure provider env setup
	runCmd := CCCPath + " run"

	// Determine if we should continue an existing session
	// Only add -c flag if Claude is actually running OR we have a specific session ID
	// This prevents "No conversation found to continue" errors on new sessions
	if sessionID != "" {
		// Explicit session ID to resume - use --resume flag
		runCmd += " --resume " + shell.Quote(sessionID)
	} else if continueSession {
		// Check if Claude is actually running before adding -c flag
		if WindowHasClaudeRunning(target, "") {
			runCmd += " -c"
			fmt.Printf("Claude is running, will continue existing session\n")
		} else {
			fmt.Printf("continueSession=true but Claude not running, will start new session instead\n")
		}
	}
	// If no sessionID and not continueSession (or Claude not running), start fresh (no flags)

	// Always pass provider flag if specified
	// This ensures provider-agnostic behavior - no special case for "anthropic"
	if providerName != "" {
		runCmd += " --provider " + shell.Quote(providerName)
	}
	// worktreeName is WorktreeAutoGenerate for auto-generation, or a specific name
	// ccc run passes --worktree with optional value
	if worktreeName != "" {
		if worktreeName == WorktreeAutoGenerate {
			// Auto-generate: pass --worktree without a value
			runCmd += " --worktree"
		} else {
			runCmd += " --worktree " + shell.Quote(worktreeName)
		}
	}

	// Send commands to switch session context
	if shouldRestart {
		// Strategy: Always use respawn-pane for clean pane restart when shouldRestart is true
		// This ensures we have a clean shell regardless of what's currently running
		// (Claude, vim, less, or any other foreground process)
		// respawn-pane kills the process and restarts the shell atomically

		fmt.Printf("Respawning pane for clean session restart\n")
		if err := exec.Command(TmuxPath, "respawn-pane", "-t", target, "-k").Run(); err != nil {
			return fmt.Errorf("failed to respawn pane: %w", err)
		}

		// Poll for pane restart completion with bounded timeout
		// Shell startup can take longer on slower systems or under load
		deadline := time.Now().Add(5 * time.Second)
		respawnComplete := false
		for time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
			if !WindowHasClaudeRunning(target, "") {
				respawnComplete = true
				fmt.Printf("Pane respawn complete, shell is ready\n")
				break
			}
		}

		if !respawnComplete && WindowHasClaudeRunning(target, "") {
			return fmt.Errorf("pane respawn timed out - still shows Claude running after 5 seconds")
		}

		// Verify we have a shell running now
		if WindowHasClaudeRunning(target, "") {
			return fmt.Errorf("pane still shows Claude running after respawn - cannot proceed safely")
		}

		// Change to work directory and start claude via ccc run (as one command)
		fullCmd := "cd " + shell.Quote(workDir) + " && " + runCmd
		if err := exec.Command(TmuxPath, "send-keys", "-t", target, fullCmd, "C-m").Run(); err != nil {
			return fmt.Errorf("failed to send command: %w", err)
		}
	} else {
		// Not restarting - check what's running in the target window
		if WindowHasClaudeRunning(target, "") {
			// Claude is already running in this window - don't send any command
			// The user can continue their existing session
			fmt.Printf("Claude already running in target window, skipping command send\n")
		} else if WindowHasShellRunning(target, "") {
			// Shell is running - decide whether to start Claude
			// When skipRestart=true, the caller indicates the session should already be usable
			// This means Claude might be running but not properly detected (false negative)
			// In this case, we should NOT send a restart command to avoid disrupting the session
			if skipRestart {
				fmt.Printf("Shell detected with skipRestart=true - not sending restart command to preserve session state\n")
			} else {
				// Shell is running but no Claude - start Claude
				fullCmd := "cd " + shell.Quote(workDir) + " && " + runCmd
				if err := exec.Command(TmuxPath, "send-keys", "-t", target, fullCmd, "C-m").Run(); err != nil {
					return fmt.Errorf("failed to send command: %w", err)
				}
			}
		} else {
			// Pane is empty or has unknown process
			// When skipRestart=true, be conservative and don't respawn
			// The session might be in a transient state (Claude starting, tool running)
			if skipRestart {
				fmt.Printf("Pane has unknown state but skipRestart=true - not respawning to preserve session state\n")
			} else {
				// Respawn to get clean state
				fmt.Printf("Pane has unknown state, respawning for clean start\n")
				if err := exec.Command(TmuxPath, "respawn-pane", "-t", target, "-k").Run(); err != nil {
					return fmt.Errorf("failed to respawn pane: %w", err)
				}

				// Wait for respawn and send command
				time.Sleep(500 * time.Millisecond)
				fullCmd := "cd " + shell.Quote(workDir) + " && " + runCmd
				if err := exec.Command(TmuxPath, "send-keys", "-t", target, fullCmd, "C-m").Run(); err != nil {
					return fmt.Errorf("failed to send command: %w", err)
				}
			}
		}
	}

	// Select the window to make it active (this is important when switching between projects)
	// Only attempt selection if there might be an attached client - ignore errors in headless mode
	exec.Command(TmuxPath, "select-window", "-t", target).Run()
	// We ignore the error from select-window because:
	// 1. In headless/non-interactive mode, there's no client to switch
	// 2. The window is still created and commands are sent successfully
	// 3. When the user later attaches, they'll see the correct window

	// Set window title for display purposes, but keep the base name stable
	// We use the 'window-status-format' to show provider info without renaming the window
	// This ensures EnsureProjectWindow can always find the window by its original name
	if providerName != "" && len(providerName) > 0 {
		// Store provider info in a user option for display purposes
		prefix := strings.ToUpper(string(providerName[0]))
		exec.Command(TmuxPath, "set-window-option", "-t", target, "@ccc-provider", prefix).Run()
	}

	// After starting Claude, poll for consent dialog and auto-accept it
	// This is needed for new sessions where Claude Code 2.1.84+ shows a workspace trust dialog
	// IMPORTANT: Only poll for new sessions (sessionID == ""), not when resuming.
	// Consent dialogs only appear on first startup, not when resuming existing sessions.
	// Polling during resume can interfere with message delivery by sending spurious Enter keys.
	if shouldRestart && sessionID == "" {
		fmt.Printf("Polling for consent dialog after Claude startup...\n")
		pollDeadline := time.Now().Add(10 * time.Second)
		consumedDialog := false
		for time.Now().Before(pollDeadline) {
			time.Sleep(500 * time.Millisecond)

			// Get the pane ID for the target
			cmd := exec.Command(TmuxPath, "list-panes", "-t", target, "-F", "#{pane_active}\t#{pane_id}")
			out, err := cmd.Output()
			if err != nil {
				continue
			}

			var activePaneID string
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				parts := strings.SplitN(line, "\t", 2)
				if len(parts) == 2 && parts[0] == "1" {
					activePaneID = parts[1]
					break
				}
			}

			if activePaneID == "" {
				continue
			}

			// Check for consent dialog and auto-accept
			if AutoAcceptTrustDialog(activePaneID) {
				fmt.Printf("Consent dialog auto-accepted during startup polling\n")
				consumedDialog = true
				// Give Claude time to proceed after accepting
				time.Sleep(2 * time.Second)
				break
			}

			// If Claude prompt is detected, Claude is ready - no need to continue polling
			if PaneHasActiveClaudePrompt(target) {
				fmt.Printf("Claude prompt detected, startup complete\n")
				break
			}
		}
		if consumedDialog {
			fmt.Printf("Consent dialog was consumed during startup\n")
		}
	}

	return nil
}

