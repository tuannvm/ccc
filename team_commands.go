package main

import (
	teampkg "github.com/tuannvm/ccc/pkg/team"
)

// TeamCommands is an alias for pkg/team.Commands
type TeamCommands = teampkg.Commands

// NewTeamCommands delegates to pkg/team.NewCommands
func NewTeamCommands() *TeamCommands {
	return teampkg.NewCommands()
}
