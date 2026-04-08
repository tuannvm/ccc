package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

const maxResponseSize = 10 * 1024 * 1024 // 10MB

// redactTokenError replaces the bot token in error messages with "***"
func redactTokenError(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "***"))
}

// telegramGet performs an HTTP GET and redacts the bot token from any errors
func telegramGet(token string, url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, redactTokenError(err, token)
	}
	return resp, nil
}

// telegramClientGet performs an HTTP GET with a custom client and redacts the bot token from any errors
func telegramClientGet(client *http.Client, token string, url string) (*http.Response, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, redactTokenError(err, token)
	}
	return resp, nil
}

// updateCCC downloads the latest ccc binary from GitHub releases and restarts
func updateCCC(config *Config, chatID, threadID int64, offset int) {
	telegram.SendMessage(config, chatID, threadID, "🔄 Updating ccc...")

	binaryName := fmt.Sprintf("ccc-%s-%s", runtime.GOOS, runtime.GOARCH)
	downloadURL := fmt.Sprintf("https://github.com/tuannvm/ccc/releases/latest/download/%s", binaryName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: HTTP %d (no release for %s?)", resp.StatusCode, binaryName))
		return
	}

	tmpPath := tmux.CCCPath + ".new"
	f, err := os.Create(tmpPath)
	if err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create temp file: %v", err))
		return
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to write binary: %v", err))
		return
	}

	// Validate downloaded binary size (ccc should be > 1MB)
	if written < 1000000 {
		os.Remove(tmpPath)
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Downloaded file too small (%d bytes), aborting", written))
		return
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to chmod: %v", err))
		return
	}

	// Test the new binary before replacing
	testCmd := exec.Command(tmpPath, "version")
	if err := testCmd.Run(); err != nil {
		os.Remove(tmpPath)
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ New binary failed validation: %v", err))
		return
	}

	// Backup old binary
	backupPath := tmux.CCCPath + ".bak"
	os.Remove(backupPath) // Remove old backup if exists
	if err := os.Rename(tmux.CCCPath, backupPath); err != nil {
		os.Remove(tmpPath)
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to backup old binary: %v", err))
		return
	}

	// Replace with new binary
	if err := os.Rename(tmpPath, tmux.CCCPath); err != nil {
		// Restore backup
		os.Rename(backupPath, tmux.CCCPath)
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to replace binary: %v", err))
		return
	}

	// Codesign on macOS
	if runtime.GOOS == "darwin" {
		if err := exec.Command("codesign", "-f", "-s", "-", tmux.CCCPath).Run(); err != nil {
			// Restore backup if codesign fails
			os.Remove(tmux.CCCPath)
			os.Rename(backupPath, tmux.CCCPath)
			telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Codesign failed: %v", err))
			return
		}
	}

	// Success - remove backup
	os.Remove(backupPath)

	telegram.SendMessage(config, chatID, threadID, "✅ Updated. Restarting...")
	// Confirm offset so the /update message is not reprocessed after restart
	http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=1", config.BotToken, offset))
	os.Exit(0)
}
