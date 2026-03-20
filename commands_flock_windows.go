//go:build windows

package main

import (
	"os"
)

// acquireFileLock is a no-op on Windows.
// Windows doesn't use the same daemon model as Unix systems.
func acquireFileLock(file *os.File) func() {
	return func() {}
}
