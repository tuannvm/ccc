package watch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

type JiraConfig struct {
	BaseURL                 string `json:"base_url"`
	AuthEnvVar              string `json:"auth_env_var"`
	AuthEmailEnvVar         string `json:"auth_email_env_var,omitempty"`
	AuthMethod              string `json:"auth_method,omitempty"`
	EnvFile                 string `json:"env_file,omitempty"`
	PollInterval            string `json:"poll_interval,omitempty"`
	JQL                     string `json:"jql"`
	ClaimTransition         string `json:"claim_transition,omitempty"`
	ClaimStatus             string `json:"claim_status,omitempty"`
	RepoField               string `json:"repo_field"`
	RepoFallbackEnvVar      string `json:"repo_fallback_env_var,omitempty"`
	AcceptanceCriteriaField string `json:"acceptance_criteria_field,omitempty"`
	MaxTicketsPerCycle      int    `json:"max_tickets_per_cycle,omitempty"`

	AuthToken    string `json:"-"`
	AuthEmail    string `json:"-"`
	RepoFallback string `json:"-"`
}

func DefaultJiraConfigPath() string {
	return filepath.Join(configpkg.ConfigDir(), "jira.json")
}

func DefaultEnvPath() string {
	return filepath.Join(configpkg.ConfigDir(), ".env")
}

func LoadJiraConfig(path string) (*JiraConfig, error) {
	if path == "" {
		path = DefaultJiraConfigPath()
	}
	var cfg JiraConfig
	if data, err := os.ReadFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if !hasJiraEnvConfig() {
				return nil, fmt.Errorf("jira watcher config not found at %s; create it with base_url, auth_env_var, jql, repo_field, and claim_transition or claim_status", path)
			}
		} else {
			return nil, fmt.Errorf("read jira watcher config: %w", err)
		}
	} else if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse jira watcher config %s: %w", path, err)
	}
	if err := applyJiraEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	if cfg.MaxTicketsPerCycle == 0 {
		cfg.MaxTicketsPerCycle = 1
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	envFile := cfg.EnvFile
	if envFile == "" {
		envFile = DefaultEnvPath()
	}
	fileEnv, err := loadEnvFile(envFile)
	if err != nil {
		return nil, err
	}
	token := cfg.AuthToken
	if strings.TrimSpace(token) == "" {
		token = lookupEnvValue(cfg.AuthEnvVar, fileEnv)
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("jira auth env var %s is not set", cfg.AuthEnvVar)
	}
	cfg.AuthToken = token
	if cfg.AuthEmailEnvVar != "" || cfg.AuthEmail != "" {
		email := strings.TrimSpace(cfg.AuthEmail)
		if email == "" {
			email = strings.TrimSpace(lookupEnvValue(cfg.AuthEmailEnvVar, fileEnv))
		}
		if email == "" {
			return nil, fmt.Errorf("jira auth email env var %s is not set", cfg.AuthEmailEnvVar)
		}
		cfg.AuthEmail = email
	}
	if cfg.RepoFallbackEnvVar != "" && cfg.RepoFallback == "" {
		cfg.RepoFallback = strings.TrimSpace(lookupEnvValue(cfg.RepoFallbackEnvVar, fileEnv))
	}
	if cfg.AuthMethod == "" {
		if cfg.AuthEmailEnvVar != "" || cfg.AuthEmail != "" {
			cfg.AuthMethod = "basic"
		} else {
			cfg.AuthMethod = "bearer"
		}
	}
	return &cfg, nil
}

func hasJiraEnvConfig() bool {
	for _, name := range []string{
		"CCC_JIRA_BASE_URL",
		"CCC_JIRA_AUTH_TOKEN",
		"CCC_JIRA_AUTH_ENV_VAR",
		"CCC_JIRA_JQL",
		"CCC_JIRA_REPO_FIELD",
	} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return true
		}
	}
	return false
}

func applyJiraEnvOverrides(cfg *JiraConfig) error {
	setStringFromEnv(&cfg.BaseURL, "CCC_JIRA_BASE_URL")
	setStringFromEnv(&cfg.AuthToken, "CCC_JIRA_AUTH_TOKEN")
	setStringFromEnv(&cfg.AuthEnvVar, "CCC_JIRA_AUTH_ENV_VAR")
	setStringFromEnv(&cfg.AuthEmail, "CCC_JIRA_AUTH_EMAIL")
	setStringFromEnv(&cfg.AuthEmailEnvVar, "CCC_JIRA_AUTH_EMAIL_ENV_VAR")
	setStringFromEnv(&cfg.AuthMethod, "CCC_JIRA_AUTH_METHOD")
	setStringFromEnv(&cfg.EnvFile, "CCC_JIRA_ENV_FILE")
	setStringFromEnv(&cfg.PollInterval, "CCC_JIRA_POLL_INTERVAL")
	setStringFromEnv(&cfg.JQL, "CCC_JIRA_JQL")
	setStringFromEnv(&cfg.ClaimTransition, "CCC_JIRA_CLAIM_TRANSITION")
	setStringFromEnv(&cfg.ClaimStatus, "CCC_JIRA_CLAIM_STATUS")
	setStringFromEnv(&cfg.RepoField, "CCC_JIRA_REPO_FIELD")
	setStringFromEnv(&cfg.RepoFallback, "CCC_JIRA_REPO_FALLBACK")
	setStringFromEnv(&cfg.RepoFallbackEnvVar, "CCC_JIRA_REPO_FALLBACK_ENV_VAR")
	setStringFromEnv(&cfg.AcceptanceCriteriaField, "CCC_JIRA_ACCEPTANCE_CRITERIA_FIELD")
	if value := strings.TrimSpace(os.Getenv("CCC_JIRA_MAX_TICKETS_PER_CYCLE")); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid CCC_JIRA_MAX_TICKETS_PER_CYCLE %q: %w", value, err)
		}
		cfg.MaxTicketsPerCycle = n
	}
	return nil
}

func setStringFromEnv(dest *string, name string) {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		*dest = value
	}
}

func lookupEnvValue(name string, fileEnv map[string]string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fileEnv[name]
}

func loadEnvFile(path string) (map[string]string, error) {
	values := make(map[string]string)
	data, err := os.ReadFile(configpkg.ExpandPath(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return values, nil
		}
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("parse env file %s:%d: expected KEY=value", path, lineNo+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("parse env file %s:%d: empty key", path, lineNo+1)
		}
		values[key] = unquoteEnvValue(strings.TrimSpace(value))
	}
	return values, nil
}

func unquoteEnvValue(value string) string {
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func (c JiraConfig) PollIntervalDuration() (time.Duration, error) {
	if strings.TrimSpace(c.PollInterval) == "" {
		return time.Minute, nil
	}
	d, err := time.ParseDuration(c.PollInterval)
	if err != nil {
		return 0, fmt.Errorf("invalid jira poll_interval %q: %w", c.PollInterval, err)
	}
	return d, nil
}

func (c JiraConfig) validate() error {
	var missing []string
	if strings.TrimSpace(c.BaseURL) == "" {
		missing = append(missing, "base_url")
	}
	if strings.TrimSpace(c.AuthEnvVar) == "" && strings.TrimSpace(c.AuthToken) == "" {
		missing = append(missing, "auth_env_var")
	}
	if c.AuthMethod != "" && c.AuthMethod != "bearer" && c.AuthMethod != "basic" {
		return fmt.Errorf("jira watcher config auth_method must be either bearer or basic")
	}
	if c.AuthMethod == "basic" && strings.TrimSpace(c.AuthEmailEnvVar) == "" && strings.TrimSpace(c.AuthEmail) == "" {
		missing = append(missing, "auth_email_env_var")
	}
	if strings.TrimSpace(c.JQL) == "" {
		missing = append(missing, "jql")
	}
	if strings.TrimSpace(c.RepoField) == "" {
		missing = append(missing, "repo_field")
	}
	if strings.TrimSpace(c.ClaimTransition) == "" && strings.TrimSpace(c.ClaimStatus) == "" {
		missing = append(missing, "claim_transition or claim_status")
	}
	if len(missing) > 0 {
		return fmt.Errorf("jira watcher config missing required field(s): %s", strings.Join(missing, ", "))
	}
	if c.MaxTicketsPerCycle < 0 {
		return fmt.Errorf("jira watcher config max_tickets_per_cycle must be >= 0")
	}
	return nil
}
