package update

import (
	"fmt"
	"net/http"
	"os"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// ApplyUpdate downloads, validates, and replaces the ccc binary, then restarts.
func ApplyUpdate(config *configpkg.Config, chatID, threadID int64, offset int) {
	telegram.SendMessage(config, chatID, threadID, "🔄 Updating ccc...")

	downloadURL := BuildDownloadURL()

	result, err := DownloadBinary(downloadURL, tmux.CCCPath)
	if err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ %v", err))
		return
	}

	if err := ValidateBinary(result.TmpPath); err != nil {
		os.Remove(result.TmpPath)
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ %v", err))
		return
	}

	if err := ReplaceBinary(tmux.CCCPath, result.TmpPath); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ %v", err))
		return
	}

	telegram.SendMessage(config, chatID, threadID, "✅ Updated. Restarting...")
	// Confirm offset so the /update message is not reprocessed after restart
	_, _ = http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=1", config.BotToken, offset))
	os.Exit(0)
}
