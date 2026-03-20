//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// acquireFileLock uses Windows LockFileEx to acquire an exclusive lock on a file.
// Returns a function that releases the lock when called.
// If the lock cannot be acquired immediately, it exits the process.
func acquireFileLock(file *os.File) func() {
	handle := windows.Handle(file.Fd())

	// Lock entire file exclusively, non-blocking
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		0xFFFFFFFF, // Lock entire file (max DWORD)
		0xFFFFFFFF,
		&overlapped,
	)
	if err != nil {
		fmt.Println("Another ccc listen instance is already running, exiting quietly")
		os.Exit(0)
	}

	return func() {
		windows.UnlockFileEx(handle, 0, 0xFFFFFFFF, 0xFFFFFFFF, &overlapped)
	}
}
