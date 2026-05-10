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
	return SendFromTelegramToBackend(target, windowName, text, tmux.ResolveAgentBackend(providerName))
}

// SendFromTelegramToBackend sends text using backend-specific TUI submission behavior.
func SendFromTelegramToBackend(target string, windowName string, text string, backend string) error {
	if err := os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600); err != nil {
		return fmt.Errorf("failed to set telegram flag: %w", err)
	}
	if tmux.IsCodexBackend(backend) {
		if err := tmux.WaitForAgentBackendInputPrompt(target, backend, 60*time.Second); err != nil {
			return err
		}
	}
	return tmux.SendKeysForBackend(target, text, backend)
}

// SendFromTelegramToProviderWithDelay sends text using backend-specific TUI
// submission behavior after the requested settling delay.
func SendFromTelegramToProviderWithDelay(target string, windowName string, text string, providerName string, delay time.Duration) error {
	return SendFromTelegramToBackendWithDelay(target, windowName, text, tmux.ResolveAgentBackend(providerName), delay)
}

// SendFromTelegramToBackendWithDelay sends text using backend-specific TUI
// submission behavior after the requested settling delay.
func SendFromTelegramToBackendWithDelay(target string, windowName string, text string, backend string, delay time.Duration) error {
	if err := os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600); err != nil {
		return fmt.Errorf("failed to set telegram flag: %w", err)
	}
	if tmux.IsCodexBackend(backend) {
		if err := tmux.WaitForAgentBackendInputPrompt(target, backend, 60*time.Second); err != nil {
			return err
		}
	}
	return tmux.SendKeysForBackendWithDelay(target, text, backend, delay)
}

// SendFromTelegramWithDelay sets the Telegram active flag before sending with a delay.
func SendFromTelegramWithDelay(target string, windowName string, text string, delay time.Duration) error {
	if err := os.WriteFile(TelegramActiveFlag(windowName), []byte("1"), 0600); err != nil {
		return fmt.Errorf("failed to set telegram flag: %w", err)
	}
	return tmux.SendKeysWithDelay(target, text, delay)
}
