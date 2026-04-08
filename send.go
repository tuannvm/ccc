package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/telegram"
)

// Send notification (only if away mode is on).
func send(message string) error {
	config, err := config.Load()
	if err != nil {
		return fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	if !config.Away {
		fmt.Println("Away mode off, skipping notification.")
		return nil
	}

	// Try to send to session topic if we're in a session directory
	if config.GroupID != 0 {
		cwd, _ := os.Getwd()
		for name, info := range config.Sessions {
			if info == nil {
				continue
			}
			// Match against saved path, subdirectories of saved path, or suffix
			if cwd == info.Path || strings.HasPrefix(cwd, info.Path+"/") || strings.HasSuffix(cwd, "/"+name) {
				return telegram.SendMessage(config, config.GroupID, info.TopicID, message)
			}
		}
	}

	// Fallback to private chat
	return telegram.SendMessage(config, config.ChatID, 0, message)
}
