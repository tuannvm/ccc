package hooks

import "encoding/json"

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

// ParseHookData unmarshals raw JSON and populates ToolInput
func ParseHookData(data []byte) (HookData, error) {
	var hd HookData
	if err := json.Unmarshal(data, &hd); err != nil {
		return hd, err
	}
	var aliases struct {
		TranscriptPath string `json:"transcriptPath"`
		SessionID      string `json:"sessionId"`
		HookEventName  string `json:"hookEventName"`
		ToolName       string `json:"toolName"`
	}
	if err := json.Unmarshal(data, &aliases); err == nil {
		if hd.TranscriptPath == "" {
			hd.TranscriptPath = aliases.TranscriptPath
		}
		if hd.SessionID == "" {
			hd.SessionID = aliases.SessionID
		}
		if hd.HookEventName == "" {
			hd.HookEventName = aliases.HookEventName
		}
		if hd.ToolName == "" {
			hd.ToolName = aliases.ToolName
		}
	}
	if len(hd.ToolInputRaw) > 0 {
		json.Unmarshal(hd.ToolInputRaw, &hd.ToolInput)
	}
	return hd, nil
}
