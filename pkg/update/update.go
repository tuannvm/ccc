package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

// DownloadResult holds the result of a binary download.
type DownloadResult struct {
	TmpPath string
	Size    int64
}

// DownloadBinary downloads a binary from the given URL to a temporary file next to the target path.
// Returns the temp file path and downloaded size.
func DownloadBinary(downloadURL, targetPath string) (*DownloadResult, error) {
	resp, err := http.Get(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	tmpPath := targetPath + ".new"
	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to write binary: %w", err)
	}

	return &DownloadResult{TmpPath: tmpPath, Size: written}, nil
}

// ValidateBinary checks that a downloaded binary is valid (>1MB and can run).
func ValidateBinary(path string) error {
	// Check minimum size
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat binary: %w", err)
	}
	if info.Size() < 1000000 {
		return fmt.Errorf("downloaded file too small (%d bytes)", info.Size())
	}

	// Make executable
	if err := os.Chmod(path, 0755); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	// Test the binary runs
	testCmd := exec.Command(path, "version")
	if err := testCmd.Run(); err != nil {
		return fmt.Errorf("binary failed validation: %w", err)
	}

	return nil
}

// ReplaceBinary backs up the old binary and replaces it with the new one.
// On macOS, codesigns the new binary.
func ReplaceBinary(oldPath, newPath string) error {
	// Backup old binary
	backupPath := oldPath + ".bak"
	os.Remove(backupPath)
	if err := os.Rename(oldPath, backupPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("failed to backup old binary: %w", err)
	}

	// Replace with new binary
	if err := os.Rename(newPath, oldPath); err != nil {
		// Restore backup
		os.Rename(backupPath, oldPath)
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	// Codesign on macOS
	if runtime.GOOS == "darwin" {
		if err := exec.Command("codesign", "-f", "-s", "-", oldPath).Run(); err != nil {
			// Restore backup if codesign fails
			os.Remove(oldPath)
			os.Rename(backupPath, oldPath)
			return fmt.Errorf("codesign failed: %w", err)
		}
	}

	// Success - remove backup
	os.Remove(backupPath)
	return nil
}

// BuildDownloadURL constructs the download URL for the latest ccc binary.
func BuildDownloadURL() string {
	binaryName := fmt.Sprintf("ccc-%s-%s", runtime.GOOS, runtime.GOARCH)
	return fmt.Sprintf("https://github.com/tuannvm/ccc/releases/latest/download/%s", binaryName)
}
