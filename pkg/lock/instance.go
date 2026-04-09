package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

// AcquireInstanceLock sets up a PID-based file lock to prevent multiple listen instances.
// Returns the lock file (caller must defer Close) and a release function (caller must defer).
func AcquireInstanceLock() (*os.File, func(), error) {
	// Small random delay to avoid race conditions when multiple instances start
	time.Sleep(time.Duration(os.Getpid()%500) * time.Millisecond)

	lockPath := filepath.Join(configpkg.CacheDir(), "ccc.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	releaseLock := acquireFileLock(lockFile)

	// Write our PID to the lock file
	lockFile.Truncate(0)
	lockFile.Seek(0, 0)
	fmt.Fprintf(lockFile, "%d\n", os.Getpid())

	return lockFile, releaseLock, nil
}
