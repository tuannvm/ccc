package routing

import (
	"os"
	"testing"

	"github.com/tuannvm/ccc/session"
)

// TestSinglePaneHookRouter tests that SinglePaneHookRouter always returns standard role
func TestSinglePaneHookRouter(t *testing.T) {
	router := &SinglePaneHookRouter{}

	// Create a mock session (we don't actually use it in SinglePaneHookRouter)
	mockSess := &mockSession{}

	tests := []struct {
		name           string
		transcriptPath string
		wantRole       session.PaneRole
	}{
		{
			name:           "empty path",
			transcriptPath: "",
			wantRole:       session.RoleStandard,
		},
		{
			name:           "simple path",
			transcriptPath: "/path/to/transcript.jsonl",
			wantRole:       session.RoleStandard,
		},
		{
			name:           "path with role name (ignored in single pane)",
			transcriptPath: "/path/to/session-planner.jsonl",
			wantRole:       session.RoleStandard,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, err := router.RouteHook(tt.transcriptPath, mockSess)
			if err != nil {
				t.Errorf("RouteHook() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteHook() role = %v, want %v", role, tt.wantRole)
			}
		})
	}
}

// TestTeamHookRouterFromPath tests role inference from transcript path
func TestTeamHookRouterFromPath(t *testing.T) {
	router := &TeamHookRouter{}
	mockSess := &mockSession{}

	tests := []struct {
		name           string
		transcriptPath string
		wantRole       session.PaneRole
	}{
		{
			name:           "planner in filename",
			transcriptPath: "/path/to/session-planner.jsonl",
			wantRole:       session.RolePlanner,
		},
		{
			name:           "executor in filename",
			transcriptPath: "/home/user/.ccc/transcripts/my-project-executor.jsonl",
			wantRole:       session.RoleExecutor,
		},
		{
			name:           "reviewer in filename",
			transcriptPath: "/var/data/team-session-reviewer.jsonl",
			wantRole:       session.RoleReviewer,
		},
		{
			name:           "no role in filename",
			transcriptPath: "/path/to/session.jsonl",
			wantRole:       session.RoleExecutor, // defaults to executor
		},
		{
			name:           "empty path",
			transcriptPath: "",
			wantRole:       session.RoleExecutor, // defaults to executor
		},
		{
			name:           "role in directory name (not matched)",
			transcriptPath: "/planner/session/transcript.jsonl",
			wantRole:       session.RoleExecutor, // only checks basename
		},
		{
			name:           "alias 'exec' defaults to executor (no alias support)",
			transcriptPath: "/path/to/session-exec.jsonl",
			wantRole:       session.RoleExecutor, // defaults to executor
		},
		{
			name:           "alias 'plan' defaults to executor (no alias support)",
			transcriptPath: "/path/to/session-plan.jsonl",
			wantRole:       session.RoleExecutor, // defaults to executor
		},
		{
			name:           "alias 'rev' defaults to executor (no alias support)",
			transcriptPath: "/path/to/session-rev.jsonl",
			wantRole:       session.RoleExecutor, // defaults to executor
		},
		{
			name:           "single-letter 'e' defaults to executor (no alias support)",
			transcriptPath: "/path/to/session-e.jsonl",
			wantRole:       session.RoleExecutor, // defaults to executor
		},
		{
			name:           "single-letter 'r' defaults to executor (no alias support)",
			transcriptPath: "/path/to/session-r.jsonl",
			wantRole:       session.RoleExecutor, // defaults to executor
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, err := router.RouteHook(tt.transcriptPath, mockSess)
			if err != nil {
				t.Errorf("RouteHook() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteHook() role = %v, want %v", role, tt.wantRole)
			}
		})
	}
}

// TestTeamHookRouterFromEnv tests role inference from CCC_ROLE environment variable
func TestTeamHookRouterFromEnv(t *testing.T) {
	router := &TeamHookRouter{}
	mockSess := &mockSession{}

	// Save original value and restore after test
	originalEnv := os.Getenv("CCC_ROLE")
	defer func() {
		if originalEnv == "" {
			os.Unsetenv("CCC_ROLE")
		} else {
			os.Setenv("CCC_ROLE", originalEnv)
		}
	}()

	tests := []struct {
		name           string
		transcriptPath string // empty to skip path inference
		envRole        string
		wantRole       session.PaneRole
	}{
		{
			name:           "CCC_ROLE=planner",
			transcriptPath: "",
			envRole:        "planner",
			wantRole:       session.RolePlanner,
		},
		{
			name:           "CCC_ROLE=executor",
			transcriptPath: "",
			envRole:        "executor",
			wantRole:       session.RoleExecutor,
		},
		{
			name:           "CCC_ROLE=reviewer",
			transcriptPath: "",
			envRole:        "reviewer",
			wantRole:       session.RoleReviewer,
		},
		{
			name:           "CCC_ROLE uppercase (normalized to lowercase)",
			transcriptPath: "",
			envRole:        "PLANNER",
			wantRole:       session.RolePlanner,
		},
		{
			name:           "CCC_ROLE mixed case (normalized to lowercase)",
			transcriptPath: "",
			envRole:        "ExEcUtOr",
			wantRole:       session.RoleExecutor,
		},
		{
			name:           "CCC_ROLE invalid (defaults to executor)",
			transcriptPath: "",
			envRole:        "invalid",
			wantRole:       session.RoleExecutor,
		},
		{
			name:           "CCC_ROLE empty (defaults to executor)",
			transcriptPath: "",
			envRole:        "",
			wantRole:       session.RoleExecutor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envRole == "" {
				os.Unsetenv("CCC_ROLE")
			} else {
				os.Setenv("CCC_ROLE", tt.envRole)
			}

			role, err := router.RouteHook(tt.transcriptPath, mockSess)
			if err != nil {
				t.Errorf("RouteHook() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteHook() role = %v, want %v", role, tt.wantRole)
			}
		})
	}
}

// TestTeamHookRouterPriority tests that transcript path takes priority over env var
func TestTeamHookRouterPriority(t *testing.T) {
	router := &TeamHookRouter{}
	mockSess := &mockSession{}

	// Save and restore original env
	originalEnv := os.Getenv("CCC_ROLE")
	defer func() {
		if originalEnv == "" {
			os.Unsetenv("CCC_ROLE")
		} else {
			os.Setenv("CCC_ROLE", originalEnv)
		}
	}()

	tests := []struct {
		name           string
		transcriptPath string
		envRole        string
		wantRole       session.PaneRole
	}{
		{
			name:           "path takes priority over env",
			transcriptPath: "/path/to/session-planner.jsonl",
			envRole:        "executor",
			wantRole:       session.RolePlanner, // from path, not env
		},
		{
			name:           "env used when path has no role",
			transcriptPath: "/path/to/session.jsonl",
			envRole:        "reviewer",
			wantRole:       session.RoleReviewer, // from env
		},
		{
			name:           "both path and env planner",
			transcriptPath: "/path/to/session-executor.jsonl",
			envRole:        "executor",
			wantRole:       session.RoleExecutor, // path and env agree
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("CCC_ROLE", tt.envRole)

			role, err := router.RouteHook(tt.transcriptPath, mockSess)
			if err != nil {
				t.Errorf("RouteHook() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteHook() role = %v, want %v", role, tt.wantRole)
			}
		})
	}
}

// TestGetHookRouter tests that GetHookRouter returns the correct router type
func TestGetHookRouter(t *testing.T) {
	tests := []struct {
		name     string
		kind     session.SessionKind
		wantType string // "team" or "single"
	}{
		{
			name:     "team session returns TeamHookRouter",
			kind:     session.SessionKindTeam,
			wantType: "team",
		},
		{
			name:     "single session returns SinglePaneHookRouter",
			kind:     session.SessionKindSingle,
			wantType: "single",
		},
		{
			name:     "unknown session kind defaults to SinglePaneHookRouter",
			kind:     "unknown",
			wantType: "single",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := GetHookRouter(tt.kind)
			if tt.wantType == "team" {
				if _, ok := router.(*TeamHookRouter); !ok {
					t.Errorf("GetHookRouter() = %T, want *TeamHookRouter", router)
				}
			} else {
				if _, ok := router.(*SinglePaneHookRouter); !ok {
					t.Errorf("GetHookRouter() = %T, want *SinglePaneHookRouter", router)
				}
			}
		})
	}
}

// mockSession is a minimal implementation of session.Session for testing
type mockSession struct{}

func (m *mockSession) GetName() string                 { return "mock-session" }
func (m *mockSession) GetPath() string                 { return "/mock/path" }
func (m *mockSession) GetTopicID() int64              { return 123 }
func (m *mockSession) GetProviderName() string         { return "test" }
func (m *mockSession) GetType() session.SessionKind   { return session.SessionKindSingle }
func (m *mockSession) GetLayoutName() string          { return "single" }
func (m *mockSession) GetPanes() map[session.PaneRole]*session.PaneInfo {
	return map[session.PaneRole]*session.PaneInfo{
		session.RoleStandard: {Role: session.RoleStandard},
	}
}
