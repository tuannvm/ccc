package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// OTP permission request/response files
var otpRequestPrefix = filepath.Join(cacheDir(), "otp-request-")
var otpResponsePrefix = filepath.Join(cacheDir(), "otp-response-")
var otpGrantPrefix = filepath.Join(cacheDir(), "otp-grant-")

const otpGrantDuration = 5 * time.Minute
const otpPermissionTimeout = 5 * time.Minute

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

// validateOTP checks if a TOTP code is valid for the configured secret
func validateOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	return totp.Validate(code, secret)
}

// isOTPEnabled checks if OTP is configured
func isOTPEnabled(config *Config) bool {
	return config.OTPSecret != ""
}

// setupOTP generates a new OTP secret, saves it, and returns instructions
func setupOTP(config *Config) (string, error) {
	secret, uri, err := generateOTPSecret()
	if err != nil {
		return "", err
	}

	config.OTPSecret = secret
	if err := saveConfig(config); err != nil {
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
func writeOTPRequest(sessionID string, req *OTPPermissionRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return os.WriteFile(otpRequestPrefix+sessionID, data, 0600)
}

// writeOTPResponse writes a permission response file for the hook to read
func writeOTPResponse(sessionID string, approved bool) error {
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
func waitForOTPResponse(sessionID, tmuxName string, timeout time.Duration) (bool, error) {
	responsePath := otpResponsePrefix + sessionID
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if another parallel hook already got approved and wrote a grant
		if hasValidOTPGrant(tmuxName) {
			os.Remove(otpRequestPrefix + sessionID)
			return true, nil
		}

		data, err := os.ReadFile(responsePath)
		if err == nil {
			// Clean up files
			os.Remove(responsePath)
			os.Remove(otpRequestPrefix + sessionID)

			var resp OTPPermissionResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return false, err
			}
			return resp.Approved, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Clean up on timeout
	os.Remove(otpRequestPrefix + sessionID)
	return false, fmt.Errorf("OTP timeout")
}

// getPendingOTPRequest reads a pending OTP request for a session
func getPendingOTPRequest(sessionID string) (*OTPPermissionRequest, error) {
	data, err := os.ReadFile(otpRequestPrefix + sessionID)
	if err != nil {
		return nil, err
	}
	var req OTPPermissionRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// findPendingOTPSession finds which session has a pending OTP request
func findPendingOTPSession() string {
	matches, err := filepath.Glob(otpRequestPrefix + "*")
	if err != nil || len(matches) == 0 {
		return ""
	}
	for _, match := range matches {
		sessionID := strings.TrimPrefix(match, otpRequestPrefix)
		return sessionID
	}
	return ""
}

// hasValidOTPGrant checks if there's a valid (non-expired) OTP grant for a tmux session
func hasValidOTPGrant(tmuxName string) bool {
	info, err := os.Stat(otpGrantPrefix + tmuxName)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < otpGrantDuration
}

// writeOTPGrant creates/refreshes a grant file for a tmux session
func writeOTPGrant(tmuxName string) {
	os.WriteFile(otpGrantPrefix+tmuxName, []byte("1"), 0600)
}
