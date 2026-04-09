package main

import (
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/ledger"
	"github.com/tuannvm/ccc/pkg/provider"
	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/session"
)

type Config = configpkg.Config
type SessionInfo = configpkg.SessionInfo
type ProviderConfig = configpkg.ProviderConfig

var _ session.Session = (*SessionInfo)(nil)

type TelegramMessage = telegram.TelegramMessage
type TelegramResponse = telegram.TelegramResponse
type TopicResult = telegram.TopicResult

type HookData = hooks.HookData
type ToolState = hooks.ToolState

type MessageRecord = ledger.MessageRecord

type Provider = provider.Provider
type OTPPermissionRequest = hooks.OTPPermissionRequest
