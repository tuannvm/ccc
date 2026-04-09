package main

import (
	"github.com/tuannvm/ccc/pkg/lookup"
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
	"github.com/tuannvm/ccc/pkg/tmux"
)

func startSession(continueSession bool) error {
	return listenpkg.StartSession(continueSession, tmux.AttachToSession)
}

func startSessionInCurrentDir(message string) error {
	return listenpkg.StartSessionInCurrentDirAuto(message, tmux.AttachToSession,
		lookup.FindSessionForPath, lookup.GenerateUniqueSessionName)
}
