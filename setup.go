package main

import (
	setuppkg "github.com/tuannvm/ccc/pkg/setup"
	"github.com/tuannvm/ccc/pkg/hooks"
)

// Setup and group configuration delegates — logic in pkg/setup/setup.go

func setup(botToken string) error {
	return setuppkg.Setup(botToken, hooks.InstallSkill)
}

func setGroup(config *Config) error {
	return setuppkg.SetGroup(config)
}
