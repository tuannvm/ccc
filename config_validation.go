package main

import (
	"github.com/tuannvm/ccc/pkg/config"
)

// validateConfig checks if the config is valid and returns any errors
// It validates required fields for configured features and provider configs
func validateConfig(c *Config) error {
	return config.Validate(c)
}
