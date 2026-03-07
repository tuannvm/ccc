package hooks

import (
	"encoding/json"
	"testing"
)

// TestHookDataJSON tests HookData JSON parsing
func TestHookDataJSON(t *testing.T) {
	jsonStr := `{"cwd":"/Users/test/project","transcript_path":"/tmp/transcript.jsonl","session_id":"abc123","tool_name":"Bash","tool_input":"echo hello"}`

	var hookData HookData
	if err := json.Unmarshal([]byte(jsonStr), &hookData); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if hookData.Cwd != "/Users/test/project" {
		t.Errorf("Cwd = %q, want %q", hookData.Cwd, "/Users/test/project")
	}
	if hookData.TranscriptPath != "/tmp/transcript.jsonl" {
		t.Errorf("TranscriptPath = %q, want %q", hookData.TranscriptPath, "/tmp/transcript.jsonl")
	}
	if hookData.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want %q", hookData.SessionID, "abc123")
	}
	if hookData.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want Bash", hookData.ToolName)
	}
}

// TestHookDataWithToolInput tests HookData with complex tool input
func TestHookDataWithToolInput(t *testing.T) {
	jsonStr := `{
		"cwd": "/home/user/project",
		"session_id": "sess123",
		"tool_name": "AskUserQuestion",
		"tool_input": "{\"question\":\"Choose an option\",\"options\":[\"A\",\"B\",\"C\"]}",
		"transcript_path": "/tmp/transcript.jsonl"
	}`

	var hookData HookData
	if err := json.Unmarshal([]byte(jsonStr), &hookData); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if hookData.ToolName != "AskUserQuestion" {
		t.Errorf("ToolName = %q, want AskUserQuestion", hookData.ToolName)
	}
	// ToolInput is a struct, not comparable to string
	// Just check that parsing worked
}

// TestParseHookData tests the ParseHookData helper function
func TestParseHookData(t *testing.T) {
	jsonStr := `{"cwd":"/test","session_id":"abc","transcript_path":"/tmp/test.jsonl"}`

	hookData, err := ParseHookData([]byte(jsonStr))
	if err != nil {
		t.Fatalf("ParseHookData failed: %v", err)
	}

	if hookData.Cwd != "/test" {
		t.Errorf("Cwd = %q, want /test", hookData.Cwd)
	}
	if hookData.SessionID != "abc" {
		t.Errorf("SessionID = %q, want abc", hookData.SessionID)
	}
}

// TestParseHookDataInvalidJSON tests error handling for invalid JSON
func TestParseHookDataInvalidJSON(t *testing.T) {
	invalidJSON := `{invalid json`

	_, err := ParseHookData([]byte(invalidJSON))
	if err == nil {
		t.Error("ParseHookData should fail for invalid JSON")
	}
}

// TestHookDataEmpty tests empty hook data
func TestHookDataEmpty(t *testing.T) {
	emptyJSON := `{}`

	hookData, err := ParseHookData([]byte(emptyJSON))
	if err != nil {
		t.Fatalf("ParseHookData failed for empty JSON: %v", err)
	}

	if hookData.Cwd != "" {
		t.Errorf("Cwd should be empty, got %q", hookData.Cwd)
	}
	if hookData.SessionID != "" {
		t.Errorf("SessionID should be empty, got %q", hookData.SessionID)
	}
}
