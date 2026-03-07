//go:build !voice

package whisper

import (
	"fmt"

	"github.com/kidandcat/ccc/internal/config"
)

const voiceSupported = false

// TranscribeAudio is a stub when built without voice support
func TranscribeAudio(cfg *config.Config, audioPath string) (string, error) {
	return "", fmt.Errorf("voice transcription not available (build with: go build -tags voice)")
}

func DoctorCheckWhisper() {
	fmt.Println("whisper........... ⚠️  not compiled (build with: go build -tags voice)")
}
