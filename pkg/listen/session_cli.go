package listen

import (
	"fmt"
	"os"
	"sort"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/lookup"
	"github.com/tuannvm/ccc/pkg/tmux"
)

type cliSessionRow struct {
	Name         string
	TopicID      int64
	Provider     string
	Source       string
	Path         string
	Conversation string
	State        string
	Kind         string
	Current      bool
}

// RunStatusFromArgs handles terminal-facing session status and management commands.
func RunStatusFromArgs(args []string) error {
	cfg, err := configpkg.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cmd := "current"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "", "current", "show":
		fmt.Print(BuildCurrentSessionStatus(cfg, currentWorkingDir()))
		return nil
	case "all":
		fmt.Print(BuildCLISessionsList(cfg, currentWorkingDir()))
		return nil
	case "restart":
		return RestartCurrentSession(cfg, currentWorkingDir())
	case "attach":
		if len(args) < 2 {
			return fmt.Errorf("usage: ccc status attach <session-name>")
		}
		return AttachSessionByName(cfg, args[1])
	default:
		return AttachSessionByName(cfg, cmd)
	}
}

// AttachSessionByName attaches the terminal to an existing CCC session.
func AttachSessionByName(cfg *configpkg.Config, sessionName string) error {
	if cfg == nil {
		return fmt.Errorf("config unavailable")
	}
	if info := cfg.Sessions[sessionName]; info != nil {
		return AttachToExistingSession(cfg, sessionName, info, "")
	}
	if cfg.TeamSessions != nil {
		for _, info := range cfg.TeamSessions {
			if info != nil && info.SessionName == sessionName {
				return fmt.Errorf("team session %q uses the team tmux layout; run 'ccc team attach %s'", sessionName, sessionName)
			}
		}
	}
	return fmt.Errorf("session %q not found; run 'ccc status all' to list known sessions", sessionName)
}

func RestartCurrentSession(cfg *configpkg.Config, cwd string) error {
	sessionName, info := lookup.FindSessionForPath(cfg, cwd)
	if sessionName == "" || info == nil {
		return fmt.Errorf("no session mapped to %s; run 'ccc status all' to pick a session", displayPath(cwd))
	}
	return AttachToExistingSession(cfg, sessionName, info, "")
}

func BuildCurrentSessionStatus(cfg *configpkg.Config, cwd string) string {
	if cfg == nil {
		return "ccc status: config unavailable\n"
	}
	sessionName, info := lookup.FindSessionForPath(cfg, cwd)
	if sessionName == "" || info == nil {
		return fmt.Sprintf("ccc status: no session mapped to %s\nrun 'ccc' here to create or attach one, or 'ccc status all' to pick an existing session\n", displayPath(cwd))
	}
	row := buildCLISessionRow(cfg, sessionName, info, cwd)
	return formatCLISessionDetail(row)
}

func BuildCLISessionsList(cfg *configpkg.Config, cwd string) string {
	if cfg == nil {
		return "ccc status: config unavailable\n"
	}

	rows := cliSessionRows(cfg, cwd)
	if len(rows) == 0 {
		return "ccc status: no sessions\nrun 'ccc' in a project directory or create one from Telegram with /new <name>\n"
	}

	var lines []string
	lines = append(lines, "ccc status all")
	lines = append(lines, "")
	for _, row := range rows {
		marker := " "
		if row.Current {
			marker = "*"
		}
		line := fmt.Sprintf("%s %s", marker, row.Name)
		if row.Kind != "" && row.Kind != "session" {
			line += fmt.Sprintf(" [%s]", row.Kind)
		}
		line += fmt.Sprintf("  %s  %s", row.State, row.Provider)
		if row.TopicID != 0 {
			line += fmt.Sprintf("  topic:%d", row.TopicID)
		}
		if row.Conversation != "" {
			line += fmt.Sprintf("  conversation:%s", row.Conversation)
		}
		lines = append(lines, line)
		if row.Path != "" {
			lines = append(lines, fmt.Sprintf("    %s", displayPath(row.Path)))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "attach: ccc status attach <session>  |  team: ccc team attach <session>  |  current: ccc status")
	return strings.Join(lines, "\n") + "\n"
}

func cliSessionRows(cfg *configpkg.Config, cwd string) []cliSessionRow {
	var rows []cliSessionRow
	for name, info := range cfg.Sessions {
		if name == "" || info == nil {
			continue
		}
		rows = append(rows, buildCLISessionRow(cfg, name, info, cwd))
	}
	if cfg.TeamSessions != nil {
		for topicID, info := range cfg.TeamSessions {
			if info == nil {
				continue
			}
			name := getSessionNameFromInfo(info)
			row := buildCLISessionRow(cfg, name, info, cwd)
			row.TopicID = topicID
			row.Kind = "team"
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Current != rows[j].Current {
			return rows[i].Current
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func buildCLISessionRow(cfg *configpkg.Config, name string, info *configpkg.SessionInfo, cwd string) cliSessionRow {
	path := lookup.GetSessionWorkDir(cfg, name, info)
	if path == "" && info != nil {
		path = info.Path
	}
	state := "stopped"
	if target, err := tmux.FindExistingWindow(name); err == nil && target != "" {
		if tmux.WindowHasAgentRunning(target, "", effectiveProviderName(cfg, info)) {
			state = "running"
		} else {
			state = "ready"
		}
	}
	kind := "session"
	if info != nil && info.IsWorktree {
		kind = "worktree"
	}
	conversation := ""
	if info != nil && info.ClaudeSessionID != "" {
		conversation = shortSessionID(info.ClaudeSessionID)
	}
	current := false
	if cwd != "" && path != "" {
		current = cwd == path || strings.HasPrefix(cwd, path+string(os.PathSeparator))
	}
	return cliSessionRow{
		Name:         name,
		TopicID:      info.TopicID,
		Provider:     effectiveProviderName(cfg, info),
		Source:       providerSource(cfg, info),
		Path:         path,
		Conversation: conversation,
		State:        state,
		Kind:         kind,
		Current:      current,
	}
}

func formatCLISessionDetail(row cliSessionRow) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("session: %s", row.Name))
	lines = append(lines, fmt.Sprintf("provider: %s", row.Provider))
	lines = append(lines, fmt.Sprintf("source: %s", row.Source))
	lines = append(lines, fmt.Sprintf("state: %s", row.State))
	if row.TopicID != 0 {
		lines = append(lines, fmt.Sprintf("telegram topic: %d", row.TopicID))
	}
	if row.Path != "" {
		lines = append(lines, fmt.Sprintf("path: %s", displayPath(row.Path)))
	}
	if row.Conversation != "" {
		lines = append(lines, fmt.Sprintf("conversation: %s", row.Conversation))
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("attach: ccc status attach %s", row.Name))
	lines = append(lines, "restart: ccc status restart")
	return strings.Join(lines, "\n") + "\n"
}

func currentWorkingDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}
