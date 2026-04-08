package main

import (
	"github.com/tuannvm/ccc/pkg/telegram"
)

// BufferedStreamer accumulates text chunks and streams them
type BufferedStreamer = telegram.BufferedStreamer

// NewBufferedStreamer creates a new buffered streamer
func NewBufferedStreamer(config *Config, chatID int64, threadID int64, enabled bool) *BufferedStreamer {
	return telegram.NewBufferedStreamer(config, chatID, threadID, enabled)
}
