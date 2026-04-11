package listen

import (
	"os"
	"os/signal"
	"syscall"

	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
)

// SetupSignalHandler registers a goroutine that gracefully exits on SIGINT/SIGTERM.
func SetupSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		loggingpkg.ListenLog("Shutting down (signal: %v)", sig)
		os.Exit(0)
	}()
}
