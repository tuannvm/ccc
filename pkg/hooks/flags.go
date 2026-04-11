package hooks

import (
	"os"
	"path/filepath"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// TelegramActiveFlag returns the path of the flag file that indicates
// a Telegram message is being processed by a tmux session.
func TelegramActiveFlag(tmuxName string) string {
	return filepath.Join(configpkg.CacheDir(), "telegram-active-"+tmuxName)
}

// ThinkingFlag returns the path of the flag file that indicates
// Claude is actively processing in a session (for typing indicator).
func ThinkingFlag(sessionName string) string {
	return filepath.Join(configpkg.CacheDir(), "thinking-"+sessionName)
}

// SetThinking creates the thinking flag file for a session.
func SetThinking(sessionName string) {
	os.WriteFile(ThinkingFlag(sessionName), []byte("1"), 0600)
}

// ClearThinking removes the thinking flag file for a session.
func ClearThinking(sessionName string) {
	os.Remove(ThinkingFlag(sessionName))
}

// PromptAckPath returns the path of the ack file that confirms
// Claude received a prompt sent from Telegram via tmux send-keys.
func PromptAckPath(sessionName string) string {
	return filepath.Join(configpkg.CacheDir(), "prompt-ack-"+sessionName)
}

// WritePromptAck creates the prompt acknowledgment flag file.
func WritePromptAck(sessionName string) {
	os.WriteFile(PromptAckPath(sessionName), []byte("1"), 0600)
}
