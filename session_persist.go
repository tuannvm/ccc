package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"

	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/session"
)

// inferRoleFromTranscriptPath extracts the role from a transcript file path
// Returns empty string if no role is found
//
// Handles multiple transcript naming patterns:
// - session-planner.jsonl, session_planner.jsonl
// - planner.jsonl, planner-session.jsonl
// - session.planner.jsonl (with dot separator)
func inferRoleFromTranscriptPath(transcriptPath string) session.PaneRole {
	if transcriptPath == "" {
		return ""
	}
	base := filepath.Base(transcriptPath)
	// Remove extensions - handle multiple extensions safely
	for {
		newBase := strings.TrimSuffix(base, ".jsonl")
		if newBase == base {
			newBase = strings.TrimSuffix(base, ".json")
		}
		if newBase == base {
			break // No more extensions to remove
		}
		base = newBase
	}

	// Convert to lowercase for case-insensitive matching
	baseLower := strings.ToLower(base)

	// Check for role keywords anywhere in the filename
	// Order matters: check for longer substrings first to avoid false matches
	if strings.Contains(baseLower, "planner") {
		return session.RolePlanner
	}
	if strings.Contains(baseLower, "executor") {
		return session.RoleExecutor
	}
	if strings.Contains(baseLower, "reviewer") {
		return session.RoleReviewer
	}

	// No role found in path
	return ""
}

// inferRoleFromTmuxPane determines the role by querying tmux for the active pane
// Team sessions have panes named: Planner, Executor, Reviewer
// Falls back to pane index if names are not set: 1=planner, 2=executor, 3=reviewer
// Returns empty string if tmux is not available or query fails
func inferRoleFromTmuxPane(sessionName string) session.PaneRole {
	if tmux.TmuxPath == "" || sessionName == "" {
		return ""
	}
	// Query tmux for the active pane name in the session window
	// Format: "ccc-team:session-name"
	target := fmt.Sprintf("ccc-team:%s", sessionName)
	cmd := exec.Command(tmux.TmuxPath, "display-message", "-t", target, "-p", "#{pane_name}")
	out, err := cmd.Output()
	if err != nil {
		hookLog("inferRoleFromTmuxPane: tmux query failed: %v", err)
		return ""
	}
	paneName := strings.TrimSpace(string(out))
	// Map pane name to role (preferred - more explicit)
	roleMap := map[string]session.PaneRole{
		"Planner":  session.RolePlanner,
		"Executor": session.RoleExecutor,
		"Reviewer": session.RoleReviewer,
	}
	if role, ok := roleMap[paneName]; ok {
		hookLog("inferRoleFromTmuxPane: determined role=%s from pane name=%s", role, paneName)
		return role
	}
	// Fallback: Try pane index if names are not set (for legacy sessions)
	cmd2 := exec.Command(tmux.TmuxPath, "display-message", "-t", target, "-p", "#{pane_index}")
	out2, err2 := cmd2.Output()
	if err2 != nil {
		hookLog("inferRoleFromTmuxPane: pane index query failed: %v", err2)
		return ""
	}
	paneIndex := strings.TrimSpace(string(out2))
	// Map pane index to role (tmux uses 1-based indexing)
	indexMap := map[string]session.PaneRole{
		"1": session.RolePlanner,
		"2": session.RoleExecutor,
		"3": session.RoleReviewer,
	}
	if role, ok := indexMap[paneIndex]; ok {
		hookLog("inferRoleFromTmuxPane: determined role=%s from pane index=%s (fallback)", role, paneIndex)
		return role
	}
	hookLog("inferRoleFromTmuxPane: unknown pane name=%s or index=%s", paneName, paneIndex)
	return ""
}

// persistClaudeSessionID saves the claude session ID to config if changed
// For single sessions, stores in SessionInfo.ClaudeSessionID
// For team sessions, stores in the matching pane's PaneInfo.ClaudeSessionID
// It also clears the same claude_session_id from any other session/pane to prevent ambiguity
// For team sessions, uses transcriptPath to infer which pane/role this session ID belongs to
func persistClaudeSessionID(config *Config, sessName string, claudeSessionID string, transcriptPath string) {
	if claudeSessionID == "" || sessName == "" {
		return
	}

	// First check if this is a team session
	var sessInfo *SessionInfo
	var isTeam bool

	// Look up the session in single sessions first
	for name, info := range config.Sessions {
		if info != nil && name == sessName {
			sessInfo = info
			break
		}
	}

	// Check team sessions
	if sessInfo == nil {
		for _, info := range config.TeamSessions {
			if info != nil && info.SessionName == sessName {
				sessInfo = info
				isTeam = true
				break
			}
		}
	}

	if sessInfo == nil {
		return
	}

	// For team sessions, store the Claude session ID in the appropriate pane
	// Use transcript path to infer which role/pane this session ID belongs to
	if isTeam && sessInfo.Panes != nil {
		hookLog("persistClaudeSessionID: team session=%s, claudeSessionID=%s, transcript=%s", sessName, claudeSessionID, transcriptPath)

		// Infer role from transcript path
		role := inferRoleFromTranscriptPath(transcriptPath)
		if role != "" {
			hookLog("persistClaudeSessionID: inferred role=%s from transcript path", role)
			// Update only the specific pane for this role
			if pane, exists := sessInfo.Panes[role]; exists && pane != nil {
				if pane.ClaudeSessionID != claudeSessionID {
					// Clear this claudeSessionID from all OTHER panes to prevent ambiguity
					for otherRole, otherPane := range sessInfo.Panes {
						if otherRole != role && otherPane != nil && otherPane.ClaudeSessionID == claudeSessionID {
							otherPane.ClaudeSessionID = ""
							hookLog("cleared duplicate claude_session_id=%s from sibling pane role=%s", claudeSessionID, otherRole)
						}
					}
					pane.ClaudeSessionID = claudeSessionID
					configpkg.Save(config)
					hookLog("persisted claude_session_id=%s for team session=%s role=%s", claudeSessionID, sessName, role)
				} else {
					hookLog("persistClaudeSessionID: pane role=%s already has this claude_session_id", role)
				}
			} else {
				hookLog("persistClaudeSessionID: ERROR - pane for role=%s not found in session", role)
			}
			return
		}

		// Fallback: Couldn't infer role from transcript path
		// Check if any pane already has this Claude session ID (for idempotency)
		hasMatch := false
		for role, pane := range sessInfo.Panes {
			if pane != nil && pane.ClaudeSessionID == claudeSessionID {
				hasMatch = true
				hookLog("persistClaudeSessionID: claude_session_id=%s already in role=%s", claudeSessionID, role)
				break
			}
		}

		if hasMatch {
			return // Already persisted, nothing to do
		}

		// Last resort: Try to infer role from tmux pane index
		// Team sessions use fixed pane indices: 1=planner, 2=executor, 3=reviewer
		// We query tmux for the active pane in the session window
		role = inferRoleFromTmuxPane(sessName)
		if role != "" {
			if pane, exists := sessInfo.Panes[role]; exists && pane != nil && pane.ClaudeSessionID == "" {
				pane.ClaudeSessionID = claudeSessionID
				configpkg.Save(config)
				hookLog("persistClaudeSessionID: FALLBACK - stored claude_session_id=%s in role=%s using tmux pane index", claudeSessionID, role)
				return
			}
		}

		// Final fallback: Store in first empty pane (unreliable, but better than losing the ID)
		hookLog("persistClaudeSessionID: WARNING - could not infer role from transcript=%s or tmux, using random fallback", transcriptPath)
		for role, pane := range sessInfo.Panes {
			if pane != nil && pane.ClaudeSessionID == "" {
				pane.ClaudeSessionID = claudeSessionID
				hookLog("persistClaudeSessionID: LAST RESORT - stored claude_session_id=%s in random role=%s (INCORRECT)", claudeSessionID, role)
				break
			}
		}
		configpkg.Save(config)
		return
	}

	// For single sessions, use the original logic
	info, exists := config.Sessions[sessName]
	if !exists || info == nil {
		return
	}

	// Always check for and clear duplicate claude_session_id values, even if the current session
	// already has this ID. This handles the case where duplicates are created via /session command.
	duplicateCleared := false
	for otherName, otherInfo := range config.Sessions {
		if otherName == sessName {
			continue
		}
		if otherInfo != nil && otherInfo.ClaudeSessionID == claudeSessionID {
			otherInfo.ClaudeSessionID = ""
			hookLog("cleared duplicate claude_session_id=%s from session=%s", claudeSessionID, otherName)
			duplicateCleared = true
		}
	}

	// Only save if the ID actually changed OR we cleared a duplicate
	if info.ClaudeSessionID != claudeSessionID {
		info.ClaudeSessionID = claudeSessionID
		configpkg.Save(config)
		hookLog("persisted claude_session_id=%s for session=%s", claudeSessionID, sessName)
	} else if duplicateCleared {
		configpkg.Save(config)
		hookLog("persisted claude_session_id=%s for session=%s (duplicate cleared)", claudeSessionID, sessName)
	}
}
