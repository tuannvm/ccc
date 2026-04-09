package main

import (
	setuppkg "github.com/tuannvm/ccc/pkg/setup"
	"github.com/tuannvm/ccc/pkg/hooks"
)

func setup(botToken string) error {
	return setuppkg.Setup(botToken, hooks.InstallSkill)
}
