package routing

import (
	"testing"

	"github.com/tuannvm/ccc/session"
)

// TestSinglePaneRouter tests that SinglePaneRouter always returns standard role
func TestSinglePaneRouter(t *testing.T) {
	router := &SinglePaneRouter{}

	tests := []struct {
		name     string
		text     string
		layout   session.LayoutSpec
		wantRole session.PaneRole
		wantText string
	}{
		{
			name:     "simple message",
			text:     "hello world",
			layout:   session.BuiltinLayouts["single"],
			wantRole: session.RoleStandard,
			wantText: "hello world",
		},
		{
			name:     "message with leading whitespace",
			text:     "   hello",
			layout:   session.BuiltinLayouts["single"],
			wantRole: session.RoleStandard,
			wantText: "   hello",
		},
		{
			name:     "empty message",
			text:     "",
			layout:   session.BuiltinLayouts["single"],
			wantRole: session.RoleStandard,
			wantText: "",
		},
		{
			name:     "message that looks like command (ignored in single pane)",
			text:     "/planner do something",
			layout:   session.BuiltinLayouts["single"],
			wantRole: session.RoleStandard,
			wantText: "/planner do something",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, text, err := router.RouteMessage(tt.text, tt.layout)
			if err != nil {
				t.Errorf("RouteMessage() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteMessage() role = %v, want %v", role, tt.wantRole)
			}
			if text != tt.wantText {
				t.Errorf("RouteMessage() text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

// TestTeamRouter tests command prefix routing for team sessions
func TestTeamRouter(t *testing.T) {
	router := &TeamRouter{}
	layout := session.BuiltinLayouts["team-3pane"]

	tests := []struct {
		name     string
		text     string
		wantRole session.PaneRole
		wantText string
	}{
		{
			name:     "planner prefix",
			text:     "/planner create a plan",
			wantRole: session.RolePlanner,
			wantText: "create a plan",
		},
		{
			name:     "@planner mention",
			text:     "@planner create architecture",
			wantRole: session.RolePlanner,
			wantText: "create architecture",
		},
		{
			name:     "executor prefix",
			text:     "/executor run tests",
			wantRole: session.RoleExecutor,
			wantText: "run tests",
		},
		{
			name:     "@executor mention",
			text:     "@executor build",
			wantRole: session.RoleExecutor,
			wantText: "build",
		},
		{
			name:     "reviewer prefix",
			text:     "/reviewer check this",
			wantRole: session.RoleReviewer,
			wantText: "check this",
		},
		{
			name:     "@reviewer mention",
			text:     "@reviewer approve",
			wantRole: session.RoleReviewer,
			wantText: "approve",
		},
		{
			name:     "/plan alias routes to planner",
			text:     "/plan create architecture",
			wantRole: session.RolePlanner,
			wantText: "create architecture",
		},
		{
			name:     "/exec alias routes to executor",
			text:     "/exec build",
			wantRole: session.RoleExecutor,
			wantText: "build",
		},
		{
			name:     "/e alias routes to executor",
			text:     "/e deploy",
			wantRole: session.RoleExecutor,
			wantText: "deploy",
		},
		{
			name:     "/rev alias routes to reviewer",
			text:     "/rev approve",
			wantRole: session.RoleReviewer,
			wantText: "approve",
		},
		{
			name:     "/r alias routes to reviewer",
			text:     "/r comments",
			wantRole: session.RoleReviewer,
			wantText: "comments",
		},
		{
			name:     "/p alias routes to planner",
			text:     "/p design",
			wantRole: session.RolePlanner,
			wantText: "design",
		},
		{
			name:     "no prefix defaults to executor",
			text:     "just do this",
			wantRole: session.RoleExecutor,
			wantText: "just do this",
		},
		{
			name:     "unknown prefix defaults to executor",
			text:     "/unknown command",
			wantRole: session.RoleExecutor,
			wantText: "/unknown command",
		},
		{
			name:     "empty message defaults to executor",
			text:     "",
			wantRole: session.RoleExecutor,
			wantText: "",
		},
		{
			name:     "only whitespace defaults to executor",
			text:     "   ",
			wantRole: session.RoleExecutor,
			wantText: "",
		},
		{
			name:     "prefix with no arguments",
			text:     "/planner",
			wantRole: session.RolePlanner,
			wantText: "",
		},
		{
			name:     "message with leading whitespace before prefix",
			text:     "  /planner test",
			wantRole: session.RolePlanner,
			wantText: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, text, err := router.RouteMessage(tt.text, layout)
			if err != nil {
				t.Errorf("RouteMessage() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteMessage() role = %v, want %v", role, tt.wantRole)
			}
			if text != tt.wantText {
				t.Errorf("RouteMessage() text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

// TestTeamRouterCaseInsensitive tests that prefix matching is case-insensitive
func TestTeamRouterCaseInsensitive(t *testing.T) {
	router := &TeamRouter{}
	layout := session.BuiltinLayouts["team-3pane"]

	tests := []struct {
		name     string
		text     string
		wantRole session.PaneRole
	}{
		{
			name:     "uppercase prefix",
			text:     "/PLANNER do this",
			wantRole: session.RolePlanner,
		},
		{
			name:     "mixed case prefix",
			text:     "/PlAnNeR test",
			wantRole: session.RolePlanner,
		},
		{
			name:     "uppercase @mention",
			text:     "@EXECUTOR run",
			wantRole: session.RoleExecutor,
		},
		{
			name:     "mixed case @mention",
			text:     "@ReViEwEr check",
			wantRole: session.RoleReviewer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, _, err := router.RouteMessage(tt.text, layout)
			if err != nil {
				t.Errorf("RouteMessage() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteMessage() role = %v, want %v", role, tt.wantRole)
			}
		})
	}
}

// TestTeamRouterAtMention tests that @mentions work as prefixes
func TestTeamRouterAtMention(t *testing.T) {
	router := &TeamRouter{}
	layout := session.BuiltinLayouts["team-3pane"]

	tests := []struct {
		name     string
		text     string
		wantRole session.PaneRole
		wantText string
	}{
		{
			name:     "@planner mention",
			text:     "@planner create plan",
			wantRole: session.RolePlanner,
			wantText: "create plan",
		},
		{
			name:     "@executor mention",
			text:     "@executor run tests",
			wantRole: session.RoleExecutor,
			wantText: "run tests",
		},
		{
			name:     "@reviewer mention",
			text:     "@reviewer check code",
			wantRole: session.RoleReviewer,
			wantText: "check code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, text, err := router.RouteMessage(tt.text, layout)
			if err != nil {
				t.Errorf("RouteMessage() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteMessage() role = %v, want %v", role, tt.wantRole)
			}
			if text != tt.wantText {
				t.Errorf("RouteMessage() text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

// TestGetRouter tests that GetRouter returns the correct router type
func TestGetRouter(t *testing.T) {
	tests := []struct {
		name           string
		kind           session.SessionKind
		wantType       string // "team" or "single"
	}{
		{
			name:     "team session returns TeamRouter",
			kind:     session.SessionKindTeam,
			wantType: "team",
		},
		{
			name:     "single session returns SinglePaneRouter",
			kind:     session.SessionKindSingle,
			wantType: "single",
		},
		{
			name:     "unknown session kind defaults to SinglePaneRouter",
			kind:     "unknown",
			wantType: "single",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := GetRouter(tt.kind)
			if tt.wantType == "team" {
				if _, ok := router.(*TeamRouter); !ok {
					t.Errorf("GetRouter() = %T, want *TeamRouter", router)
				}
			} else {
				if _, ok := router.(*SinglePaneRouter); !ok {
					t.Errorf("GetRouter() = %T, want *SinglePaneRouter", router)
				}
			}
		})
	}
}

// TestTeamRouterComplexMessage tests routing with complex message patterns
func TestTeamRouterComplexMessage(t *testing.T) {
	router := &TeamRouter{}
	layout := session.BuiltinLayouts["team-3pane"]

	tests := []struct {
		name     string
		text     string
		wantRole session.PaneRole
		wantText string
	}{
		{
			name:     "multiple words after prefix",
			text:     "/planner create a detailed plan for the API",
			wantRole: session.RolePlanner,
			wantText: "create a detailed plan for the API",
		},
		{
			name:     "message with special characters",
			text:     "/executor run test --verbose",
			wantRole: session.RoleExecutor,
			wantText: "run test --verbose",
		},
		{
			name:     "message with quoted text",
			text:     `/planner say "hello world"`,
			wantRole: session.RolePlanner,
			wantText: `say "hello world"`,
		},
		{
			name:     "message with newlines normalized to spaces",
			text:     "/reviewer check line1\nline2",
			wantRole: session.RoleReviewer,
			wantText: "check line1 line2", // Fields splits on whitespace, Join uses single space
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, text, err := router.RouteMessage(tt.text, layout)
			if err != nil {
				t.Errorf("RouteMessage() error = %v", err)
				return
			}
			if role != tt.wantRole {
				t.Errorf("RouteMessage() role = %v, want %v", role, tt.wantRole)
			}
			if text != tt.wantText {
				t.Errorf("RouteMessage() text = %q, want %q", text, tt.wantText)
			}
		})
	}
}
