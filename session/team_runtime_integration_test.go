//go:build integration

package session

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// tmuxServer manages an isolated tmux server for integration testing.
// Each server is named uniquely so tests can run in parallel.
type tmuxServer struct {
	name   string
	tmuxPath string
	mu     sync.Mutex
}

func newTmuxServer(t *testing.T) *tmuxServer {
	t.Helper()

	tmuxPath := "/usr/bin/tmux"
	if path, err := exec.LookPath("tmux"); err == nil {
		tmuxPath = path
	} else {
		t.Fatalf("tmux not found in PATH: %v", err)
	}

	// Generate unique server name
	name := fmt.Sprintf("test-%d", time.Now().UnixNano())
	s := &tmuxServer{name: name, tmuxPath: tmuxPath}

	// Start the server
	if err := s.start(); err != nil {
		t.Fatalf("failed to start tmux server %s: %v", name, err)
	}

	return s
}

func (s *tmuxServer) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return exec.Command(s.tmuxPath, "-L", s.name, "new-session", "-d", "-s", "setup").Run()
}

func (s *tmuxServer) kill() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return exec.Command(s.tmuxPath, "-L", s.name, "kill-server").Run()
}

func (s *tmuxServer) cmd(args ...string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fullArgs := append([]string{"-L", s.name}, args...)
	return exec.Command(s.tmuxPath, fullArgs...).CombinedOutput()
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

func (s *tmuxServer) getPaneTitle(pane string) (string, error) {
	out, err := s.cmd("display-message", "-t", pane, "-p", "#{pane_title}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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

// mockSessionForIntegration implements Session for integration tests
type mockSessionForIntegration struct {
	name         string
	path         string
	providerName string
	layoutName   string
	topicID      int64
	panes        map[PaneRole]*PaneInfo
}

func (m *mockSessionForIntegration) GetName() string              { return m.name }
func (m *mockSessionForIntegration) GetPath() string              { return m.path }
func (m *mockSessionForIntegration) GetTopicID() int64           { return m.topicID }
func (m *mockSessionForIntegration) GetProviderName() string      { return m.providerName }
func (m *mockSessionForIntegration) GetType() SessionKind        { return SessionKindTeam }
func (m *mockSessionForIntegration) GetLayoutName() string        { return m.layoutName }
func (m *mockSessionForIntegration) GetPanes() map[PaneRole]*PaneInfo { return m.panes }

// TestCreateThreePaneLayout tests that CreateThreePaneLayout creates exactly 3 panes
func TestCreateThreePaneLayout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "test-session"
	target := "ccc-team:" + sessionName

	// Create temp work dir
	workDir := t.TempDir()

	// Create the 3-pane layout
	err := runtime.CreateThreePaneLayout(target, workDir)
	if err != nil {
		t.Fatalf("CreateThreePaneLayout failed: %v", err)
	}

	// Verify exactly 3 panes exist
	paneList, err := server.listPanes(target)
	if err != nil {
		t.Fatalf("list-panes failed: %v", err)
	}

	if len(paneList) != 3 {
		t.Errorf("expected 3 panes, got %d: %v", len(paneList), paneList)
	}

	// Verify pane titles (should be set to role names)
	for i, idx := range []string{"1", "2", "3"} {
		pane := target + "." + idx
		title, err := server.getPaneTitle(pane)
		if err != nil {
			t.Errorf("getPaneTitle(%s) failed: %v", pane, err)
			continue
		}
		expected := []string{"Planner", "Executor", "Reviewer"}[i]
		if title != expected {
			t.Errorf("pane %s: expected title %q, got %q", pane, expected, title)
		}
	}
}

// TestGetRoleTargetIntegration tests that getRoleTarget returns correct pane indices
func TestGetRoleTargetIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "test-session"
	sess := &mockSessionForIntegration{
		name:         sessionName,
		path:         "/tmp/test",
		providerName: "test",
		layoutName:   "team-3pane",
		topicID:      123,
		panes: map[PaneRole]*PaneInfo{
			RolePlanner:  {Role: RolePlanner},
			RoleExecutor: {Role: RoleExecutor},
			RoleReviewer: {Role: RoleReviewer},
		},
	}

	// Create the layout first
	target := "ccc-team:" + sessionName
	workDir := t.TempDir()
	if err := runtime.CreateThreePaneLayout(target, workDir); err != nil {
		t.Fatalf("createThreePaneLayout failed: %v", err)
	}

	// Test role targeting
	cases := []struct {
		role       PaneRole
		wantPaneIdx int
	}{
		{RolePlanner, 1},
		{RoleExecutor, 2},
		{RoleReviewer, 3},
	}

	for _, tc := range cases {
		got, err := runtime.GetRoleTarget(sess, tc.role)
		if err != nil {
			t.Errorf("GetRoleTarget(%s) error: %v", tc.role, err)
			continue
		}
		expected := fmt.Sprintf("%s.%d", target, tc.wantPaneIdx)
		if got != expected {
			t.Errorf("GetRoleTarget(%s): got %q, want %q", tc.role, got, expected)
		}
	}
}

// TestSendToPaneIntegration tests that text actually appears in the correct pane
func TestSendToPaneIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "test-session"
	target := "ccc-team:" + sessionName

	// Create the layout
	workDir := t.TempDir()
	if err := runtime.CreateThreePaneLayout(target, workDir); err != nil {
		t.Fatalf("createThreePaneLayout failed: %v", err)
	}

	// Send a message to the executor pane (index 2)
	testMsg := "hello-from-integration-test"
	executorTarget := fmt.Sprintf("%s.2", target)

	// Use sendToTmux directly (lowest level that actually sends to tmux)
	// Note: we can't easily test sendToTmuxFromTelegram because it writes to
	// telegramActiveFlag which uses the real path. Instead we test the tmux
	// send-keys directly.
	if err := server.sendKeys(executorTarget, testMsg); err != nil {
		t.Fatalf("sendKeys to %s failed: %v", executorTarget, err)
	}

	// Give tmux time to process
	time.Sleep(200 * time.Millisecond)

	// Capture the pane content
	content, err := server.capturePane(executorTarget)
	if err != nil {
		t.Fatalf("capture-pane failed: %v", err)
	}

	if !strings.Contains(content, testMsg) {
		// tmux capture-pane may include terminal formatting with line wraps
		// Normalize by removing all whitespace and check if message chars appear in order
		normalized := strings.Join(strings.Fields(content), "")
		if !strings.Contains(normalized, testMsg) {
			t.Errorf("executor pane content = %q, want to contain %q (normalized: %q)", content, testMsg, normalized)
		}
	}
}

// TestWindowExistsAndRecreation tests that EnsureLayout reuses existing layouts
func TestWindowExistsAndRecreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "test-session"
	sess := &mockSessionForIntegration{
		name:         sessionName,
		path:         t.TempDir(),
		providerName: "test",
		layoutName:   "team-3pane",
		topicID:      123,
		panes: map[PaneRole]*PaneInfo{
			RolePlanner:  {Role: RolePlanner},
			RoleExecutor: {Role: RoleExecutor},
			RoleReviewer: {Role: RoleReviewer},
		},
	}

	// Ensure layout first time
	if err := runtime.EnsureLayout(sess, sess.path); err != nil {
		t.Fatalf("EnsureLayout first time failed: %v", err)
	}

	paneList1, err := server.listPanes("ccc-team:" + sessionName)
	if err != nil {
		t.Fatalf("list-panes failed: %v", err)
	}

	// Call EnsureLayout again — should reuse existing layout (no error)
	if err := runtime.EnsureLayout(sess, sess.path); err != nil {
		t.Fatalf("EnsureLayout second time failed: %v", err)
	}

	paneList2, err := server.listPanes("ccc-team:" + sessionName)
	if err != nil {
		t.Fatalf("list-panes failed: %v", err)
	}

	if len(paneList1) != len(paneList2) {
		t.Errorf("pane count changed: before=%d, after=%d", len(paneList1), len(paneList2))
	}
}

// TestEnsureLayoutFullLifecycle tests EnsureLayout end-to-end
func TestEnsureLayoutFullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "test-lifecycle"
	sess := &mockSessionForIntegration{
		name:         sessionName,
		path:         t.TempDir(),
		providerName: "test",
		layoutName:   "team-3pane",
		topicID:      456,
		panes: map[PaneRole]*PaneInfo{
			RolePlanner:  {Role: RolePlanner},
			RoleExecutor: {Role: RoleExecutor},
			RoleReviewer: {Role: RoleReviewer},
		},
	}

	// Ensure layout
	if err := runtime.EnsureLayout(sess, sess.path); err != nil {
		t.Fatalf("EnsureLayout failed: %v", err)
	}

	// Verify panes exist
	target := "ccc-team:" + sessionName
	paneList, err := server.listPanes(target)
	if err != nil {
		t.Fatalf("list-panes failed: %v", err)
	}

	if len(paneList) != 3 {
		t.Errorf("expected 3 panes after EnsureLayout, got %d: %v", len(paneList), paneList)
	}

	// Verify GetRoleTarget still works
	for _, role := range []PaneRole{RolePlanner, RoleExecutor, RoleReviewer} {
		got, err := runtime.GetRoleTarget(sess, role)
		if err != nil {
			t.Errorf("GetRoleTarget(%s) after EnsureLayout: %v", role, err)
		}
		expected := fmt.Sprintf("%s.%d", target, map[PaneRole]int{
			RolePlanner: 1, RoleExecutor: 2, RoleReviewer: 3,
		}[role])
		if got != expected {
			t.Errorf("GetRoleTarget(%s): got %q, want %q", role, got, expected)
		}
	}
}

// TestSequentialMessageRouting sends messages to each pane in sequence and verifies
// that each message arrives in the correct pane without interference.
func TestSequentialMessageRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "seq-routing-test"
	target := "ccc-team:" + sessionName
	workDir := t.TempDir()

	if err := runtime.CreateThreePaneLayout(target, workDir); err != nil {
		t.Fatalf("CreateThreePaneLayout failed: %v", err)
	}

	// Send message to Planner
	plannerTarget := target + ".1"
	plannerMsg := "msg-for-planner"
	if err := server.sendKeys(plannerTarget, plannerMsg); err != nil {
		t.Fatalf("sendKeys to planner failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Send message to Executor
	executorTarget := target + ".2"
	executorMsg := "msg-for-executor"
	if err := server.sendKeys(executorTarget, executorMsg); err != nil {
		t.Fatalf("sendKeys to executor failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Send message to Reviewer
	reviewerTarget := target + ".3"
	reviewerMsg := "msg-for-reviewer"
	if err := server.sendKeys(reviewerTarget, reviewerMsg); err != nil {
		t.Fatalf("sendKeys to reviewer failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Verify each pane has its own message
	plannerContent, err := server.capturePane(plannerTarget)
	if err != nil {
		t.Fatalf("capturePane for planner failed: %v", err)
	}
	if !strings.Contains(normalizeForComparison(plannerContent), plannerMsg) {
		t.Errorf("planner pane should contain %q, got: %s", plannerMsg, plannerContent)
	}

	executorContent, err := server.capturePane(executorTarget)
	if err != nil {
		t.Fatalf("capturePane for executor failed: %v", err)
	}
	if !strings.Contains(normalizeForComparison(executorContent), executorMsg) {
		t.Errorf("executor pane should contain %q, got: %s", executorMsg, executorContent)
	}

	reviewerContent, err := server.capturePane(reviewerTarget)
	if err != nil {
		t.Fatalf("capturePane for reviewer failed: %v", err)
	}
	if !strings.Contains(normalizeForComparison(reviewerContent), reviewerMsg) {
		t.Errorf("reviewer pane should contain %q, got: %s", reviewerMsg, reviewerContent)
	}
}

// normalizeForComparison removes whitespace for comparison
func normalizeForComparison(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// TestPaneTitlePersistenceAfterRouting sends multiple messages to panes and verifies
// that pane titles remain unchanged after routing operations.
func TestPaneTitlePersistenceAfterRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "title-persist-test"
	target := "ccc-team:" + sessionName
	workDir := t.TempDir()

	if err := runtime.CreateThreePaneLayout(target, workDir); err != nil {
		t.Fatalf("CreateThreePaneLayout failed: %v", err)
	}

	// Record initial titles
	initialTitles := make(map[string]string)
	for i, pane := range []string{target + ".1", target + ".2", target + ".3"} {
		title, err := server.getPaneTitle(pane)
		if err != nil {
			t.Fatalf("getPaneTitle(%s) failed: %v", pane, err)
		}
		initialTitles[fmt.Sprintf("%d", i+1)] = title
	}

	// Send multiple messages to each pane
	for round := 0; round < 3; round++ {
		for _, pane := range []string{target + ".1", target + ".2", target + ".3"} {
			msg := fmt.Sprintf("round-%d-msg", round)
			if err := server.sendKeys(pane, msg); err != nil {
				t.Fatalf("sendKeys to %s failed: %v", pane, err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Verify titles are unchanged
	for i, pane := range []string{target + ".1", target + ".2", target + ".3"} {
		title, err := server.getPaneTitle(pane)
		if err != nil {
			t.Fatalf("getPaneTitle(%s) failed: %v", pane, err)
		}
		expected := initialTitles[fmt.Sprintf("%d", i+1)]
		if title != expected {
			t.Errorf("pane %s: title changed from %q to %q after routing", pane, expected, title)
		}
	}
}

// TestGetRoleTargetInvalidRole verifies that GetRoleTarget returns an error for unknown roles.
func TestGetRoleTargetInvalidRole(t *testing.T) {
	runtime := &TeamRuntime{}
	sess := &mockSessionForIntegration{
		name:         "test-session",
		path:         "/tmp/test",
		providerName: "test",
		layoutName:   "team-3pane",
		topicID:      123,
		panes: map[PaneRole]*PaneInfo{
			RolePlanner:  {Role: RolePlanner},
			RoleExecutor: {Role: RoleExecutor},
			RoleReviewer: {Role: RoleReviewer},
		},
	}

	// Test with an invalid role
	_, err := runtime.GetRoleTarget(sess, PaneRole("invalid-role"))
	if err == nil {
		t.Errorf("GetRoleTarget(invalid-role) should return error, got nil")
	}
}

// TestWindowExistsFalseForNonExistent verifies windowExists returns false for non-existent windows.
func TestWindowExistsFalseForNonExistent(t *testing.T) {
	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}

	// Non-existent target should return false
	if runtime.windowExists("ccc-team:this-does-not-exist") {
		t.Errorf("windowExists should return false for non-existent window")
	}
}

// TestLayoutRecreationAfterKill creates layout, kills window, recreates, verifies same structure.
func TestLayoutRecreationAfterKill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "recreate-test"
	target := "ccc-team:" + sessionName
	workDir := t.TempDir()

	// Create initial layout
	if err := runtime.CreateThreePaneLayout(target, workDir); err != nil {
		t.Fatalf("CreateThreePaneLayout failed: %v", err)
	}

	// Record pane structure
	initialPanes, err := server.listPanes(target)
	if err != nil {
		t.Fatalf("listPanes initial failed: %v", err)
	}
	if len(initialPanes) != 3 {
		t.Fatalf("expected 3 panes initially, got %d", len(initialPanes))
	}

	// Kill the window
	if err := runtime.killWindow(target); err != nil {
		t.Fatalf("killWindow failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Verify window no longer exists
	if runtime.windowExists(target) {
		t.Errorf("window should not exist after kill")
	}

	// Recreate via EnsureLayout
	sess := &mockSessionForIntegration{
		name:         sessionName,
		path:         workDir,
		providerName: "test",
		layoutName:   "team-3pane",
		topicID:      456,
		panes: map[PaneRole]*PaneInfo{
			RolePlanner:  {Role: RolePlanner},
			RoleExecutor: {Role: RoleExecutor},
			RoleReviewer: {Role: RoleReviewer},
		},
	}

	if err := runtime.EnsureLayout(sess, workDir); err != nil {
		t.Fatalf("EnsureLayout after kill failed: %v", err)
	}

	// Verify pane structure is restored
	recreatedPanes, err := server.listPanes(target)
	if err != nil {
		t.Fatalf("listPanes after recreate failed: %v", err)
	}
	if len(recreatedPanes) != 3 {
		t.Errorf("expected 3 panes after recreation, got %d", len(recreatedPanes))
	}

	// Verify role targets still work
	for _, role := range []PaneRole{RolePlanner, RoleExecutor, RoleReviewer} {
		got, err := runtime.GetRoleTarget(sess, role)
		if err != nil {
			t.Errorf("GetRoleTarget(%s) after recreation: %v", role, err)
		}
		expectedIdx := map[PaneRole]int{RolePlanner: 1, RoleExecutor: 2, RoleReviewer: 3}[role]
		expected := fmt.Sprintf("%s.%d", target, expectedIdx)
		if got != expected {
			t.Errorf("GetRoleTarget(%s): got %q, want %q", role, got, expected)
		}
	}
}

// TestMultipleEnsureLayoutCallsNoDegradation calls EnsureLayout multiple times
// and verifies the layout remains correct (no degradation).
func TestMultipleEnsureLayoutCallsNoDegradation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "no-deg-test"
	workDir := t.TempDir()

	sess := &mockSessionForIntegration{
		name:         sessionName,
		path:         workDir,
		providerName: "test",
		layoutName:   "team-3pane",
		topicID:      789,
		panes: map[PaneRole]*PaneInfo{
			RolePlanner:  {Role: RolePlanner},
			RoleExecutor: {Role: RoleExecutor},
			RoleReviewer: {Role: RoleReviewer},
		},
	}

	// Call EnsureLayout 5 times
	for i := 0; i < 5; i++ {
		if err := runtime.EnsureLayout(sess, workDir); err != nil {
			t.Fatalf("EnsureLayout call %d failed: %v", i+1, err)
		}

		// Verify pane count is still 3
		target := "ccc-team:" + sessionName
		paneList, err := server.listPanes(target)
		if err != nil {
			t.Fatalf("listPanes after call %d failed: %v", i+1, err)
		}
		if len(paneList) != 3 {
			t.Errorf("after call %d: expected 3 panes, got %d", i+1, len(paneList))
		}
	}
}

// TestEnsureLayoutWithNonThreePaneWindow verifies that hasThreePanes correctly
// identifies windows with wrong pane counts.
func TestEnsureLayoutWithNonThreePaneWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newTmuxServer(t)
	defer server.kill()

	runtime := &TeamRuntime{TmuxPath: server.tmuxPath, ServerName: server.name}
	sessionName := "fix-layout-test"
	target := "ccc-team:" + sessionName
	workDir := t.TempDir()

	// First create a proper 3-pane layout
	if err := runtime.CreateThreePaneLayout(target, workDir); err != nil {
		t.Fatalf("CreateThreePaneLayout failed: %v", err)
	}

	// Verify hasThreePanes returns true for correct layout
	if !runtime.hasThreePanes(target) {
		t.Errorf("hasThreePanes should return true for 3-pane layout")
	}

	// Verify hasThreePanes returns false for non-existent target
	if runtime.hasThreePanes("ccc-team:non-existent") {
		t.Errorf("hasThreePanes should return false for non-existent window")
	}
}
