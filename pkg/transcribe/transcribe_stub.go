//go:build !voice

package transcribe

import (
	"fmt"

	"github.com/tuannvm/ccc/pkg/config"
)

const VoiceSupported = false

// TranscribeAudio is a stub when built without voice support
func TranscribeAudio(cfg *config.Config, audioPath string) (string, error) {
	return "", fmt.Errorf("voice transcription not available (build with: go build -tags voice)")
}

// DoctorCheckWhisper is a stub when built without voice support
func DoctorCheckWhisper() {
	fmt.Println("whisper........... ⚠️  not compiled (build with: go build -tags voice)")
}
