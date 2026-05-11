package lock

import (
	"fmt"
	"os"
	"path/filepath"
)

// AcquireFileLock takes a blocking exclusive lock on path and returns a release
// function. It is intended for short-lived cross-process data-file updates.
func AcquireFileLock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	release, err := lockFileBlocking(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to lock %s: %w", path, err)
	}
	return func() {
		release()
		_ = file.Close()
	}, nil
}
