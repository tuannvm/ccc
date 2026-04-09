package listen

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/telegram"
)

// PollResult holds the parsed response from a Telegram getUpdates poll.
type PollResult struct {
	Response  *telegram.TelegramUpdate
	Offset    int
	ShouldSkip bool // true if the response was invalid (caller should continue)
}

// PollUpdates performs a single long-poll request to Telegram getUpdates API.
// Returns parsed response and the new offset. On transient errors, logs and returns
// ShouldSkip=true so the caller can retry.
func PollUpdates(client *http.Client, config *configpkg.Config, offset int) PollResult {
	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", config.BotToken, offset)
	resp, err := telegram.TelegramClientGet(client, config.BotToken, reqURL)
	if err != nil {
		loggingpkg.ListenLog("Network error: %v (retrying...)", err)
		time.Sleep(5 * time.Second)
		return PollResult{Offset: offset, ShouldSkip: true}
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.MaxResponseSize))
	resp.Body.Close()

	var updates telegram.TelegramUpdate
	if err := json.Unmarshal(body, &updates); err != nil {
		loggingpkg.ListenLog("Parse error: %v", err)
		time.Sleep(time.Second)
		return PollResult{Offset: offset, ShouldSkip: true}
	}

	if !updates.OK {
		loggingpkg.ListenLog("Telegram API error: %s", updates.Description)
		time.Sleep(5 * time.Second)
		return PollResult{Offset: offset, ShouldSkip: true}
	}

	return PollResult{
		Response: &updates,
		Offset:   offset,
	}
}
