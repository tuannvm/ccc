package main

import (
	"encoding/json"
	"strings"

	"github.com/tuannvm/ccc/session"
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

// ========== Telegram Types ==========

// TelegramMessage represents a Telegram message
type TelegramMessage struct {
	MessageID       int             `json:"message_id"`
	MessageThreadID int64           `json:"message_thread_id,omitempty"` // Topic ID
	Chat            struct {
		ID   int64  `json:"id"`
		Type string `json:"type"` // "private", "group", "supergroup"
	} `json:"chat"`
	From struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
	SenderTag      string            `json:"sender_tag,omitempty"` // API 9.5: Member tag
	Text           string            `json:"text"`
	ReplyToMessage *TelegramMessage  `json:"reply_to_message,omitempty"`
	Voice          *TelegramVoice    `json:"voice,omitempty"`
	Photo          []TelegramPhoto   `json:"photo,omitempty"`
	Document       *TelegramDocument `json:"document,omitempty"`
	Caption        string            `json:"caption,omitempty"`
}

type TelegramVoice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
}

type TelegramPhoto struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int    `json:"file_size"`
}

type TelegramDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	FileSize int    `json:"file_size"`
}

// CallbackQuery represents a Telegram callback query (button press)
type CallbackQuery struct {
	ID   string `json:"id"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Message *TelegramMessage `json:"message"`
	Data    string           `json:"data"`
}

// TelegramUpdate represents an update from Telegram
type TelegramUpdate struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      []struct {
		UpdateID      int             `json:"update_id"`
		Message       TelegramMessage `json:"message"`
		CallbackQuery *CallbackQuery  `json:"callback_query"`
	} `json:"result"`
}

// TelegramResponse represents a response from Telegram API
type TelegramResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// TopicResult represents the result of creating a forum topic
type TopicResult struct {
	MessageThreadID int64  `json:"message_thread_id"`
	Name            string `json:"name"`
}

// InlineKeyboardButton represents a Telegram inline keyboard button
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// ========== Hook Types ==========

// HookData represents data received from Claude hook
type HookData struct {
	Cwd              string          `json:"cwd"`
	TranscriptPath   string          `json:"transcript_path"`
	SessionID        string          `json:"session_id"`
	HookEventName    string          `json:"hook_event_name"`
	ToolName         string          `json:"tool_name"`
	Prompt           string          `json:"prompt"`            // For UserPromptSubmit hook
	Message          string          `json:"message"`           // For Notification hook
	Title            string          `json:"title"`             // For Notification hook
	NotificationType string          `json:"notification_type"` // For Notification hook
	StopHookActive   bool            `json:"stop_hook_active"`  // For Stop hook
	ToolInputRaw     json.RawMessage `json:"tool_input"`        // Raw tool input JSON
	ToolInput        HookToolInput   `json:"-"`                 // Parsed from ToolInputRaw
}

// HookToolInput holds parsed tool input for known tool types
type HookToolInput struct {
	Questions []struct {
		Question    string `json:"question"`
		Header      string `json:"header"`
		MultiSelect bool   `json:"multiSelect"`
		Options     []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
	} `json:"questions"`
	Command     string `json:"command,omitempty"`     // For Bash
	Description string `json:"description,omitempty"` // For Bash/Task
	FilePath    string `json:"file_path,omitempty"`   // For Read/Write/Edit
	Query       string `json:"query,omitempty"`       // For WebSearch
	Pattern     string `json:"pattern,omitempty"`     // For Grep/Glob
	URL         string `json:"url,omitempty"`         // For WebFetch
	Prompt      string `json:"prompt,omitempty"`      // For Task/WebFetch
	OldString   string `json:"old_string,omitempty"`  // For Edit
}

// parseHookData unmarshals raw JSON and populates ToolInput
func parseHookData(data []byte) (HookData, error) {
	var hd HookData
	if err := json.Unmarshal(data, &hd); err != nil {
		return hd, err
	}
	if len(hd.ToolInputRaw) > 0 {
		json.Unmarshal(hd.ToolInputRaw, &hd.ToolInput)
	}
	return hd, nil
}

// ========== Ledger Types ==========

// MessageRecord tracks the delivery state of a single message
type MessageRecord struct {
	ID                string `json:"id"`                        // unique: "{requestId}:{hash}" or "tg:{update_id}"
	Session           string `json:"session"`                   // session name
	Type              string `json:"type"`                      // user_prompt / tool_call / assistant_text / notification
	Text              string `json:"text"`                      // message content
	Origin            string `json:"origin"`                    // terminal / telegram / claude
	TerminalDelivered bool   `json:"terminal_delivered"`        // whether terminal received it
	TelegramDelivered bool   `json:"telegram_delivered"`        // whether Telegram received it
	TelegramMsgID     int64  `json:"telegram_msg_id,omitempty"` // Telegram message ID (for editing)
	Timestamp         int64  `json:"timestamp"`                 // unix timestamp
	Update            string `json:"update,omitempty"`          // if set, this is an update record for the given ID
	UpdateField       string `json:"update_field,omitempty"`    // field name to update
	UpdateValue       any    `json:"update_value,omitempty"`    // new value
}
