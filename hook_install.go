package main

import (
	"github.com/tuannvm/ccc/pkg/hooks"
)

func installSkill() error {
	return hooks.InstallSkill()
}

func installHooksToCurrentDir() error {
	return hooks.InstallHooksToCurrentDir()
}

// ensureHooksForSession ensures ccc hooks are installed in the session's project directory
func ensureHooksForSession(config *Config, sessionName string, sessionInfo *SessionInfo) error {
	return hooks.EnsureHooksForSession(&hooks.EnsureHooksForSessionConfig{
		Config:            config,
		SessionName:       sessionName,
		SessionInfo:       sessionInfo,
		GetSessionWorkDir: getSessionWorkDir,
	})
}
