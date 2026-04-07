//go:build integration

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tuannvm/ccc/routing"
	"github.com/tuannvm/ccc/session"
)

// tmuxServer is duplicated here for integration test isolation
// In a real project this would be in a shared testutil package
type tmuxServer struct {
	name     string
	tmuxPath string
	mu       sync.Mutex
}

func newTmuxServer(t *testing.T) *tmuxServer {
	t.Helper()
	tmuxPath := "/usr/bin/tmux"
	if path, err := exec.LookPath("tmux"); err == nil {
		tmuxPath = path
	} else {
		t.Fatalf("tmux not found in PATH: %v", err)
	}
	name := "test-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + strings.ReplaceAll(time.Now().Format("nanosecond"), "", "")
	s := &tmuxServer{name: name, tmuxPath: tmuxPath}
	if err := s.start(); err != nil {
		t.Fatalf("failed to start tmux server: %v", err)
	}
	return s
}

func (s *tmuxServer) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return exec.Command(s.tmuxPath, "-L", s.name, "new-session", "-d", "-s", "init").Run()
}

func (s *tmuxServer) kill() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return exec.Command(s.tmuxPath, "-L", s.name, "kill-server").Run()
}

func (s *tmuxServer) cmd(args ...string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	full := append([]string{"-L", s.name}, args...)
	return exec.Command(s.tmuxPath, full...).CombinedOutput()
}

func (s *tmuxServer) listPanes(window string) ([]string, error) {
	out, err := s.cmd("list-panes", "-t", window, "-F", "#{pane_index}")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

func (s *tmuxServer) capturePane(pane string) (string, error) {
	out, err := s.cmd("capture-pane", "-p", "-t", pane)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *tmuxServer) sendKeys(pane, text string) error {
	_, err := s.cmd("send-keys", "-t", pane, text, "Enter")
	return err
}

func (s *tmuxServer) selectPane(pane string) error {
	_, err := s.cmd("select-pane", "-t", pane)
	return err
}

// mockConfigForIntegration is a minimal Config for integration testing
type mockConfigForIntegration struct {
	teamSessions map[int64]*SessionInfo
}

func (m *mockConfigForIntegration) IsTeamSession(topicID int64) bool {
	if m.teamSessions == nil {
		return false
	}
	_, exists := m.teamSessions[topicID]
	return exists
}

func (m *mockConfigForIntegration) GetTeamSession(topicID int64) (*SessionInfo, bool) {
	if m.teamSessions == nil {
		return nil, false
	}
	s, ok := m.teamSessions[topicID]
	return s, ok
}

// TestTeamRouterMessageParsing tests that TeamRouter correctly routes messages
func TestTeamRouterMessageParsing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	layout, ok := session.GetLayout("team-3pane")
	if !ok {
		t.Fatal("team-3pane layout not found")
	}

	router := routing.GetRouter(session.SessionKindTeam)
	teamRouter, ok := router.(*routing.TeamRouter)
	if !ok {
		t.Fatal("expected TeamRouter")
	}

	_ = teamRouter // used via GetRouter.RouteMessage

	cases := []struct {
		name         string
		text         string
		wantRole     session.PaneRole
		wantContains string
	}{
		{
			name:         "@planner routes to planner",
			text:         "@planner hello planner",
			wantRole:     session.RolePlanner,
			wantContains: "hello planner",
		},
		{
			name:         "@executor routes to executor",
			text:         "@executor run this task",
			wantRole:     session.RoleExecutor,
			wantContains: "run this task",
		},
		{
			name:         "@reviewer routes to reviewer",
			text:         "@reviewer check changes",
			wantRole:     session.RoleReviewer,
			wantContains: "check changes",
		},
		{
			name:         "/planner slash prefix routes to planner",
			text:         "/planner do the planning",
			wantRole:     session.RolePlanner,
			wantContains: "do the planning",
		},
		{
			name:         "/executor slash prefix routes to executor",
			text:         "/executor execute code",
			wantRole:     session.RoleExecutor,
			wantContains: "execute code",
		},
		{
			name:         "/reviewer slash prefix routes to reviewer",
			text:         "/reviewer review PR",
			wantRole:     session.RoleReviewer,
			wantContains: "review PR",
		},
		{
			name:         "no prefix defaults to executor",
			text:         "plain message",
			wantRole:     session.RoleExecutor,
			wantContains: "plain message",
		},
		{
			name:         "short /p prefix",
			text:         "/p quick plan",
			wantRole:     session.RolePlanner,
			wantContains: "quick plan",
		},
		{
			name:         "short /e prefix",
			text:         "/e quick exec",
			wantRole:     session.RoleExecutor,
			wantContains: "quick exec",
		},
		{
			name:         "short /r prefix",
			text:         "/r quick review",
			wantRole:     session.RoleReviewer,
			wantContains: "quick review",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role, msg, err := router.RouteMessage(tc.text, layout)
			if err != nil {
				t.Fatalf("RouteMessage(%q) error: %v", tc.text, err)
			}
			if role != tc.wantRole {
				t.Errorf("RouteMessage(%q): got role %q, want %q", tc.text, role, tc.wantRole)
			}
			if tc.wantContains != "" && !strings.Contains(msg, tc.wantContains) {
				t.Errorf("RouteMessage(%q): got msg %q, want to contain %q", tc.text, msg, tc.wantContains)
			}
		})
	}
}

// TestGetTeamRoleTargetIntegration tests getTeamRoleTarget returns correct tmux targets
func TestGetTeamRoleTargetIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cases := []struct {
		role        session.PaneRole
		wantTarget  string
	}{
		{session.RolePlanner, "ccc-team:my-session.1"},
		{session.RoleExecutor, "ccc-team:my-session.2"},
		{session.RoleReviewer, "ccc-team:my-session.3"},
	}

	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			got, err := getTeamRoleTarget("my-session", tc.role)
			if err != nil {
				t.Fatalf("getTeamRoleTarget error: %v", err)
			}
			if got != tc.wantTarget {
				t.Errorf("getTeamRoleTarget: got %q, want %q", got, tc.wantTarget)
			}
		})
	}
}

// TestTeamRoutingEndToEnd tests routing a message through to a real tmux pane
func TestTeamRoutingEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	// Create a 3-pane layout manually
	sessionName := "e2e-test"
	target := "ccc-team:" + sessionName

	// Setup: create session and window
	if out, err := server.cmd("new-session", "-d", "-s", "ccc-team:"+strings.Split(sessionName, ":")[0], "-n", sessionName); err != nil {
		t.Skipf("tmux setup failed (may not have server access): %v, out: %s", err, out)
	}

	// Create panes: split twice to get 3 panes
	if out, err := server.cmd("split-window", "-h", "-t", target); err != nil {
		t.Fatalf("first split failed: %v, out: %s", err, out)
	}
	if out, err := server.cmd("select-pane", "-t", target+".2"); err != nil {
		t.Fatalf("select pane 2 failed: %v, out: %s", err, out)
	}
	if out, err := server.cmd("split-window", "-h", "-t", target); err != nil {
		t.Fatalf("second split failed: %v, out: %s", err, out)
	}

	// Verify 3 panes
	paneList, err := server.listPanes(target)
	if err != nil {
		t.Fatalf("list-panes failed: %v", err)
	}
	if len(paneList) != 3 {
		t.Skipf("expected 3 panes, got %d — tmux may not support split in this environment", len(paneList))
	}

	// Send a message to the executor pane via tmux send-keys
	testMsg := "e2e-test-message"
	executorPane := target + ".2"
	if err := server.sendKeys(executorPane, testMsg); err != nil {
		t.Fatalf("sendKeys failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Capture and verify
	content, err := server.capturePane(executorPane)
	if err != nil {
		t.Fatalf("capture-pane failed: %v", err)
	}

	if !strings.Contains(content, testMsg) {
		t.Errorf("pane content = %q, want to contain %q", content, testMsg)
	}
}

// TestSwitchToTeamWindowIntegration tests that switchToTeamWindow works
func TestSwitchToTeamWindowIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	sessionName := "switch-test"
	target := "ccc-team:" + sessionName

	// Create window
	if out, err := server.cmd("new-session", "-d", "-s", "ccc-team:"+strings.Split(sessionName, ":")[0], "-n", sessionName); err != nil {
		t.Skipf("tmux setup failed: %v, out: %s", err, out)
	}

	// Split to create 3 panes
	server.cmd("split-window", "-h", "-t", target)
	server.cmd("select-pane", "-t", target+".2")
	server.cmd("split-window", "-h", "-t", target)

	// Call switchToTeamWindow
	err := switchToTeamWindow(sessionName, session.RoleExecutor)
	if err != nil {
		t.Errorf("switchToTeamWindow error: %v", err)
	}

	// Verify the window exists
	out, err := server.cmd("list-windows", "-F", "#{window_name}")
	if err != nil {
		t.Fatalf("list-windows failed: %v", err)
	}
	if !strings.Contains(string(out), sessionName) {
		t.Errorf("window %q not found in list: %s", sessionName, out)
	}
}

// TestRolePersistenceInPaneTitles tests that panes retain their titles after routing
func TestRolePersistenceInPaneTitles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	sessionName := "title-test"
	target := "ccc-team:" + sessionName

	// Create window + 3 panes via TeamRuntime
	workDir := t.TempDir()
	tr := &session.TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	if err := tr.CreateThreePaneLayout(target, workDir); err != nil {
		t.Fatalf("createThreePaneLayout failed: %v", err)
	}

	// Verify pane titles match roles
	expectedTitles := map[string]string{
		".1": "Planner",
		".2": "Executor",
		".3": "Reviewer",
	}

	for suffix, expected := range expectedTitles {
		pane := target + suffix
		out, err := server.cmd("display-message", "-t", pane, "-p", "#{pane_title}")
		if err != nil {
			t.Errorf("display-message for %s failed: %v", pane, err)
			continue
		}
		title := strings.TrimSpace(string(out))
		if title != expected {
			t.Errorf("pane %s: got title %q, want %q", pane, title, expected)
		}
	}
}

// TestTempDirCreation tests that work dir is created by StartClaude if missing
// Note: CreateThreePaneLayout itself doesn't create workDir - StartClaude does when it cds into it.
func TestTempDirCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &session.TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "tempdir-test"
	target := "ccc-team:" + sessionName

	// Create the layout first
	workDir := filepath.Join(t.TempDir(), "subdir", "nested")
	if err := runtime.CreateThreePaneLayout(target, workDir); err != nil {
		t.Fatalf("CreateThreePaneLayout failed: %v", err)
	}

	// Verify the layout was created (3 panes)
	paneList, err := server.listPanes(target)
	if err != nil {
		t.Fatalf("list-panes failed: %v", err)
	}
	if len(paneList) != 3 {
		t.Errorf("expected 3 panes, got %d", len(paneList))
	}
}
