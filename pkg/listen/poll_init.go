package listen

import (
	"fmt"
	"os"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	lockpkg "github.com/tuannvm/ccc/pkg/lock"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/telegram"
)

// ListenerInit holds the results of listener initialization.
type ListenerInit struct {
	Config      *configpkg.Config
	LockFile    *os.File
	ReleaseLock func()
}

// InitializeListener performs all pre-loop setup: acquires lock, initializes
// logging, loads config, sets bot commands, recovers messages, and starts
// signal handler and typing indicator.
func InitializeListener() (*ListenerInit, error) {
	lockFile, releaseLock, err := lockpkg.AcquireInstanceLock()
	if err != nil {
		return nil, err
	}

	loggingpkg.Init()

	config, err := configpkg.Load()
	if err != nil {
		lockFile.Close()
		releaseLock()
		return nil, fmt.Errorf("not configured. Run: ccc setup <bot_token>")
	}

	loggingpkg.ListenLog("Bot started (chat: %d, group: %d, sessions: %d)", config.ChatID, config.GroupID, len(config.Sessions))

	telegram.SetBotCommands(config.BotToken)

	RecoverUndeliveredMessages(config)
	SetupSignalHandler()
	StartTypingIndicator()

	return &ListenerInit{
		Config:      config,
		LockFile:    lockFile,
		ReleaseLock: releaseLock,
	}, nil
}
