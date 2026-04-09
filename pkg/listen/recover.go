package listen

import (
	"fmt"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/telegram"
)

// RecoverUndeliveredMessages sends any undelivered Telegram messages from the ledger.
func RecoverUndeliveredMessages(config *configpkg.Config) {
	for sessName, info := range config.Sessions {
		if info == nil || info.TopicID == 0 || config.GroupID == 0 {
			continue
		}
		undelivered := ledger.FindUndelivered(sessName, "telegram")
		for _, ur := range undelivered {
			if ur.Type == "assistant_text" || ur.Type == "notification" {
				telegram.SendMessage(config, config.GroupID, info.TopicID, fmt.Sprintf("*%s:*\n%s", sessName, ur.Text))
				ledger.UpdateDelivery(sessName, ur.ID, "telegram_delivered", true)
			}
		}
	}
}
