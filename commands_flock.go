//go:build unix

package main

import (
	"fmt"
	"os"
	"syscall"
)

// acquireFileLock uses Unix flock to acquire an exclusive lock on a file.
// Returns a function that releases the lock when called.
// If the lock cannot be acquired immediately, it exits the process.
func acquireFileLock(file *os.File) func() {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fmt.Println("Another ccc listen instance is already running, exiting quietly")
		os.Exit(0) // Exit with 0 so launchd doesn't restart
	}
	return func() {
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	}
}
