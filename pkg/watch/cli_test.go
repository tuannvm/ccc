package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFromArgsUnknownWatcherType(t *testing.T) {
	err := RunFromArgs([]string{"linear"})
	if err == nil || !strings.Contains(err.Error(), "unknown watcher type") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunPollFromArgsUsesJiraConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	err := RunPollFromArgs([]string{"--dry-run"})
	if err == nil || !strings.Contains(err.Error(), "jira watcher config not found") {
		t.Fatalf("err = %v output=%q", err, out.String())
	}
}

func TestRunJiraFromArgsMissingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	err := RunJiraFromArgs(context.Background(), []string{"--once", "--dry-run"}, &out)
	if err == nil || !strings.Contains(err.Error(), "jira watcher config not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunJiraFromArgsOnceDryRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		writeJSON(t, w, map[string]any{"issues": []any{
			map[string]any{
				"id":  "1",
				"key": "ABC-10",
				"fields": map[string]any{
					"summary":           "Dry run",
					"customfield_10001": "/tmp/repo",
				},
			},
		}})
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CCC_TEST_JIRA_TOKEN", "token")
	configDir := filepath.Join(home, ".config", "ccc")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	data := map[string]any{
		"base_url":              server.URL,
		"auth_env_var":          "CCC_TEST_JIRA_TOKEN",
		"jql":                   "project = ABC",
		"claim_status":          "In Progress",
		"repo_field":            "customfield_10001",
		"poll_interval":         "1m",
		"max_tickets_per_cycle": 1,
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "jira.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunJiraFromArgs(context.Background(), []string{"--once", "--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ABC-10 Dry run") {
		t.Fatalf("output = %q", out.String())
	}
}
