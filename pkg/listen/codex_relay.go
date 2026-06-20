package listen

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tuannvm/ccc/pkg/codexapp"
	"github.com/tuannvm/ccc/pkg/codexrelay"
	configpkg "github.com/tuannvm/ccc/pkg/config"
	loggingpkg "github.com/tuannvm/ccc/pkg/logging"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

var codexRelaySessionLocks sync.Map

func useCodexRelay(cfg *configpkg.Config, providerName string) bool {
	if cfg == nil || !cfg.CodexRelay.Enabled {
		return false
	}
	return providerpkg.IsCodexBackend(providerpkg.BackendForName(cfg, providerName))
}

func relayCodexSessionMessage(cfg *configpkg.Config, sessionName string, sessionInfo *configpkg.SessionInfo, text string, chatID, threadID int64) {
	relayCodexSessionInput(cfg, sessionName, sessionInfo, text, nil, chatID, threadID)
}

func relayCodexSessionInput(cfg *configpkg.Config, sessionName string, sessionInfo *configpkg.SessionInfo, text string, input []codexapp.UserInput, chatID, threadID int64) {
	go func() {
		lockValue, _ := codexRelaySessionLocks.LoadOrStore(sessionName, &sync.Mutex{})
		lock := lockValue.(*sync.Mutex)
		lock.Lock()
		defer lock.Unlock()
		freshCfg, err := configpkg.Load()
		if err == nil && freshCfg != nil {
			if freshInfo := freshCfg.Sessions[sessionName]; freshInfo != nil {
				cfg = freshCfg
				sessionInfo = freshInfo
			}
		}
		providerName := effectiveProviderName(cfg, sessionInfo)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := codexrelay.DeliverPrompt(ctx, codexrelay.Options{
			Config:      cfg,
			SessionName: sessionName,
			SessionInfo: sessionInfo,
			Prompt:      text,
			Input:       input,
			SaveConfig:  saveCodexRelaySessionState(sessionName, sessionInfo),
			NewClient:   codexRelayClientFactory(cfg, providerName),
			NewStream:   codexrelay.TelegramStreamFactory(cfg, chatID, threadID),
			ThreadStart: codexRelayThreadStart(cfg, providerName),
		}); err != nil {
			loggingpkg.ListenLog("[codex-relay] %s failed: %v", sessionName, err)
			telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("Codex relay failed: %v", err))
		}
	}()
}

func codexRelayThreadStart(cfg *configpkg.Config, providerName string) codexapp.ThreadStartParams {
	provider := providerpkg.GetProvider(cfg, providerName)
	configMap, model, modelProvider := providerpkg.CodexAppServerThreadConfig(provider)
	return codexapp.ThreadStartParams{
		Model:         model,
		ModelProvider: modelProvider,
		Config:        configMap,
	}
}

func codexRelayClientFactory(cfg *configpkg.Config, providerName string) codexrelay.ClientFactory {
	provider := providerpkg.GetProvider(cfg, providerName)
	env := os.Environ()
	if provider != nil {
		env = providerpkg.ApplyProviderEnv(env, provider, cfg)
	}
	return codexrelay.ClientFactoryWithOptions(codexapp.StartOptions{
		CodexPath:  tmux.CodexPath,
		Env:        env,
		ConfigArgs: providerpkg.CodexAppServerConfigArgs(provider),
	})
}

func saveCodexRelaySessionState(sessionName string, source *configpkg.SessionInfo) func(*configpkg.Config) error {
	return func(*configpkg.Config) error {
		fresh, err := configpkg.Load()
		if err != nil {
			return err
		}
		if fresh.Sessions == nil {
			return nil
		}
		info := fresh.Sessions[sessionName]
		if info == nil {
			return nil
		}
		info.CodexThreadID = source.CodexThreadID
		info.CodexTurnID = source.CodexTurnID
		return configpkg.Save(fresh)
	}
}

func interruptCodexRelaySession(cfg *configpkg.Config, sessionInfo *configpkg.SessionInfo, chatID, threadID int64) bool {
	providerName := effectiveProviderName(cfg, sessionInfo)
	if sessionInfo == nil || !useCodexRelay(cfg, providerName) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := codexrelay.Interrupt(ctx, cfg, sessionInfo, codexRelayClientFactory(cfg, providerName)); err != nil {
		telegram.SendMessage(cfg, chatID, threadID, fmt.Sprintf("Failed to interrupt Codex relay: %v", err))
		return true
	}
	telegram.SendMessage(cfg, chatID, threadID, "Interrupt sent to Codex relay")
	return true
}
