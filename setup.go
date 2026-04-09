package main

import (
	setuppkg "github.com/tuannvm/ccc/pkg/setup"
	"github.com/tuannvm/ccc/pkg/hooks"
)

func setupFromArgs(args []string) error {
	return setuppkg.SetupFromArgs(args, hooks.InstallSkill)
}
