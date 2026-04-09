package main

import (
	teampkg "github.com/tuannvm/ccc/pkg/team"
)

type TeamCommands = teampkg.Commands

func NewTeamCommands() *TeamCommands {
	return teampkg.NewCommands()
}
