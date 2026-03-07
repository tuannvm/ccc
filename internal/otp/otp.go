package otp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"
	"github.com/kidandcat/ccc/internal/config"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// OTP permission request/response files
var OTPRequestPrefix = filepath.Join(config.CacheDir(), "otp-request-")
var otpResponsePrefix = filepath.Join(config.CacheDir(), "otp-response-")
var otpGrantPrefix = filepath.Join(config.CacheDir(), "otp-grant-")

const OTPGrantDuration = 5 * time.Minute
const OTPPermissionTimeout = 5 * time.Minute

// OTPPermissionRequest is written by the hook to request OTP approval
type OTPPermissionRequest struct {
	SessionName string `json:"session_name"`
	ToolName    string `json:"tool_name"`
	ToolInput   string `json:"tool_input"`
	Timestamp   int64  `json:"timestamp"`
}

// OTPPermissionResponse is written by the listener after OTP validation
type OTPPermissionResponse struct {
	Approved  bool  `json:"approved"`
	Timestamp int64 `json:"timestamp"`
}

// generateOTPSecret creates a new TOTP secret and returns the provisioning URI
func generateOTPSecret() (secret string, provisioningURI string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "CCC",
		AccountName: "claude-code-companion",
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP key: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// ValidateOTP checks if a TOTP code is valid for the configured secret
func ValidateOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	return totp.Validate(code, secret)
}

// isOTPEnabled checks if OTP is configured
func IsOTPEnabled(config *config.Config) bool {
	return config.OTPSecret != ""
}

// setupOTP generates a new OTP secret, saves it, and returns instructions
func SetupOTP(cfg *config.Config) (string, error) {
	secret, uri, err := generateOTPSecret()
	if err != nil {
		return "", err
	}

	cfg.OTPSecret = secret
	if err := config.SaveConfig(cfg); err != nil {
		return "", fmt.Errorf("failed to save config: %w", err)
	}

	// Print QR code to terminal
	fmt.Println("\nScan this QR code with your authenticator app:")
	fmt.Println()
	qrterminal.GenerateHalfBlock(uri, qrterminal.L, os.Stdout)

	msg := fmt.Sprintf("Or enter the secret manually: %s", secret)
	return msg, nil
}

// writeOTPRequest writes a permission request file for the listener to pick up
func WriteOTPRequest(sessionID string, req *OTPPermissionRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return os.WriteFile(OTPRequestPrefix+sessionID, data, 0600)
}

// WriteOTPResponse writes a permission response file for the hook to read
func WriteOTPResponse(sessionID string, approved bool) error {
	resp := OTPPermissionResponse{
		Approved:  approved,
		Timestamp: time.Now().Unix(),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return os.WriteFile(otpResponsePrefix+sessionID, data, 0600)
}

// waitForOTPResponse waits for the listener to write a response file.
// It also checks for a valid grant (written by another parallel hook that was approved first).
func WaitForOTPResponse(sessionID, tmuxName string, timeout time.Duration) (bool, error) {
	responsePath := otpResponsePrefix + sessionID
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if another parallel hook already got approved and wrote a grant
		if HasValidOTPGrant(tmuxName) {
			os.Remove(OTPRequestPrefix + sessionID)
			return true, nil
		}

		data, err := os.ReadFile(responsePath)
		if err == nil {
			// Clean up files
			os.Remove(responsePath)
			os.Remove(OTPRequestPrefix + sessionID)

			var resp OTPPermissionResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return false, err
			}
			return resp.Approved, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Clean up on timeout
	os.Remove(OTPRequestPrefix + sessionID)
	return false, fmt.Errorf("OTP timeout")
}

// getPendingOTPRequest reads a pending OTP request for a session
func getPendingOTPRequest(sessionID string) (*OTPPermissionRequest, error) {
	data, err := os.ReadFile(OTPRequestPrefix + sessionID)
	if err != nil {
		return nil, err
	}
	var req OTPPermissionRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// FindPendingOTPSession finds which session has a pending OTP request
func FindPendingOTPSession() string {
	matches, err := filepath.Glob(OTPRequestPrefix + "*")
	if err != nil || len(matches) == 0 {
		return ""
	}
	for _, match := range matches {
		sessionID := strings.TrimPrefix(match, OTPRequestPrefix)
		return sessionID
	}
	return ""
}

// hasValidOTPGrant checks if there's a valid (non-expired) OTP grant for a tmux session
func HasValidOTPGrant(tmuxName string) bool {
	info, err := os.Stat(otpGrantPrefix + tmuxName)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < OTPGrantDuration
}

// writeOTPGrant creates/refreshes a grant file for a tmux session
func WriteOTPGrant(tmuxName string) {
	os.WriteFile(otpGrantPrefix+tmuxName, []byte("1"), 0600)
}
