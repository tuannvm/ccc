//go:build windows

package lock

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockFileBlocking(file *os.File) (func(), error) {
	handle := windows.Handle(file.Fd())
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		0xFFFFFFFF,
		0xFFFFFFFF,
		&overlapped,
	); err != nil {
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(handle, 0, 0xFFFFFFFF, 0xFFFFFFFF, &overlapped)
	}, nil
}
