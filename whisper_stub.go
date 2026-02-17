//go:build !voice

package main

import "fmt"

const voiceSupported = false

// transcribeAudio is a stub when built without voice support
func transcribeAudio(config *Config, audioPath string) (string, error) {
	return "", fmt.Errorf("voice transcription not available (build with: go build -tags voice)")
}

func doctorCheckWhisper() {
	fmt.Println("whisper........... ⚠️  not compiled (build with: go build -tags voice)")
}
