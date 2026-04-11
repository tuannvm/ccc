package notify

import (
	"fmt"
	"os"
	"strings"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/telegram"
)

// SendIfAway sends a notification message via Telegram if away mode is on.
// It tries the session topic first (if in a session directory), then falls back to private chat.
func SendIfAway(message string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	if !cfg.Away {
		fmt.Println("Away mode off, skipping notification.")
		return nil
	}

	// Try to send to session topic if we're in a session directory
	if cfg.GroupID != 0 {
		cwd, _ := os.Getwd()
		for name, info := range cfg.Sessions {
			if info == nil {
				continue
			}
			if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") || strings.HasSuffix(cwd, "/"+name) {
				return telegram.SendMessage(cfg, cfg.GroupID, info.TopicID, message)
			}
		}
	}

	// Fallback to private chat
	return telegram.SendMessage(cfg, cfg.ChatID, 0, message)
}
