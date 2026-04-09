package lookup

import (
	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// GetSessionNameFromInfo returns the session name from info, falling back to
// deriving it from the session path via tmux.
func GetSessionNameFromInfo(info *config.SessionInfo) string {
	if info.SessionName != "" {
		return info.SessionName
	}
	return tmux.GetSessionNameFromPath(info.Path)
}
