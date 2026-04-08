package main

import (
	"github.com/tuannvm/ccc/pkg/config"
)

// saveConfig atomically writes the config to disk using write-then-rename pattern
// Multiple processes may write config simultaneously; atomic rename prevents corruption
func saveConfig(c *Config) error {
	return config.Save(c)
}
