//go:build voice

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mutablelogic/go-whisper/pkg/schema"
	whisper "github.com/mutablelogic/go-whisper/pkg/whisper"
)

const voiceSupported = true

const whisperModelName = "ggml-small.bin"
const whisperModelURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin"

func getModelsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ccc", "models")
}

// ensureModel downloads the whisper model if not present
func ensureModel() (string, error) {
	modelsDir := getModelsDir()
	modelPath := filepath.Join(modelsDir, whisperModelName)
	if _, err := os.Stat(modelPath); err == nil {
		return modelPath, nil
	}

	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create models dir: %w", err)
	}

	fmt.Printf("Downloading whisper model %s...\n", whisperModelName)
	resp, err := http.Get(whisperModelURL)
	if err != nil {
		return "", fmt.Errorf("failed to download model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download model: HTTP %d", resp.StatusCode)
	}

	tmpPath := modelPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to create model file: %w", err)
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write model: %w", err)
	}

	if err := os.Rename(tmpPath, modelPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to rename model: %w", err)
	}

	fmt.Printf("Model downloaded: %s (%d MB)\n", whisperModelName, written/1024/1024)
	return modelPath, nil
}

// transcribeAudio transcribes audio using native go-whisper
func transcribeAudio(config *Config, audioPath string) (string, error) {
	modelsDir := getModelsDir()

	// Ensure model exists
	if _, err := ensureModel(); err != nil {
		return "", fmt.Errorf("model setup failed: %w", err)
	}

	manager, err := whisper.New(modelsDir)
	if err != nil {
		return "", fmt.Errorf("failed to create whisper manager: %w", err)
	}
	defer manager.Close()

	model := manager.GetModelById("ggml-small")
	if model == nil {
		return "", fmt.Errorf("model ggml-small not found in %s", modelsDir)
	}

	var result strings.Builder
	err = manager.WithModel(model, func(task *whisper.Task) error {
		if config.TranscriptionLang != "" {
			if err := task.SetLanguage(config.TranscriptionLang); err != nil {
				return fmt.Errorf("failed to set language: %w", err)
			}
		}
		f, err := os.Open(audioPath)
		if err != nil {
			return fmt.Errorf("failed to open audio: %w", err)
		}
		defer f.Close()
		return task.TranscribeReader(context.Background(), f, func(seg *schema.Segment) {
			result.WriteString(seg.Text)
		})
	})
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}

	return strings.TrimSpace(result.String()), nil
}

func doctorCheckWhisper() {
	fmt.Print("whisper model..... ")
	modelPath := filepath.Join(getModelsDir(), whisperModelName)
	if _, err := os.Stat(modelPath); err == nil {
		fmt.Printf("✅ %s\n", modelPath)
	} else {
		fmt.Println("⚠️  not downloaded (will auto-download on first voice message)")
		fmt.Println("   Model: " + whisperModelName)
	}
}
