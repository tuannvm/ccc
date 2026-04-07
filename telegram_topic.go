package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// worktreeColor generates a consistent color for worktree sessions based on the base project name.
// All worktrees belonging to the same base project will have the same color, creating visual grouping.
// Returns the Telegram icon_color integer value as a string (Telegram only allows 6 specific colors).
func worktreeColor(baseSessionName string) string {
	// Hash the base name to get a consistent color using FNV-1a algorithm
	var hash uint32 = 2166136261 // FNV offset basis
	for _, c := range baseSessionName {
		hash ^= uint32(c)
		hash *= 16777619 // FNV prime
	}

	// Telegram only allows these 6 specific icon_color values (decimal integers)
	// See: https://core.telegram.org/bots/api#createforumtopic
	colors := []string{
		"7322096",  // Blue (0x6FB9F0)
		"16766590", // Yellow (0xFFD67E)
		"13338331", // Violet (0xCB86DB)
		"9367192",  // Green (0x8EEE98)
		"16749490", // Rose (0xFF93B2)
		"16478047", // Red (0xFB6F5F)
	}
	return colors[hash%uint32(len(colors))]
}

func createForumTopic(config *Config, name string, providerName string, baseSessionName string) (int64, error) {
	// Use createForumTopicWithEmoji for API 9.5 custom emoji support
	// This automatically handles fallback to letter prefix if emoji is not available
	return createForumTopicWithEmoji(config, name, providerName, baseSessionName)
}

func deleteForumTopic(config *Config, topicID int64) error {
	if config.GroupID == 0 {
		return fmt.Errorf("no group configured")
	}

	params := url.Values{
		"chat_id":           {fmt.Sprintf("%d", config.GroupID)},
		"message_thread_id": {fmt.Sprintf("%d", topicID)},
	}

	result, err := telegramAPI(config, "deleteForumTopic", params)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("failed to delete topic: %s", result.Description)
	}

	return nil
}

// setBotCommands sets the bot commands in Telegram
func setBotCommands(botToken string) {
	commands := []map[string]string{
		{"command": "new", "description": "Create/restart session: /new <name>"},
		{"command": "team", "description": "Create team session (3-pane): /team <name>"},
		{"command": "continue", "description": "Restart session with history"},
		{"command": "delete", "description": "Delete current session and thread"},
		{"command": "resume", "description": "List/switch Claude sessions: /resume [id]"},
		{"command": "worktree", "description": "Create worktree session: /worktree <base> <name>"},
		{"command": "providers", "description": "List available AI providers"},
		{"command": "cleanup", "description": "Delete ALL sessions, folders and threads"},
		{"command": "c", "description": "Execute shell command: /c <cmd>"},
		{"command": "update", "description": "Update ccc binary from GitHub"},
		{"command": "version", "description": "Show ccc version"},
		{"command": "stats", "description": "Show system stats (RAM, disk, etc)"},
		{"command": "auth", "description": "Re-authenticate Claude OAuth"},
		{"command": "stop", "description": "Stop/interrupt current Claude execution"},
	}

	// Set for default scope
	defaultBody, _ := json.Marshal(map[string]any{
		"commands": commands,
	})
	resp, err := http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", botToken),
		"application/json",
		bytes.NewReader(defaultBody),
	)
	if err == nil {
		resp.Body.Close()
	}

	// Set for all group chats (makes the / button appear)
	groupBody, _ := json.Marshal(map[string]any{
		"commands": commands,
		"scope":    map[string]string{"type": "all_group_chats"},
	})
	resp, err = http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", botToken),
		"application/json",
		bytes.NewReader(groupBody),
	)
	if err == nil {
		resp.Body.Close()
	}
}

// ========== API 9.5: Telegram Bot API 9.5 Support ==========
// https://core.telegram.org/bots/api#march-1-2026

// createForumTopicWithEmoji creates a forum topic with a custom emoji icon
// This is an enhanced version of createForumTopic that uses icon_custom_emoji_id instead of letter prefix
func createForumTopicWithEmoji(config *Config, name string, providerName string, baseSessionName string) (int64, error) {
	if config.GroupID == 0 {
		return 0, fmt.Errorf("no group configured")
	}

	// Determine emoji ID based on provider and whether this is a worktree
	var emojiID string
	isWorktree := baseSessionName != ""
	if isWorktree {
		emojiID = getEmojiIDForWorktree(config, baseSessionName)
	} else if providerName != "" {
		emojiID = getEmojiIDForProvider(config, providerName)
	}

	// Build topic name - when using emoji, we don't need the letter prefix
	topicName := name

	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", config.GroupID)},
		"name":    {topicName},
	}

	// Add custom emoji icon if available
	if emojiID != "" {
		params.Add("icon_custom_emoji_id", emojiID)
		// Note: icon_color is mutually exclusive with icon_custom_emoji_id
		// We cannot use both, so we skip icon_color when using custom emoji
	} else {
		// No custom emoji available - use fallbacks
		if providerName != "" && len(providerName) > 0 {
			// Add letter prefix for provider identification
			prefix := strings.ToUpper(string(providerName[0]))
			topicName = fmt.Sprintf("%s %s", prefix, name)
			params.Set("name", topicName)
		}

		// Add icon color for worktree sessions to group them by base project
		// This applies even when providerName is empty (e.g., worktree for default session)
		if baseSessionName != "" {
			params.Add("icon_color", worktreeColor(baseSessionName))
		}
	}

	result, err := telegramAPI(config, "createForumTopic", params)
	if err != nil {
		return 0, err
	}
	if !result.OK {
		// If we used a custom emoji and it failed, retry without it
		// This handles invalid/stale emoji IDs gracefully
		if emojiID != "" {
			// Remove custom emoji and use fallback (letter prefix + icon color)
			params.Del("icon_custom_emoji_id")

			// Add letter prefix for provider identification
			if providerName != "" && len(providerName) > 0 {
				prefix := strings.ToUpper(string(providerName[0]))
				topicName = fmt.Sprintf("%s %s", prefix, name)
				params.Set("name", topicName)
			}

			// Add icon color for worktree sessions
			if baseSessionName != "" {
				params.Add("icon_color", worktreeColor(baseSessionName))
			}

			// Retry topic creation without custom emoji
			result, err = telegramAPI(config, "createForumTopic", params)
			if err != nil {
				return 0, err
			}
			if !result.OK {
				return 0, fmt.Errorf("failed to create topic (even without emoji): %s", result.Description)
			}
		} else {
			return 0, fmt.Errorf("failed to create topic: %s", result.Description)
		}
	}

	var topic TopicResult
	if err := json.Unmarshal(result.Result, &topic); err != nil {
		return 0, fmt.Errorf("failed to parse topic result: %w", err)
	}

	return topic.MessageThreadID, nil
}

// normalizeProviderAlias converts provider aliases to canonical names
// This ensures that custom emoji lookups work regardless of which alias was used
func normalizeProviderAlias(providerName string) string {
	// Map common aliases to canonical provider names
	// These should match the documented aliases in API_9_5_FEATURES.md
	switch strings.ToLower(providerName) {
	case "z":
		return "zai"
	case "d", "ds":
		return "deepseek"
	case "m":
		return "minimax"
	case "c", "anthropic":
		return "claude"
	default:
		return strings.ToLower(providerName)
	}
}

// getEmojiIDForProvider returns the custom emoji ID for a given provider
// Only returns non-empty if the user has explicitly configured a custom emoji ID
// Supports provider aliases (z→zai, d/ds→deepseek, m→minimax, c/anthropic→claude)
func getEmojiIDForProvider(config *Config, providerName string) string {
	if providerName == "" {
		return ""
	}

	// Normalize the provider name to handle aliases
	canonicalName := normalizeProviderAlias(providerName)

	// Only use user-configured custom emoji IDs
	// Built-in placeholder constants are NOT returned to avoid API errors
	if config.CustomEmojiIDs != nil {
		// Try canonical name first
		if emojiID, ok := config.CustomEmojiIDs[canonicalName]; ok && emojiID != "" {
			return emojiID
		}
		// Fall back to original providerName (in case user configured with alias)
		if emojiID, ok := config.CustomEmojiIDs[providerName]; ok && emojiID != "" {
			return emojiID
		}
	}
	return "" // No valid custom emoji configured - will fall back to letter prefix
}

// getEmojiIDForWorktree returns a consistent emoji ID for worktree sessions
// Only returns non-empty if the user has explicitly configured a custom emoji ID
func getEmojiIDForWorktree(config *Config, baseSessionName string) string {
	// Only use user-configured custom emoji IDs
	// Built-in placeholder constants are NOT returned to avoid API errors
	if config.CustomEmojiIDs != nil {
		if emojiID, ok := config.CustomEmojiIDs["worktree"]; ok && emojiID != "" {
			return emojiID
		}
	}
	return "" // No valid custom emoji configured - will fall back to letter prefix
}
