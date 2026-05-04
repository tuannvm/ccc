package hooks

import (
	"fmt"
	"os"
	"time"

	"github.com/tuannvm/ccc/pkg/tmux"
)

// SendFromTelegram sets the Telegram active flag before sending,
// so the permission hook knows this input came from Telegram and requires OTP.
func SendFromTelegram(target string, windowName string, text string) error {
	if err := os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600); err != nil {
		return fmt.Errorf("failed to set telegram flag: %w", err)
	}
	return tmux.SendKeys(target, text)
}

// SendFromTelegramToProvider sends text using backend-specific TUI submission behavior.
func SendFromTelegramToProvider(target string, windowName string, text string, providerName string) error {
	if err := os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600); err != nil {
		return fmt.Errorf("failed to set telegram flag: %w", err)
	}
	if tmux.IsCodexProviderName(providerName) {
		if err := tmux.WaitForAgentInputPrompt(target, providerName, 60*time.Second); err != nil {
			return err
		}
	}
	return tmux.SendKeysForProvider(target, text, providerName)
}

// SendFromTelegramToProviderWithDelay sends text using backend-specific TUI
// submission behavior after the requested settling delay.
func SendFromTelegramToProviderWithDelay(target string, windowName string, text string, providerName string, delay time.Duration) error {
	if err := os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600); err != nil {
		return fmt.Errorf("failed to set telegram flag: %w", err)
	}
	if tmux.IsCodexProviderName(providerName) {
		if err := tmux.WaitForAgentInputPrompt(target, providerName, 60*time.Second); err != nil {
			return err
		}
	}
	return tmux.SendKeysForProviderWithDelay(target, text, providerName, delay)
}

// SendFromTelegramWithDelay sets the Telegram active flag before sending with a delay.
func SendFromTelegramWithDelay(target string, windowName string, text string, delay time.Duration) error {
	if err := os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600); err != nil {
		return fmt.Errorf("failed to set telegram flag: %w", err)
	}
	return tmux.SendKeysWithDelay(target, text, delay)
}
