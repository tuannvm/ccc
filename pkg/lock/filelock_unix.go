//go:build unix

package lock

import (
	"os"
	"syscall"
)

func lockFileBlocking(file *os.File) (func(), error) {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	}, nil
}
