package config

// SessionInfo stores information about a session
type SessionInfo struct {
	TopicID         int64  `json:"topic_id"`
	Path            string `json:"path"`
	ClaudeSessionID string `json:"claude_session_id,omitempty"`
	WindowID        string `json:"window_id,omitempty"`     // tmux window ID (@N)
	ProviderName    string `json:"provider_name,omitempty"` // Provider to use for this session
	IsWorktree      bool   `json:"is_worktree,omitempty"`   // Whether this is a worktree session
	WorktreeName    string `json:"worktree_name,omitempty"` // Name of the worktree
	BaseSession     string `json:"base_session,omitempty"`  // Base session name for worktree
}

// ProviderConfig configures Claude provider (API keys, models, etc.)
type ProviderConfig struct {
	// API settings
	AuthToken  string `json:"auth_token,omitempty"`   // API key/token
	AuthEnvVar string `json:"auth_env_var,omitempty"` // Env var to read auth token from (e.g., "MY_API_KEY")
	BaseURL    string `json:"base_url,omitempty"`     // API base URL
	ApiTimeout int    `json:"api_timeout,omitempty"`  // API timeout in milliseconds

	// Model overrides
	OpusModel     string `json:"opus_model,omitempty"`
	SonnetModel   string `json:"sonnet_model,omitempty"`
	HaikuModel    string `json:"haiku_model,omitempty"`
	SubagentModel string `json:"subagent_model,omitempty"`

	// Config directory for this provider (supports ~ expansion)
	ConfigDir string `json:"config_dir,omitempty"`
}

// Config stores bot configuration and session mappings
type Config struct {
	BotToken          string                       `json:"bot_token"`
	ChatID            int64                        `json:"chat_id"`                      // Private chat for simple commands
	GroupID           int64                        `json:"group_id,omitempty"`           // Group with topics for sessions
	Sessions          map[string]*SessionInfo      `json:"sessions,omitempty"`           // session name -> session info
	ProjectsDir       string                       `json:"projects_dir,omitempty"`       // Base directory for new projects (default: ~)
	TranscriptionLang string                       `json:"transcription_lang,omitempty"` // Language code for whisper (e.g. "es", "en")
	RelayURL          string                       `json:"relay_url,omitempty"`          // Relay server URL for large file transfers
	Away              bool                         `json:"away"`
	OAuthToken        string                       `json:"oauth_token,omitempty"`
	OTPSecret         string                       `json:"otp_secret,omitempty"`      // TOTP secret for safe mode
	ActiveProvider    string                       `json:"active_provider,omitempty"` // Which provider to use from providers map
	Providers         map[string]*ProviderConfig   `json:"providers,omitempty"`       // Named provider configurations
	Provider          *ProviderConfig              `json:"provider,omitempty"`        // Deprecated: Use providers + active_provider
}
