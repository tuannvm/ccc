package shell

import "strings"

// Quote safely quotes a string for shell command arguments using single quotes.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
