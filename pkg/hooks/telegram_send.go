package hooks

import (
	"os"
	"time"

	"github.com/tuannvm/ccc/pkg/tmux"
)

// SendFromTelegram sets the Telegram active flag before sending,
// so the permission hook knows this input came from Telegram and requires OTP.
func SendFromTelegram(target string, windowName string, text string) error {
	os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600)
	return tmux.SendKeys(target, text)
}

// SendFromTelegramWithDelay sets the Telegram active flag before sending with a delay.
func SendFromTelegramWithDelay(target string, windowName string, text string, delay time.Duration) error {
	os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600)
	return tmux.SendKeysWithDelay(target, text, delay)
}
