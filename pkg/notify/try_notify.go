package notify

import (
	"fmt"
	"os"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// HandleDefaultCommand implements the default CLI case: try notification first,
// then fall through to session creation. Returns nil if notification was sent.
func HandleDefaultCommand(args []string, startSessionFn func(string) error) error {
	message := strings.Join(args, " ")
	if message != "" && TryNotifyIfAway(message) {
		return nil
	}
	return startSessionFn(message)
}

// TryNotifyIfAway attempts to send a notification if away mode is on.
// Returns true if the message was sent successfully (caller should return).
// Returns false if the caller should fall through to session creation.
// Exits the process on transient errors (network, rate limit).
func TryNotifyIfAway(message string) bool {
	config, err := configpkg.Load()
	if err != nil || config == nil {
		return false // not configured, fall through
	}

	if !config.Away {
		return false // away mode off, fall through
	}

	sendErr := SendIfAway(message)
	if sendErr == nil {
		return true // sent successfully
	}

	errMsg := strings.ToLower(sendErr.Error())
	isConfigError := strings.Contains(errMsg, "not configured") ||
		strings.Contains(errMsg, "chat not found") ||
		strings.Contains(errMsg, "unauthorized") ||
		strings.Contains(errMsg, "forbidden") ||
		strings.Contains(errMsg, "bad request")

	if isConfigError {
		fmt.Fprintf(os.Stderr, "Note: %v\n", sendErr)
		return false
	}

	fmt.Fprintf(os.Stderr, "Error: %v\n", sendErr)
	os.Exit(1)
	return false // unreachable
}
