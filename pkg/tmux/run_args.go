package tmux

import "strings"

// RunArgs holds parsed flags for the 'ccc run' command.
type RunArgs struct {
	ContinueSession bool
	ResumeSessionID string
	ProviderOverride string
	WorktreeName    string
}

// ParseRunArgs parses the flags for 'ccc run' from os.Args[2:].
func ParseRunArgs(args []string, autoWorktree string) RunArgs {
	var ra RunArgs
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" {
			ra.ContinueSession = true
		} else if args[i] == "--resume" && i+1 < len(args) {
			ra.ResumeSessionID = args[i+1]
			i++
		} else if args[i] == "--provider" && i+1 < len(args) {
			ra.ProviderOverride = args[i+1]
			i++
		} else if args[i] == "--worktree" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				ra.WorktreeName = args[i+1]
				i++
			} else {
				ra.WorktreeName = autoWorktree
			}
		}
	}
	return ra
}
