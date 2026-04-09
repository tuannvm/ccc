package listen

import (
	"os"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/telegram"
)

// StartTypingIndicator runs a background goroutine that sends "typing" actions
// for sessions that have a thinking flag set.
func StartTypingIndicator() {
	go func() {
		for {
			time.Sleep(4 * time.Second)
			cfg, err := configpkg.Load()
			if err != nil || cfg == nil {
				continue
			}
			for sessName, info := range cfg.Sessions {
				if info == nil || info.TopicID == 0 || cfg.GroupID == 0 {
					continue
				}
				if flagInfo, err := os.Stat(hooks.ThinkingFlag(sessName)); err == nil {
					// Auto-expire after 10 minutes to handle missed stop hooks
					if time.Since(flagInfo.ModTime()) > 10*time.Minute {
						hooks.ClearThinking(sessName)
						continue
					}
					telegram.SendTypingAction(cfg, cfg.GroupID, info.TopicID)
				}
			}
		}
	}()
}
