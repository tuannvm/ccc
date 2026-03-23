package interpane

import (
	"testing"

	"github.com/tuannvm/ccc/session"
)

func TestParseMentions(t *testing.T) {
	router := NewRouter()

	tests := []struct {
		name     string
		text     string
		wantLen  int
		wantRoles []session.PaneRole
	}{
		{
			name:     "no mentions",
			text:     "This is a normal response without any mentions.",
			wantLen:  0,
			wantRoles: nil,
		},
		{
			name: "single mention at start",
			text: `@executor please run the tests`,
			wantLen: 1,
			wantRoles: []session.PaneRole{session.RoleExecutor},
		},
		{
			name: "multiple mentions",
			text: `@executor please run the tests

@reviewer please review my changes`,
			wantLen: 2,
			wantRoles: []session.PaneRole{session.RoleExecutor, session.RoleReviewer},
		},
		{
			name: "mention with multi-line message",
			text: `@executor please do the following:
1. Run the tests
2. Check the output
3. Report back`,
			wantLen: 1,
			wantRoles: []session.PaneRole{session.RoleExecutor},
		},
		{
			name: "all three roles",
			text: `@planner create a plan
@executor implement it
@reviewer check the work`,
			wantLen: 3,
			wantRoles: []session.PaneRole{session.RolePlanner, session.RoleExecutor, session.RoleReviewer},
		},
		{
			name: "case insensitive matching",
			text: `@EXECUTOR please run tests
@Reviewer check code`,
			wantLen: 2,
			wantRoles: []session.PaneRole{session.RoleExecutor, session.RoleReviewer},
		},
		{
			name: "mention with surrounding text (on new lines)",
			text: `I've completed the analysis.
@planner please refine this approach.

The implementation is ready.
@executor please proceed.`,
			wantLen: 2,
			wantRoles: []session.PaneRole{session.RolePlanner, session.RoleExecutor},
		},
		{
			name: "mention followed by another mention",
			text: `@executor task 1
@planner task 2`,
			wantLen: 2,
			wantRoles: []session.PaneRole{session.RoleExecutor, session.RolePlanner},
		},
		{
			name: "@mention not at start of line should not match",
			text: `Please ask @executor to run the tests`,
			wantLen: 0,
			wantRoles: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For testing, use empty request ID and 0 hop count
			mentions := router.ParseMentions(tt.text, "test-request-id", 0)

			if len(mentions) != tt.wantLen {
				t.Errorf("ParseMentions() returned %d mentions, want %d", len(mentions), tt.wantLen)
			}

			if tt.wantRoles != nil {
				for i, wantRole := range tt.wantRoles {
					if i >= len(mentions) {
						break
					}
					if mentions[i].Role != wantRole {
						t.Errorf("ParseMentions()[%d].Role = %v, want %v", i, mentions[i].Role, wantRole)
					}
				}
			}
		})
	}
}

func TestParseMentions_MessageContent(t *testing.T) {
	router := NewRouter()

	text := `@executor please run the tests and report back`

	mentions := router.ParseMentions(text, "test-request-id", 0)

	if len(mentions) != 1 {
		t.Fatalf("ParseMentions() returned %d mentions, want 1", len(mentions))
	}

	expectedMsg := "please run the tests and report back"
	if mentions[0].Message != expectedMsg {
		t.Errorf("ParseMentions()[0].Message = %q, want %q", mentions[0].Message, expectedMsg)
	}
}

func TestInferRoleFromPaneID(t *testing.T) {
	router := NewRouter()

	tests := []struct {
		paneID  string
		wantRole session.PaneRole
	}{
		{"%0", session.RolePlanner},
		{"%1", session.RoleExecutor},
		{"%2", session.RoleReviewer},
		{"%3", session.RoleStandard}, // Unknown index defaults to standard
		{"invalid", session.RoleStandard}, // Invalid format defaults to standard
	}

	for _, tt := range tests {
		t.Run(tt.paneID, func(t *testing.T) {
			got := router.inferRoleFromPaneID(tt.paneID)
			if got != tt.wantRole {
				t.Errorf("inferRoleFromPaneID(%q) = %v, want %v", tt.paneID, got, tt.wantRole)
			}
		})
	}
}
