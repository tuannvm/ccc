package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// Shared logging and state for the listen loop and related commands.

// listenLog writes timestamped log entries to ccc.log AND stdout.
// This ensures logs are always persisted regardless of how the process is started.
var listenLogFile *os.File

func listenLog(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] [pid:%d] %s\n", time.Now().Format("2006-01-02 15:04:05"), os.Getpid(), msg)
	fmt.Print(line)
	if listenLogFile != nil {
		listenLogFile.WriteString(line)
	}
}

func initListenLog() {
	logPath := filepath.Join(configpkg.CacheDir(), "ccc.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		listenLogFile = f
	}
}
