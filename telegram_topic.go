package main

import (
	"github.com/tuannvm/ccc/pkg/telegram"
)

// createForumTopic creates a new forum topic
func createForumTopic(config *Config, name string, providerName string, baseSessionName string) (int64, error) {
	return telegram.CreateForumTopic(config, name, providerName, baseSessionName)
}

// createForumTopicWithEmoji creates a forum topic with a custom emoji icon
func createForumTopicWithEmoji(config *Config, name string, providerName string, baseSessionName string) (int64, error) {
	return telegram.CreateForumTopicWithEmoji(config, name, providerName, baseSessionName)
}

// deleteForumTopic deletes a forum topic
func deleteForumTopic(config *Config, topicID int64) error {
	return telegram.DeleteForumTopic(config, topicID)
}

// setBotCommands sets the bot commands in Telegram
func setBotCommands(botToken string) {
	telegram.SetBotCommands(botToken)
}
