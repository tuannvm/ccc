package listen

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
)

const newSessionCallbackTTL = 24 * time.Hour

type newSessionCallback struct {
	Action       string `json:"action"`
	SessionName  string `json:"session_name"`
	AgentName    string `json:"agent_name,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

func newSessionCallbackPath() string {
	return filepath.Join(configpkg.CacheDir(), "new-session-callbacks.json")
}

func newSessionCallbackData(record newSessionCallback) string {
	token, err := saveNewSessionCallback(record)
	if err != nil {
		return ""
	}
	return "new:" + token
}

func saveNewSessionCallback(record newSessionCallback) (string, error) {
	callbacks, err := loadNewSessionCallbacks()
	if err != nil {
		callbacks = make(map[string]newSessionCallback)
	}
	now := time.Now()
	for token, callback := range callbacks {
		if now.Sub(time.Unix(callback.CreatedAt, 0)) > newSessionCallbackTTL {
			delete(callbacks, token)
		}
	}
	if record.CreatedAt == 0 {
		record.CreatedAt = now.Unix()
	}
	for i := 0; i < 5; i++ {
		token, err := randomCallbackToken()
		if err != nil {
			return "", err
		}
		if _, exists := callbacks[token]; exists {
			continue
		}
		callbacks[token] = record
		return token, saveNewSessionCallbacks(callbacks)
	}
	return "", fmt.Errorf("failed to allocate callback token")
}

func loadNewSessionCallback(token string) (newSessionCallback, bool) {
	callbacks, err := loadNewSessionCallbacks()
	if err != nil {
		return newSessionCallback{}, false
	}
	callback, ok := callbacks[token]
	if !ok {
		return newSessionCallback{}, false
	}
	if time.Since(time.Unix(callback.CreatedAt, 0)) > newSessionCallbackTTL {
		delete(callbacks, token)
		_ = saveNewSessionCallbacks(callbacks)
		return newSessionCallback{}, false
	}
	return callback, true
}

func loadNewSessionCallbacks() (map[string]newSessionCallback, error) {
	data, err := os.ReadFile(newSessionCallbackPath())
	if os.IsNotExist(err) {
		return make(map[string]newSessionCallback), nil
	}
	if err != nil {
		return nil, err
	}
	var callbacks map[string]newSessionCallback
	if err := json.Unmarshal(data, &callbacks); err != nil {
		return nil, err
	}
	if callbacks == nil {
		callbacks = make(map[string]newSessionCallback)
	}
	return callbacks, nil
}

func saveNewSessionCallbacks(callbacks map[string]newSessionCallback) error {
	data, err := json.MarshalIndent(callbacks, "", "  ")
	if err != nil {
		return err
	}
	path := newSessionCallbackPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func randomCallbackToken() (string, error) {
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
