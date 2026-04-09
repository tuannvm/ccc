package main

import (
	listenpkg "github.com/tuannvm/ccc/pkg/listen"
)

func listen() error {
	return listenpkg.Run(version)
}
