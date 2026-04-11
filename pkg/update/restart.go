package update

import (
	"os"
	"os/exec"
	"time"
)

// RestartProcess re-execs the current binary with the given arguments after a short delay.
func RestartProcess(args ...string) {
	go func() {
		time.Sleep(500 * time.Millisecond)
		exe, err := os.Executable()
		if err != nil {
			return
		}
		exec.Command(exe, args...).Start()
		os.Exit(0)
	}()
}
