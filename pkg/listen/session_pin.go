package listen

import (
	"fmt"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	"github.com/tuannvm/ccc/pkg/lookup"
	"github.com/tuannvm/ccc/pkg/telegram"
)

func sessionPinMessage(cfg *configpkg.Config, sessionName string, info *configpkg.SessionInfo) string {
	path := ""
	if info != nil {
		path = lookup.GetSessionWorkDir(cfg, sessionName, info)
	}
	if path == "" {
		path = configpkg.ResolveProjectPath(cfg, sessionName)
	}

	return fmt.Sprintf("session: %s\nprovider: %s\npath: %s", sessionName, effectiveProviderName(cfg, info), path)
}

func pinSessionHeader(cfg *configpkg.Config, sessionName string, info *configpkg.SessionInfo) {
	if cfg == nil || cfg.GroupID == 0 || info == nil || info.TopicID == 0 {
		return
	}

	msg := sessionPinMessage(cfg, sessionName, info)
	if info.PinnedHeaderMsgID != 0 {
		if err := telegram.EditPlainMessage(cfg, cfg.GroupID, info.PinnedHeaderMsgID, msg); err == nil {
			if err := telegram.PinMessage(cfg, cfg.GroupID, info.PinnedHeaderMsgID); err != nil {
				loggingpkg.ListenLog("[pin] failed to re-pin session header for %s: %v", sessionName, err)
			}
			return
		} else {
			loggingpkg.ListenLog("[pin] failed to update session header for %s: %v", sessionName, err)
		}
	}

	msgID, err := telegram.SendPlainMessageGetID(cfg, cfg.GroupID, info.TopicID, msg)
	if err != nil {
		loggingpkg.ListenLog("[pin] failed to send session header for %s: %v", sessionName, err)
		return
	}
	info.PinnedHeaderMsgID = msgID
	if err := configpkg.Save(cfg); err != nil {
		loggingpkg.ListenLog("[pin] failed to persist pinned header for %s: %v", sessionName, err)
	}
	if err := telegram.PinMessage(cfg, cfg.GroupID, msgID); err != nil {
		loggingpkg.ListenLog("[pin] failed to pin session header for %s: %v", sessionName, err)
	}
}
