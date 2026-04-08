package main

import (
	"github.com/tuannvm/ccc/pkg/tmux"
)

// tmuxWindowHasClaudeRunning checks if the tmux window has a functional Claude/ccc process running
// Returns false if window doesn't exist or only has a shell (zsh/bash) without Claude
// Handles npm-installed Claude which shows as 'node' or 'nodejs' process
// Checks only the ACTIVE pane to avoid false positives in split-pane windows
func tmuxWindowHasClaudeRunning(windowID string, windowName string) bool {
	return tmux.WindowHasClaudeRunning(windowID, windowName)
}

// tmuxTargetHasClaudeRunning checks if a tmux target (pane or window) has Claude running
// This is the shared implementation used by both tmuxWindowHasClaudeRunning and currentPaneHasClaudeRunning
// Uses hybrid detection: process name check + process tree check + prompt character check
func tmuxTargetHasClaudeRunning(target string) bool {
	return tmux.TargetHasClaudeRunning(target)
}

// tmuxPaneHasClaudePrompt checks if the tmux pane contains Claude's prompt character (❯)
// This is used to verify that a node/nodejs process is actually Claude Code
// The target can be a pane ID (%0) or window:pane format (session:window.pane)
// NOTE: This only checks for the prompt anywhere in the buffer. For detecting ACTIVE
// sessions, use tmuxPaneHasActiveClaudePrompt() instead to avoid false positives.
func tmuxPaneHasClaudePrompt(paneTarget string) bool {
	return tmux.PaneHasClaudePrompt(paneTarget)
}

// tmuxPaneHasActiveClaudePrompt checks if the tmux pane has Claude's prompt at the END of the buffer
// This indicates Claude is currently active and waiting for input (not just historical content)
// The target can be a pane ID (%0) or window:pane format (session:window.pane)
// Uses strict detection with context requirement to avoid false positives from shell prompts
func tmuxPaneHasActiveClaudePrompt(paneTarget string) bool {
	return tmux.PaneHasActiveClaudePrompt(paneTarget)
}

// tmuxPaneHasClaudeChild checks if the pane's process (typically a shell) has Claude as a child process
// This is used when pane_current_command shows a shell (zsh/bash) but Claude might be running under it
// Returns true if the pane has a child process that is Claude (claude binary or node with claude/cli)
func tmuxPaneHasClaudeChild(paneID string) bool {
	return tmux.PaneHasClaudeChild(paneID)
}

// tmuxPaneIsClaudeProcess checks if the pane's foreground process is actually the Claude CLI
// This finds the foreground node process (child of shell) and examines its command line
// Returns true if the pane is running a node process with claude/cli in its command line
func tmuxPaneIsClaudeProcess(paneID string) bool {
	return tmux.PaneIsClaudeProcess(paneID)
}

// tmuxWindowHasShellRunning checks if the target tmux window has a shell running
// Returns true if the ACTIVE pane has a shell, which means the window is ready for input
// This is scoped to the active pane to avoid misrouting commands in split-pane windows
// Supports common shells: zsh, bash, sh, fish, dash, nu, elvish, xonsh, tcsh, csh, ksh
func tmuxWindowHasShellRunning(windowID string, windowName string) bool {
	return tmux.WindowHasShellRunning(windowID, windowName)
}
