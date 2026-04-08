package main

import (
	"github.com/tuannvm/ccc/pkg/config"
)

// loadConfig reads and parses the config file, handling migration from old format
func loadConfig() (*Config, error) {
	return config.Load()
}
