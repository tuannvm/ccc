package config

import (
	"strings"

	"github.com/tuannvm/ccc/pkg/session"
)

// ========== Session Types ==========

// SessionInfo stores information about a session
type SessionInfo struct {
	TopicID         int64  `json:"topic_id"`
	Path            string `json:"path"`
	SessionName     string `json:"session_name,omitempty"`    // User-provided session name (for team sessions)
	ClaudeSessionID string `json:"claude_session_id,omitempty"`
	WindowID        string `json:"window_id,omitempty"`     // tmux window ID (@N)
	ProviderName    string `json:"provider_name,omitempty"` // Provider to use for this session
	IsWorktree      bool   `json:"is_worktree,omitempty"`   // Whether this is a worktree session
	WorktreeName    string `json:"worktree_name,omitempty"` // Name of the worktree
	BaseSession     string `json:"base_session,omitempty"`  // Base session name for worktree

	// Multi-pane support (for team sessions)
	Type            session.SessionKind            `json:"type,omitempty"`           // "single" or "team"
	LayoutName      string                         `json:"layout_name,omitempty"`     // "single", "team-3pane"
	DefaultPaneID   string                         `json:"default_pane_id,omitempty"` // Default input pane ID
	Panes           map[session.PaneRole]*PaneInfo `json:"panes,omitempty"`           // role -> pane info
}

// PaneInfo stores information about a single pane in a session
// This is the main package version of session.PaneInfo to avoid circular imports
type PaneInfo struct {
	ClaudeSessionID string              `json:"claude_session_id,omitempty"` // Claude session ID for this pane
	PaneID          string              `json:"pane_id,omitempty"`           // Tmux pane ID (%1, %2, etc.)
	Role            session.PaneRole    `json:"role"`                        // Role of this pane
}

// ========== Session Interface Implementation ==========

// Ensure SessionInfo implements session.Session interface
var _ session.Session = (*SessionInfo)(nil)

// GetName returns the session name (derived from path for now)
func (s *SessionInfo) GetName() string {
	// For team sessions, use the SessionName field
	if s.SessionName != "" {
		return s.SessionName
	}
	// Fallback to path basename for backward compatibility
	if idx := strings.LastIndex(s.Path, "/"); idx >= 0 {
		return s.Path[idx+1:]
	}
	return s.Path
}

// GetPath returns the working directory path
func (s *SessionInfo) GetPath() string {
	return s.Path
}

// GetTopicID returns the Telegram topic ID
func (s *SessionInfo) GetTopicID() int64 {
	return s.TopicID
}

// GetProviderName returns the provider name for this session
func (s *SessionInfo) GetProviderName() string {
	return s.ProviderName
}

// GetType returns the session kind (single or team)
func (s *SessionInfo) GetType() session.SessionKind {
	if s.Type == "" {
		return session.SessionKindSingle // Default to single
	}
	return s.Type
}

// GetLayoutName returns the layout name for this session
func (s *SessionInfo) GetLayoutName() string {
	if s.LayoutName == "" {
		return "single" // Default to single
	}
	return s.LayoutName
}

// GetPanes returns the panes map for this session
func (s *SessionInfo) GetPanes() map[session.PaneRole]*session.PaneInfo {
	// Convert main.PaneInfo to session.PaneInfo
	result := make(map[session.PaneRole]*session.PaneInfo)
	for role, info := range s.Panes {
		result[role] = &session.PaneInfo{
			ClaudeSessionID: info.ClaudeSessionID,
			PaneID:          info.PaneID,
			Role:            info.Role,
		}
	}
	return result
}

// ========== Provider Types ==========

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

// ========== Config Structure ==========

// Config stores bot configuration and session mappings
type Config struct {
	// ========== Telegram Integration ==========
	BotToken      string `json:"bot_token"`
	ChatID        int64  `json:"chat_id"`                    // Private chat for simple commands
	GroupID       int64  `json:"group_id,omitempty"`         // Group with topics for sessions
	MultiUserMode bool   `json:"multi_user_mode,omitempty"`  // Allow any group member (default: false = owner only)
	// API 9.5: Custom emoji IDs for forum topic icons (optional)
	CustomEmojiIDs map[string]string `json:"custom_emoji_ids,omitempty"` // provider -> emoji_id (e.g., "zai": "5372874709367178364")
	// API 9.5: Enable streaming responses for real-time typing effect
	EnableStreaming bool `json:"enable_streaming,omitempty"` // Use sendMessageDraft for AI responses

	// ========== Sessions ==========
	Sessions map[string]*SessionInfo `json:"sessions,omitempty"` // session name -> session info (single-pane sessions)

	// ========== Team Sessions ==========
	TeamSessions map[int64]*SessionInfo `json:"team_sessions,omitempty"` // topic ID -> session info (multi-pane team sessions)

	// ========== User Preferences ==========
	ProjectsDir       string `json:"projects_dir,omitempty"`       // Base directory for new projects (default: ~)
	TranscriptionLang string `json:"transcription_lang,omitempty"` // Language code for whisper (e.g. "es", "en")
	RelayURL          string `json:"relay_url,omitempty"`          // Relay server URL for large file transfers
	Away              bool   `json:"away"`

	// ========== Authentication ==========
	OAuthToken string `json:"oauth_token,omitempty"`
	OTPSecret string `json:"otp_secret,omitempty"` // TOTP secret for safe mode

	// ========== AI Providers ==========
	ActiveProvider string                     `json:"active_provider,omitempty"` // Which provider to use from providers map
	Providers     map[string]*ProviderConfig `json:"providers,omitempty"`       // Named provider configurations
	Provider      *ProviderConfig            `json:"provider,omitempty"`        // Deprecated: Use providers + active_provider
}

type coreConfig struct {
	BotToken         string            `json:"bot_token"`
	ChatID           int64             `json:"chat_id"`
	GroupID          int64             `json:"group_id,omitempty"`
	MultiUserMode    bool              `json:"multi_user_mode,omitempty"`
	CustomEmojiIDs   map[string]string `json:"custom_emoji_ids,omitempty"`
	EnableStreaming  bool              `json:"enable_streaming,omitempty"`
	ProjectsDir      string            `json:"projects_dir,omitempty"`
	TranscriptionLang string           `json:"transcription_lang,omitempty"`
	RelayURL         string            `json:"relay_url,omitempty"`
	Away             bool              `json:"away"`
	OAuthToken       string            `json:"oauth_token,omitempty"`
	OTPSecret        string            `json:"otp_secret,omitempty"`
}

type sessionsConfig struct {
	Sessions     map[string]*SessionInfo `json:"sessions,omitempty"`
	TeamSessions map[int64]*SessionInfo  `json:"team_sessions,omitempty"`
}

type providersConfig struct {
	ActiveProvider string                     `json:"active_provider,omitempty"`
	Providers      map[string]*ProviderConfig `json:"providers,omitempty"`
	Provider       *ProviderConfig            `json:"provider,omitempty"`
}

func (c *Config) CoreConfig() coreConfig {
	return coreConfig{
		BotToken:          c.BotToken,
		ChatID:            c.ChatID,
		GroupID:           c.GroupID,
		MultiUserMode:     c.MultiUserMode,
		CustomEmojiIDs:    c.CustomEmojiIDs,
		EnableStreaming:   c.EnableStreaming,
		ProjectsDir:       c.ProjectsDir,
		TranscriptionLang: c.TranscriptionLang,
		RelayURL:          c.RelayURL,
		Away:              c.Away,
		OAuthToken:        c.OAuthToken,
		OTPSecret:         c.OTPSecret,
	}
}

func (c *Config) SessionsConfig() sessionsConfig {
	return sessionsConfig{
		Sessions:     c.Sessions,
		TeamSessions: c.TeamSessions,
	}
}

func (c *Config) ProvidersConfig() providersConfig {
	return providersConfig{
		ActiveProvider: c.ActiveProvider,
		Providers:      c.Providers,
		Provider:       c.Provider,
	}
}

// IsTeamSession checks if a topic ID corresponds to a team session
func (c *Config) IsTeamSession(topicID int64) bool {
	if c.TeamSessions == nil {
		return false
	}
	_, exists := c.TeamSessions[topicID]
	return exists
}

// GetTeamSession retrieves a team session by topic ID
func (c *Config) GetTeamSession(topicID int64) (*SessionInfo, bool) {
	if c.TeamSessions == nil {
		return nil, false
	}
	sess, exists := c.TeamSessions[topicID]
	return sess, exists
}

// SetTeamSession stores or updates a team session
func (c *Config) SetTeamSession(topicID int64, sess *SessionInfo) {
	if c.TeamSessions == nil {
		c.TeamSessions = make(map[int64]*SessionInfo)
	}
	c.TeamSessions[topicID] = sess
}

// DeleteTeamSession removes a team session by topic ID
func (c *Config) DeleteTeamSession(topicID int64) {
	if c.TeamSessions != nil {
		delete(c.TeamSessions, topicID)
	}
}
