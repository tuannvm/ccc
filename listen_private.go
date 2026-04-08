package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tuannvm/ccc/pkg/telegram"
)

// handlePrivateChat handles one-shot Claude execution in private chat
func handlePrivateChat(config *Config, msg TelegramMessage, chatID, threadID int64) {
	telegram.SendMessage(config, chatID, threadID, "🤖 Running Claude...")

	prompt := strings.TrimSpace(msg.Text)
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.Text != "" {
		origText := msg.ReplyToMessage.Text
		origWords := strings.Fields(origText)
		if len(origWords) > 0 {
			home, _ := os.UserHomeDir()
			potentialDir := filepath.Join(home, origWords[0])
			if info, err := os.Stat(potentialDir); err == nil && info.IsDir() {
				prompt = origWords[0] + " " + msg.Text
			}
		}
		prompt = fmt.Sprintf("Original message:\n%s\n\nReply:\n%s", origText, prompt)
	}

	go func(p string, cid int64) {
		defer func() {
			if r := recover(); r != nil {
				telegram.SendMessage(config, cid, 0, fmt.Sprintf("💥 Panic: %v", r))
			}
		}()
		output, err := runClaude(p)
		if err != nil {
			if strings.Contains(err.Error(), "context deadline exceeded") {
				output = fmt.Sprintf("⏱️ Timeout (10min)\n\n%s", output)
			} else {
				output = fmt.Sprintf("⚠️ %s\n\nExit: %v", output, err)
			}
		}
		telegram.SendMessage(config, cid, 0, output)
	}(prompt, chatID)
}
