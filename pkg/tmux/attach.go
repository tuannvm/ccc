package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// AttachDelay is the time to wait for attach command to complete
const AttachDelay = 100 * time.Millisecond

// AttachToSession attaches the user's terminal to the tmux session for a given session name.
// If already inside tmux, it selects the window. Otherwise, it attaches to the session.
func AttachToSession(sessionName string) error {
	target, err := GetWindowTarget(sessionName)
	if err != nil {
		return fmt.Errorf("failed to get ccc window: %w", err)
	}

	if os.Getenv("TMUX") != "" {
		// Inside tmux: just select the window
		cmd := exec.Command(TmuxPath, "select-window", "-t", target)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Outside tmux: need to attach to the session and select the window
	sessName := strings.SplitN(target, ":", 2)[0]
	cmd := exec.Command(TmuxPath, "attach-session", "-t", sessName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start attach in background, then select the window
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait for attach to complete, then select the window
	time.Sleep(AttachDelay)
	exec.Command(TmuxPath, "select-window", "-t", target).Run()

	return cmd.Wait()
}
