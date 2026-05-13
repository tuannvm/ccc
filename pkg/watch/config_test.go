package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadJiraConfigMissingFile(t *testing.T) {
	_, err := LoadJiraConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "jira watcher config not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadJiraConfigMissingRequiredFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jira.json")
	if err := os.WriteFile(path, []byte(`{"base_url":"https://jira.example.com"}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadJiraConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, field := range []string{"auth_env_var", "jql", "repo_field", "claim_transition or claim_status"} {
		if !strings.Contains(err.Error(), field) {
			t.Fatalf("error %q missing %q", err.Error(), field)
		}
	}
}

func TestLoadJiraConfigReadsAuthTokenFromEnv(t *testing.T) {
	t.Setenv("CCC_TEST_JIRA_TOKEN", "token-123")
	path := filepath.Join(t.TempDir(), "jira.json")
	data := `{
		"base_url":"https://jira.example.com",
		"auth_env_var":"CCC_TEST_JIRA_TOKEN",
		"poll_interval":"30s",
		"jql":"project = ABC",
		"claim_status":"In Progress",
		"repo_field":"customfield_10001"
	}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadJiraConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthToken != "token-123" {
		t.Fatalf("AuthToken = %q", cfg.AuthToken)
	}
	if cfg.MaxTicketsPerCycle != 1 {
		t.Fatalf("MaxTicketsPerCycle = %d, want 1", cfg.MaxTicketsPerCycle)
	}
}

func TestLoadJiraConfigReadsBasicAuthEmailFromEnv(t *testing.T) {
	t.Setenv("CCC_TEST_JIRA_TOKEN", "token-123")
	t.Setenv("CCC_TEST_JIRA_EMAIL", "user@example.com")
	path := filepath.Join(t.TempDir(), "jira.json")
	data := `{
		"base_url":"https://jira.example.com",
		"auth_method":"basic",
		"auth_env_var":"CCC_TEST_JIRA_TOKEN",
		"auth_email_env_var":"CCC_TEST_JIRA_EMAIL",
		"jql":"project = ABC",
		"claim_status":"In Progress",
		"repo_field":"customfield_10001"
	}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadJiraConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthMethod != "basic" || cfg.AuthEmail != "user@example.com" || cfg.AuthToken != "token-123" {
		t.Fatalf("cfg auth = method:%q email:%q token:%q", cfg.AuthMethod, cfg.AuthEmail, cfg.AuthToken)
	}
}

func TestLoadJiraConfigReadsRepoFallbackFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("CCC_TEST_JIRA_TOKEN=token-123\nCCC_TEST_REPO=/tmp/fallback-repo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "jira.json")
	data := fmt.Sprintf(`{
		"base_url":"https://jira.example.com",
		"auth_env_var":"CCC_TEST_JIRA_TOKEN",
		"env_file":%q,
		"jql":"project = ABC",
		"claim_status":"In Progress",
		"repo_field":"customfield_10001",
		"repo_fallback_env_var":"CCC_TEST_REPO"
	}`, envPath)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadJiraConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthToken != "token-123" || cfg.RepoFallback != "/tmp/fallback-repo" {
		t.Fatalf("cfg token=%q fallback=%q", cfg.AuthToken, cfg.RepoFallback)
	}
}
