package main

import (
	"fmt"
	"os"

	"github.com/tuannvm/ccc/pkg/auth"
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/lookup"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/session"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

func newHandlerCallbacks() *hooks.HandlerCallbacks {
	return &hooks.HandlerCallbacks{
		LoadConfig:              configpkg.Load,
		SaveConfig:              configpkg.Save,
		FindSession:             lookup.FindSession,
		PersistClaudeSessionID:  lookup.PersistClaudeSessionID,
		GetSessionWorkDir:       lookup.GetSessionWorkDir,
		LoadToolState:           hooks.LoadToolState,
		SaveToolState:           hooks.SaveToolState,
		ClearToolState:          hooks.ClearToolState,
		AddTextToToolState:      hooks.AddTextToToolState,
		FormatToolMessage:       hooks.FormatToolMessage,
		SetThinking:             hooks.SetThinking,
		ClearThinking:           hooks.ClearThinking,
		TelegramActiveFlag:      hooks.TelegramActiveFlag,
		SendMessage:             telegram.SendMessage,
		SendMessageHTML:         telegram.SendMessageHTMLGetID,
		SendMessageGetID:        telegram.SendMessageGetID,
		EditMessageHTML:         telegram.EditMessageHTML,
		SendMessageWithKeyboard: telegram.SendMessageWithKeyboard,
		IsDelivered:             ledger.IsDelivered,
		AppendMessage:           func(msg *ledger.MessageRecord) { ledger.AppendMessage(msg) },
		UpdateDelivery: func(sessName, msgID, field string, value bool) {
			ledger.UpdateDelivery(sessName, msgID, field, value)
		},
		IsOTPEnabled:       auth.IsOTPEnabled,
		HasValidOTPGrant:   auth.HasValidOTPGrant,
		WriteOTPRequest:    func(sessionID string, req *hooks.OTPPermissionRequest) { auth.WriteOTPRequest(sessionID, req) },
		WriteOTPGrant:      auth.WriteOTPGrant,
		WaitForOTPResponse: auth.WaitForOTPResponse,
		TmuxSafeName:       tmux.SafeName,
		WritePromptAck:     hooks.WritePromptAck,
		InferRoleFromTranscriptPath: func(transcriptPath string) string {
			return string(session.InferRoleFromTranscriptPath(transcriptPath))
		},
		ToolInputSummary: hooks.ToolInputSummary,
		ReadHookStdin:    hooks.ReadHookStdin,
		CCCPath:          tmux.CCCPath,
	}
}

func deliverUnsentTexts(cfg *configpkg.Config, sessName string, topicID int64, transcriptPath string, insertIntoToolMsg bool, claudeSessionID string) int {
	return hooks.DeliverUnsentTexts(&hooks.DeliverUnsentTextsConfig{
		Config:             cfg,
		SessionName:        sessName,
		TopicID:            topicID,
		TranscriptPath:     transcriptPath,
		InsertIntoToolMsg:  insertIntoToolMsg,
		ClaudeSessionID:    claudeSessionID,
		LoadToolState:      hooks.LoadToolState,
		AddTextToToolState: hooks.AddTextToToolState,
		SaveToolState:      hooks.SaveToolState,
		FormatToolMessage:  hooks.FormatToolMessage,
		EditMessageHTML:    telegram.EditMessageHTML,
		SendMessageGetID:   telegram.SendMessageGetID,
		IsDelivered:        ledger.IsDelivered,
		AppendMessage: func(msg *ledger.MessageRecord) {
			ledger.AppendMessage(msg)
		},
		ClearToolState:              hooks.ClearToolState,
		InferRoleFromTranscriptPath: session.InferRoleFromTranscriptPath,
	})
}

func handleStopRetry(sessName string, topicID int64, transcriptPath string) error {
	return hooks.HandleStopRetry(&hooks.HandleStopRetryConfig{
		SessionName:        sessName,
		TopicID:            topicID,
		TranscriptPath:     transcriptPath,
		LoadConfig:         configpkg.Load,
		DeliverUnsentTexts: deliverUnsentTexts,
	})
}

func installHooksToCurrentDir() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	cfg, err := configpkg.Load()
	if err != nil || cfg == nil {
		return hooks.InstallHooksToCurrentDir()
	}

	sessionName, sessionInfo := lookup.FindSessionForPath(cfg, cwd)
	providerName := ""
	if sessionInfo != nil {
		providerName = sessionInfo.ProviderName
	} else {
		providerName = cfg.ActiveProvider
	}
	backend := providerpkg.BackendClaude
	if sessionInfo == nil || providerName != "" {
		backend = providerpkg.BackendForName(cfg, providerName)
	}
	if !providerpkg.IsCodexBackend(backend) {
		return hooks.InstallHooksToCurrentDir()
	}

	if sessionName == "" {
		sessionName = "current"
	}
	effectiveInfo := &configpkg.SessionInfo{Path: cwd, ProviderName: providerName}
	if sessionInfo != nil {
		sessionCopy := *sessionInfo
		if sessionCopy.Path == "" {
			sessionCopy.Path = cwd
		}
		effectiveInfo = &sessionCopy
	}

	ensureCfg := &hooks.EnsureHooksForSessionConfig{
		Config:            cfg,
		SessionName:       sessionName,
		SessionInfo:       effectiveInfo,
		GetSessionWorkDir: lookup.GetSessionWorkDir,
	}
	if err := hooks.EnsureCodexHooksForSession(ensureCfg); err != nil {
		return err
	}
	projectPath := lookup.GetSessionWorkDir(cfg, sessionName, effectiveInfo)
	fmt.Printf("Codex hooks installed/trusted in %s/.codex/hooks.json\n", projectPath)
	return nil
}

func ensureHooksForSession(config *configpkg.Config, sessionName string, sessionInfo *configpkg.SessionInfo) error {
	cfg := &hooks.EnsureHooksForSessionConfig{
		Config:            config,
		SessionName:       sessionName,
		SessionInfo:       sessionInfo,
		GetSessionWorkDir: lookup.GetSessionWorkDir,
	}
	providerName := ""
	if sessionInfo != nil {
		providerName = sessionInfo.ProviderName
	} else if config != nil {
		providerName = config.ActiveProvider
	}
	backend := providerpkg.BackendClaude
	if sessionInfo == nil || providerName != "" {
		backend = providerpkg.BackendForName(config, providerName)
	}
	if providerpkg.IsCodexBackend(backend) {
		return hooks.EnsureCodexHooksForSession(cfg)
	}
	return hooks.EnsureHooksForSession(cfg)
}
