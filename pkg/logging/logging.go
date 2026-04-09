package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// logFile is the shared log file handle for the listen loop.
var logFile *os.File

// ListenLog writes timestamped log entries to ccc.log AND stdout.
func ListenLog(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] [pid:%d] %s\n", time.Now().Format("2006-01-02 15:04:05"), os.Getpid(), msg)
	fmt.Print(line)
	if logFile != nil {
		logFile.WriteString(line)
	}
}

// Init opens the log file for appending.
func Init() {
	logPath := filepath.Join(configpkg.CacheDir(), "ccc.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		logFile = f
	}
}
