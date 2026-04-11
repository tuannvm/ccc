package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/hooks"
	"github.com/tuannvm/ccc/pkg/telegram"

	qrterminal "github.com/mdp/qrterminal/v3"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// OTP file prefixes for permission request/response
var (
	OTPRequestPrefix  = filepath.Join(config.CacheDir(), "otp-request-")
	OTPResponsePrefix = filepath.Join(config.CacheDir(), "otp-response-")
	OTPGrantPrefix    = filepath.Join(config.CacheDir(), "otp-grant-")
)

// OTP durations
const (
	OTPGrantDuration       = 5 * time.Minute
	OTPPermissionTimeout   = 5 * time.Minute
)

// OTPPermissionResponse is written by the listener after OTP validation
type OTPPermissionResponse struct {
	Approved  bool  `json:"approved"`
	Timestamp int64 `json:"timestamp"`
}

// GenerateOTPSecret creates a new TOTP secret and returns the provisioning URI
func GenerateOTPSecret() (secret string, provisioningURI string, err error) {
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

// IsOTPEnabled checks if OTP is configured
func IsOTPEnabled(cfg *config.Config) bool {
	return cfg.OTPSecret != ""
}

// SetupOTP generates a new OTP secret, saves it, and returns instructions
func SetupOTP(cfg *config.Config) (string, error) {
	secret, uri, err := GenerateOTPSecret()
	if err != nil {
		return "", err
	}

	cfg.OTPSecret = secret
	if err := config.Save(cfg); err != nil {
		return "", fmt.Errorf("failed to save config: %w", err)
	}

	// Print QR code to terminal
	fmt.Println("\nScan this QR code with your authenticator app:")
	fmt.Println()
	qrterminal.GenerateHalfBlock(uri, qrterminal.L, os.Stdout)

	msg := fmt.Sprintf("Or enter the secret manually: %s", secret)
	return msg, nil
}

// WriteOTPRequest writes a permission request file for the listener to pick up
func WriteOTPRequest(sessionID string, req *hooks.OTPPermissionRequest) error {
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
	return os.WriteFile(OTPResponsePrefix+sessionID, data, 0600)
}

// WaitForOTPResponse waits for the listener to write a response file.
// It also checks for a valid grant (written by another parallel hook that was approved first).
func WaitForOTPResponse(sessionID, tmuxName string, timeout time.Duration) (bool, error) {
	responsePath := OTPResponsePrefix + sessionID
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

// GetPendingOTPRequest reads a pending OTP request for a session
func GetPendingOTPRequest(sessionID string) (*hooks.OTPPermissionRequest, error) {
	data, err := os.ReadFile(OTPRequestPrefix + sessionID)
	if err != nil {
		return nil, err
	}
	var req hooks.OTPPermissionRequest
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

// HasValidOTPGrant checks if there's a valid (non-expired) OTP grant for a tmux session
func HasValidOTPGrant(tmuxName string) bool {
	info, err := os.Stat(OTPGrantPrefix + tmuxName)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < OTPGrantDuration
}

// WriteOTPGrant creates/refreshes a grant file for a tmux session
func WriteOTPGrant(tmuxName string) {
	os.WriteFile(OTPGrantPrefix+tmuxName, []byte("1"), 0600)
}

// IsAuthorizedCallback checks if a callback query is authorized based on multi-user mode
func IsAuthorizedCallback(cfg *config.Config, cb *telegram.CallbackQuery) bool {
	if cb == nil || cb.Message == nil {
		return false
	}
	if cfg.MultiUserMode {
		if cfg.GroupID == 0 {
			return false
		}
		return cb.Message.Chat.ID == cfg.GroupID
	}
	return cb.From.ID == cfg.ChatID
}

// IsAuthorizedMessage checks if a message is authorized based on multi-user mode
func IsAuthorizedMessage(cfg *config.Config, msg telegram.TelegramMessage) bool {
	if cfg.MultiUserMode {
		if cfg.GroupID == 0 {
			return false
		}
		return msg.Chat.ID == cfg.GroupID
	}
	return msg.From.ID == cfg.ChatID
}
