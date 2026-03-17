package main

import (
	"encoding/json"
)

// ========== Session Types ==========

// PaneInfo stores information about a pane within a session window
type PaneInfo struct {
	PaneIndex       string `json:"pane_index,omitempty"`       // runtime pane index ("0", "1", "2"...)
	ClaudeSessionID string `json:"claude_session_id,omitempty"` // Claude session ID in this pane
	ProviderName    string `json:"provider_name,omitempty"`     // Provider for this pane
	Name            string `json:"name,omitempty"`              // Friendly name (e.g., "reviewer")
}

// SessionInfo stores information about a session
type SessionInfo struct {
	TopicID         int64               `json:"topic_id"`
	Path            string              `json:"path"`
	ClaudeSessionID string              `json:"claude_session_id,omitempty"` // Legacy: primary pane's session ID
	WindowID        string              `json:"window_id,omitempty"`         // tmux window ID (@N)
	ProviderName    string              `json:"provider_name,omitempty"`     // Legacy: primary pane's provider
	IsWorktree      bool                `json:"is_worktree,omitempty"`       // Whether this is a worktree session
	WorktreeName    string              `json:"worktree_name,omitempty"`     // Name of the worktree
	BaseSession     string              `json:"base_session,omitempty"`      // Base session name for worktree
	Panes           map[string]*PaneInfo `json:"panes,omitempty"`           // pane_index -> PaneInfo
	ActivePane      string              `json:"active_pane,omitempty"`       // Currently active pane index
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
	BotToken string `json:"bot_token"`
	ChatID   int64  `json:"chat_id"`              // Private chat for simple commands
	GroupID  int64  `json:"group_id,omitempty"`   // Group with topics for sessions

	// ========== Sessions ==========
	Sessions map[string]*SessionInfo `json:"sessions,omitempty"` // session name -> session info

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
