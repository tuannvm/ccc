package lookup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/tmux"
	"github.com/tuannvm/ccc/pkg/session"
)

// Session matching priority constants
const (
	priorityExactPath   = 0 // Exact path match (highest priority)
	priorityPrefixMatch = 1 // Parent directory prefix match
)

// sessionMatch represents a potential session match with its priority
type sessionMatch struct {
	name     string
	info     *config.SessionInfo
	priority int
}

// GetSessionWorkDir returns the correct working directory for a session.
// For worktree sessions, this returns the base repository path (not the .claude/worktrees path).
// For regular sessions, this returns the session's stored Path.
func GetSessionWorkDir(cfg *config.Config, sessionName string, sessionInfo *config.SessionInfo) string {
	if sessionInfo == nil {
		if cfg != nil && cfg.Sessions != nil {
			sessionInfo = cfg.Sessions[sessionName]
		}
		if sessionInfo == nil {
			return config.ResolveProjectPath(cfg, sessionName)
		}
	}

	// For worktree sessions, use the base session's path
	if sessionInfo.IsWorktree && sessionInfo.BaseSession != "" {
		if cfg != nil && cfg.Sessions != nil {
			if baseInfo := cfg.Sessions[sessionInfo.BaseSession]; baseInfo != nil && baseInfo.Path != "" {
				return baseInfo.Path
			}
		}
		// Fallback: derive from worktree path (remove .claude/worktrees/<name>/ suffix)
		worktreePath := sessionInfo.Path
		if strings.HasSuffix(worktreePath, "/.claude/worktrees/"+sessionInfo.WorktreeName) {
			return strings.TrimSuffix(worktreePath, "/.claude/worktrees/"+sessionInfo.WorktreeName)
		}
	}

	// For regular sessions, use the stored Path
	if sessionInfo.Path != "" {
		return sessionInfo.Path
	}

	// Backward compatibility: check old default path ($HOME/<sessionName>)
	home, _ := os.UserHomeDir()
	oldPath := filepath.Join(home, sessionName)
	if _, err := os.Stat(oldPath); err == nil {
		return oldPath
	}

	return config.ResolveProjectPath(cfg, sessionName)
}

// FindSessionForPath finds the best matching session for a given directory path.
// Uses deterministic selection: exact path match first, then longest prefix match.
func FindSessionForPath(cfg *config.Config, cwd string) (string, *config.SessionInfo) {
	if cfg == nil || cfg.Sessions == nil {
		return "", nil
	}

	var matches []sessionMatch
	for name, info := range cfg.Sessions {
		if info == nil {
			continue
		}
		if cwd == info.Path {
			matches = append(matches, sessionMatch{name: name, info: info, priority: priorityExactPath})
		} else if info.Path != "" && strings.HasPrefix(cwd, info.Path+"/") {
			matches = append(matches, sessionMatch{name: name, info: info, priority: priorityPrefixMatch})
		}
	}

	if len(matches) == 0 {
		return "", nil
	}

	bestMatch := matches[0]
	for _, m := range matches {
		if m.priority < bestMatch.priority {
			bestMatch = m
		} else if m.priority == priorityPrefixMatch && bestMatch.priority == priorityPrefixMatch {
			if len(m.info.Path) > len(bestMatch.info.Path) {
				bestMatch = m
			}
		}
	}

	return bestMatch.name, bestMatch.info
}

// GetCurrentTmuxWindowName returns the current tmux window name, or empty string if not in tmux.
func GetCurrentTmuxWindowName() string {
	if tmux.TmuxPath == "" {
		return ""
	}
	cmd := exec.Command(tmux.TmuxPath, "display-message", "-p", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FindSessionByWindowName finds a session by its tmux window name.
// Handles tmux name sanitization where dots are replaced with "__".
// Prefers exact matches over sanitized matches to avoid collision ambiguity.
// Checks both Sessions and TeamSessions.
func FindSessionByWindowName(cfg *config.Config, windowName string) (string, int64) {
	if windowName == "" {
		return "", 0
	}
	// First pass: look for exact match in regular sessions (highest priority)
	for name, info := range cfg.Sessions {
		if name == "" || info == nil {
			continue
		}
		if name == windowName {
			return name, info.TopicID
		}
	}
	// First pass: look for exact match in team sessions
	if cfg.TeamSessions != nil {
		for tid, info := range cfg.TeamSessions {
			if info == nil {
				continue
			}
			if info.SessionName == windowName {
				return info.SessionName, tid
			}
		}
	}
	// Second pass: look for sanitized match in regular sessions (lower priority)
	var sanitizedMatch string
	var sanitizedTopicID int64
	for name, info := range cfg.Sessions {
		if name == "" || info == nil {
			continue
		}
		if tmux.SafeName(name) == windowName {
			if sanitizedMatch != "" {
				hooks.HookLog("WARNING: Ambiguous window name '%s' matches multiple sessions: %s, %s",
					windowName, sanitizedMatch, name)
				return "", 0
			}
			sanitizedMatch = name
			sanitizedTopicID = info.TopicID
		}
	}
	// Second pass: look for sanitized match in team sessions
	if cfg.TeamSessions != nil {
		for tid, info := range cfg.TeamSessions {
			if info == nil {
				continue
			}
			if tmux.SafeName(info.SessionName) == windowName {
				if sanitizedMatch != "" {
					hooks.HookLog("WARNING: Ambiguous window name '%s' matches multiple sessions: %s, %s",
						windowName, sanitizedMatch, info.SessionName)
					return "", 0
				}
				sanitizedMatch = info.SessionName
				sanitizedTopicID = tid
			}
		}
	}
	return sanitizedMatch, sanitizedTopicID
}

// GetSessionByTopic finds a session name by its Telegram topic ID.
// Checks both Sessions and TeamSessions.
func GetSessionByTopic(cfg *config.Config, topicID int64) string {
	for name, info := range cfg.Sessions {
		if info != nil && info.TopicID == topicID {
			return name
		}
	}
	if cfg.TeamSessions != nil {
		for tid, info := range cfg.TeamSessions {
			if info != nil && tid == topicID {
				return info.SessionName
			}
		}
	}
	return ""
}

// GetSessionInfo resolves SessionInfo from either Sessions or TeamSessions.
func GetSessionInfo(cfg *config.Config, sessionName string, topicID int64) *config.SessionInfo {
	if info, ok := cfg.Sessions[sessionName]; ok && info != nil {
		return info
	}
	if cfg.TeamSessions != nil {
		if info, ok := cfg.TeamSessions[topicID]; ok && info != nil {
			return info
		}
	}
	return nil
}

// FindSessionByClaudeID matches a claude session ID to a configured session.
func FindSessionByClaudeID(cfg *config.Config, claudeSessionID string) (string, int64) {
	if claudeSessionID == "" {
		return "", 0
	}
	currentWindowName := GetCurrentTmuxWindowName()
	if currentWindowName != "" {
		if info, exists := cfg.Sessions[currentWindowName]; exists && info != nil && info.ClaudeSessionID == claudeSessionID {
			return currentWindowName, info.TopicID
		}
		if cfg.TeamSessions != nil {
			for tid, info := range cfg.TeamSessions {
				if info != nil && info.SessionName == currentWindowName {
					if info.Panes != nil {
						for _, pane := range info.Panes {
							if pane != nil && pane.ClaudeSessionID == claudeSessionID {
								return info.SessionName, tid
							}
						}
					}
				}
			}
		}
		var sanitizedMatch string
		var sanitizedTopicID int64
		for name, info := range cfg.Sessions {
			if info == nil || info.ClaudeSessionID != claudeSessionID {
				continue
			}
			if tmux.SafeName(name) == currentWindowName {
				if sanitizedMatch != "" {
					hooks.HookLog("WARNING: Ambiguous claude_session_id '%s' and window '%s' matches multiple sessions: %s, %s",
						claudeSessionID, currentWindowName, sanitizedMatch, name)
					return "", 0
				}
				sanitizedMatch = name
				sanitizedTopicID = info.TopicID
			}
		}
		if cfg.TeamSessions != nil {
			for tid, info := range cfg.TeamSessions {
				if info == nil {
					continue
				}
				if info.Panes != nil {
					for _, pane := range info.Panes {
						if pane != nil && pane.ClaudeSessionID == claudeSessionID {
							if tmux.SafeName(info.SessionName) == currentWindowName {
								if sanitizedMatch != "" {
									hooks.HookLog("WARNING: Ambiguous claude_session_id '%s' and window '%s' matches multiple sessions: %s, %s",
										claudeSessionID, currentWindowName, sanitizedMatch, info.SessionName)
									return "", 0
								}
								sanitizedMatch = info.SessionName
								sanitizedTopicID = tid
							}
						}
					}
				}
			}
		}
		if sanitizedMatch != "" {
			return sanitizedMatch, sanitizedTopicID
		}
	}
	for name, info := range cfg.Sessions {
		if name == "" || info == nil {
			continue
		}
		if info.ClaudeSessionID == claudeSessionID {
			return name, info.TopicID
		}
	}
	if cfg.TeamSessions != nil {
		for tid, info := range cfg.TeamSessions {
			if info == nil {
				continue
			}
			if info.Panes != nil {
				for _, pane := range info.Panes {
					if pane != nil && pane.ClaudeSessionID == claudeSessionID {
						return info.SessionName, tid
					}
				}
			}
		}
	}
	return "", 0
}

// FindSessionByCwd matches a hook's cwd to a configured session (fallback).
func FindSessionByCwd(cfg *config.Config, cwd string) (string, int64) {
	for name, info := range cfg.Sessions {
		if name == "" || info == nil {
			continue
		}
		if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") || strings.HasSuffix(cwd, "/"+name) {
			return name, info.TopicID
		}
	}
	if cfg.TeamSessions != nil {
		for tid, info := range cfg.TeamSessions {
			if info == nil {
				continue
			}
			if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") {
				return info.SessionName, tid
			}
		}
	}
	return "", 0
}

// FindSession matches by claude_session_id first, then tmux window name, then falls back to cwd.
func FindSession(cfg *config.Config, cwd string, claudeSessionID string) (string, int64) {
	if name, topicID := FindSessionByClaudeID(cfg, claudeSessionID); name != "" {
		return name, topicID
	}
	if windowName := GetCurrentTmuxWindowName(); windowName != "" {
		if name, topicID := FindSessionByWindowName(cfg, windowName); name != "" {
			return name, topicID
		}
	}
	return FindSessionByCwd(cfg, cwd)
}

// GetWorktreeNames returns a set of existing worktree names at basePath.
// Returns nil if the worktrees directory doesn't exist.
func GetWorktreeNames(basePath string) map[string]bool {
	worktreesDir := filepath.Join(basePath, ".claude", "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil
	}

	names := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			names[entry.Name()] = true
		}
	}
	return names
}

// WaitForNewWorktree polls for a new worktree directory to be created.
// Takes a snapshot of existing worktrees and waits for a new one to appear.
// Returns the new worktree name or empty string if timeout occurs.
func WaitForNewWorktree(basePath string, existingNames map[string]bool, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		currentNames := GetWorktreeNames(basePath)
		if currentNames == nil {
			time.Sleep(pollInterval)
			continue
		}

		var newName string
		for name := range currentNames {
			if !existingNames[name] {
				newName = name
				break
			}
		}

		if newName != "" {
			time.Sleep(pollInterval)
			confirmNames := GetWorktreeNames(basePath)
			if confirmNames != nil && confirmNames[newName] {
				loggingpkg.ListenLog("[WaitForNewWorktree] Confirmed new worktree: %s", newName)
				return newName
			}
			loggingpkg.ListenLog("[WaitForNewWorktree] Worktree %s not confirmed, continuing to wait", newName)
		}

		time.Sleep(pollInterval)
	}

	return ""
}

// GenerateUniqueSessionName creates a unique session name based on the current directory.
// If the basename collides with an existing session, it uses the parent directory as prefix.
// If that also collides, it appends a counter.
func GenerateUniqueSessionName(cfg *config.Config, cwd string, basename string) string {
	sessionName := basename

	if _, exists := cfg.Sessions[sessionName]; !exists {
		return sessionName
	}

	if info, ok := cfg.Sessions[sessionName]; ok && info != nil && info.Path == cwd {
		return sessionName
	}

	parentDir := filepath.Base(filepath.Dir(cwd))
	sessionName = parentDir + "-" + sessionName

	if _, exists := cfg.Sessions[sessionName]; !exists {
		return sessionName
	}

	counter := 1
	for {
		candidateName := fmt.Sprintf("%s-%d", sessionName, counter)
		if _, exists := cfg.Sessions[candidateName]; !exists {
			return candidateName
		}
		counter++
	}
}

// InferRoleFromTmuxPane determines the role by querying tmux for the active pane.
// Team sessions have panes named: Planner, Executor, Reviewer.
// Falls back to pane index if names are not set: 1=planner, 2=executor, 3=reviewer.
func InferRoleFromTmuxPane(sessionName string) session.PaneRole {
	if tmux.TmuxPath == "" || sessionName == "" {
		return ""
	}
	target := fmt.Sprintf("ccc-team:%s", sessionName)
	cmd := exec.Command(tmux.TmuxPath, "display-message", "-t", target, "-p", "#{pane_name}")
	out, err := cmd.Output()
	if err != nil {
		hooks.HookLog("InferRoleFromTmuxPane: tmux query failed: %v", err)
		return ""
	}
	paneName := strings.TrimSpace(string(out))
	roleMap := map[string]session.PaneRole{
		"Planner":  session.RolePlanner,
		"Executor": session.RoleExecutor,
		"Reviewer": session.RoleReviewer,
	}
	if role, ok := roleMap[paneName]; ok {
		hooks.HookLog("InferRoleFromTmuxPane: determined role=%s from pane name=%s", role, paneName)
		return role
	}
	cmd2 := exec.Command(tmux.TmuxPath, "display-message", "-t", target, "-p", "#{pane_index}")
	out2, err2 := cmd2.Output()
	if err2 != nil {
		hooks.HookLog("InferRoleFromTmuxPane: pane index query failed: %v", err2)
		return ""
	}
	paneIndex := strings.TrimSpace(string(out2))
	indexMap := map[string]session.PaneRole{
		"1": session.RolePlanner,
		"2": session.RoleExecutor,
		"3": session.RoleReviewer,
	}
	if role, ok := indexMap[paneIndex]; ok {
		hooks.HookLog("InferRoleFromTmuxPane: determined role=%s from pane index=%s (fallback)", role, paneIndex)
		return role
	}
	hooks.HookLog("InferRoleFromTmuxPane: unknown pane name=%s or index=%s", paneName, paneIndex)
	return ""
}

// PersistClaudeSessionID saves the claude session ID to config if changed.
// For single sessions, stores in SessionInfo.ClaudeSessionID.
// For team sessions, stores in the matching pane's PaneInfo.ClaudeSessionID.
// It also clears the same claude_session_id from any other session/pane to prevent ambiguity.
func PersistClaudeSessionID(cfg *config.Config, sessName string, claudeSessionID string, transcriptPath string) {
	if claudeSessionID == "" || sessName == "" {
		return
	}

	var sessInfo *config.SessionInfo
	var isTeam bool

	for name, info := range cfg.Sessions {
		if info != nil && name == sessName {
			sessInfo = info
			break
		}
	}

	if sessInfo == nil {
		for _, info := range cfg.TeamSessions {
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

	if isTeam && sessInfo.Panes != nil {
		hooks.HookLog("PersistClaudeSessionID: team session=%s, claudeSessionID=%s, transcript=%s", sessName, claudeSessionID, transcriptPath)

		role := session.InferRoleFromTranscriptPath(transcriptPath)
		if role != "" {
			hooks.HookLog("PersistClaudeSessionID: inferred role=%s from transcript path", role)
			if pane, exists := sessInfo.Panes[role]; exists && pane != nil {
				if pane.ClaudeSessionID != claudeSessionID {
					for otherRole, otherPane := range sessInfo.Panes {
						if otherRole != role && otherPane != nil && otherPane.ClaudeSessionID == claudeSessionID {
							otherPane.ClaudeSessionID = ""
							hooks.HookLog("cleared duplicate claude_session_id=%s from sibling pane role=%s", claudeSessionID, otherRole)
						}
					}
					pane.ClaudeSessionID = claudeSessionID
					config.Save(cfg)
					hooks.HookLog("persisted claude_session_id=%s for team session=%s role=%s", claudeSessionID, sessName, role)
				} else {
					hooks.HookLog("PersistClaudeSessionID: pane role=%s already has this claude_session_id", role)
				}
			} else {
				hooks.HookLog("PersistClaudeSessionID: ERROR - pane for role=%s not found in session", role)
			}
			return
		}

		hasMatch := false
		for role, pane := range sessInfo.Panes {
			if pane != nil && pane.ClaudeSessionID == claudeSessionID {
				hasMatch = true
				hooks.HookLog("PersistClaudeSessionID: claude_session_id=%s already in role=%s", claudeSessionID, role)
				break
			}
		}

		if hasMatch {
			return
		}

		role = InferRoleFromTmuxPane(sessName)
		if role != "" {
			if pane, exists := sessInfo.Panes[role]; exists && pane != nil && pane.ClaudeSessionID == "" {
				pane.ClaudeSessionID = claudeSessionID
				config.Save(cfg)
				hooks.HookLog("PersistClaudeSessionID: FALLBACK - stored claude_session_id=%s in role=%s using tmux pane index", claudeSessionID, role)
				return
			}
		}

		hooks.HookLog("PersistClaudeSessionID: WARNING - could not infer role from transcript=%s or tmux, using random fallback", transcriptPath)
		for role, pane := range sessInfo.Panes {
			if pane != nil && pane.ClaudeSessionID == "" {
				pane.ClaudeSessionID = claudeSessionID
				hooks.HookLog("PersistClaudeSessionID: LAST RESORT - stored claude_session_id=%s in random role=%s (INCORRECT)", claudeSessionID, role)
				break
			}
		}
		config.Save(cfg)
		return
	}

	info, exists := cfg.Sessions[sessName]
	if !exists || info == nil {
		return
	}

	duplicateCleared := false
	for otherName, otherInfo := range cfg.Sessions {
		if otherName == sessName {
			continue
		}
		if otherInfo != nil && otherInfo.ClaudeSessionID == claudeSessionID {
			otherInfo.ClaudeSessionID = ""
			hooks.HookLog("cleared duplicate claude_session_id=%s from session=%s", claudeSessionID, otherName)
			duplicateCleared = true
		}
	}

	if info.ClaudeSessionID != claudeSessionID {
		info.ClaudeSessionID = claudeSessionID
		config.Save(cfg)
		hooks.HookLog("persisted claude_session_id=%s for session=%s", claudeSessionID, sessName)
	} else if duplicateCleared {
		config.Save(cfg)
		hooks.HookLog("persisted claude_session_id=%s for session=%s (duplicate cleared)", claudeSessionID, sessName)
	}
}


// GetSessionContext returns commonly needed session attributes in a single call.
// This avoids repeating the same IsWorktree/ClaudeSessionID/ProviderName pattern across handlers.
func GetSessionContext(info *config.SessionInfo) (worktreeName, resumeSessionID, providerName string) {
	if info == nil {
		return "", "", ""
	}
	if info.IsWorktree {
		worktreeName = info.WorktreeName
	}
	resumeSessionID = info.ClaudeSessionID
	providerName = info.ProviderName
	return
}
