package main

import (
	"github.com/tuannvm/ccc/pkg/telegram"
)

// sendFile sends a file to Telegram (max 50MB)
func sendFile(config *Config, chatID int64, threadID int64, filePath string, caption string) error {
	return telegram.SendFile(config, chatID, threadID, filePath, caption)
}

// downloadTelegramFile downloads a file from Telegram
func downloadTelegramFile(config *Config, fileID string, destPath string) error {
	return telegram.DownloadTelegramFile(config, fileID, destPath)
}
