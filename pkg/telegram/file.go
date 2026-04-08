package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tuannvm/ccc/pkg/config"
)

// SendFile sends a file to Telegram (max 50MB)
func SendFile(cfg *config.Config, chatID int64, threadID int64, filePath string, caption string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add chat_id
	writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if threadID > 0 {
		writer.WriteField("message_thread_id", fmt.Sprintf("%d", threadID))
	}
	if caption != "" {
		writer.WriteField("caption", caption)
	}

	// Add file
	part, err := writer.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		return err
	}
	io.Copy(part, file)
	writer.Close()

	resp, err := http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", cfg.BotToken),
		writer.FormDataContentType(),
		body,
	)
	if err != nil {
		return RedactTokenError(err, cfg.BotToken)
	}
	defer resp.Body.Close()

	var result TelegramResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

// DownloadTelegramFile downloads a file from Telegram
func DownloadTelegramFile(cfg *config.Config, fileID string, destPath string) error {
	// Get file path from Telegram
	resp, err := TelegramGet(cfg.BotToken, fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", cfg.BotToken, fileID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("failed to get file path")
	}

	// Download the file
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", cfg.BotToken, result.Result.FilePath)
	fileResp, err := TelegramGet(cfg.BotToken, fileURL)
	if err != nil {
		return err
	}
	defer fileResp.Body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, fileResp.Body)
	return err
}

// TelegramGet performs an HTTP GET and redacts the bot token from any errors
func TelegramGet(token string, url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, RedactTokenError(err, token)
	}
	return resp, nil
}
