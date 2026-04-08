package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// InstallService installs the ccc background service (launchd on macOS, systemd on Linux)
func InstallService() error {
	home, _ := os.UserHomeDir()

	// Detect OS and install appropriate service
	if _, err := os.Stat("/Library"); err == nil {
		// macOS - use launchd
		return installLaunchdService(home)
	}
	// Linux - use systemd
	return installSystemdService(home)
}

func installLaunchdService(home string) error {
	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents dir: %w", err)
	}

	plistPath := filepath.Join(plistDir, "com.ccc.plist")
	logPath := filepath.Join(configpkg.CacheDir(), "ccc.log")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ccc</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>listen</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, tmux.CCCPath, logPath, logPath)

	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	// Unload if exists, then load
	exec.Command("launchctl", "unload", plistPath).Run()
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load service: %w", err)
	}

	fmt.Println("✅ Service installed and started (launchd)")
	return nil
}

func installSystemdService(home string) error {
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd dir: %w", err)
	}

	servicePath := filepath.Join(serviceDir, "ccc.service")
	service := fmt.Sprintf(`[Unit]
Description=Claude Code Companion
After=network.target

[Service]
ExecStart=%s listen
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
`, tmux.CCCPath)

	if err := os.WriteFile(servicePath, []byte(service), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload and start
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	exec.Command("systemctl", "--user", "enable", "ccc").Run()
	if err := exec.Command("systemctl", "--user", "start", "ccc").Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Println("✅ Service installed and started (systemd)")
	return nil
}

// StopListenerService stops the background listener service
func StopListenerService() {
	home, _ := os.UserHomeDir()
	if _, err := os.Stat("/Library"); err == nil {
		// macOS - launchd
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ccc.plist")
		exec.Command("launchctl", "unload", plistPath).Run()
	} else {
		// Linux - systemd
		exec.Command("systemctl", "--user", "stop", "ccc").Run()
	}
	// Also kill any manual listener via lock file PID
	lockPath := filepath.Join(configpkg.CacheDir(), "ccc.lock")
	if data, err := os.ReadFile(lockPath); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pidStr != "" {
			exec.Command("kill", pidStr).Run()
		}
	}
	time.Sleep(500 * time.Millisecond)
}

// StartListenerService starts the background listener service
func StartListenerService() {
	home, _ := os.UserHomeDir()
	if _, err := os.Stat("/Library"); err == nil {
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ccc.plist")
		exec.Command("launchctl", "load", plistPath).Run()
	} else {
		exec.Command("systemctl", "--user", "start", "ccc").Run()
	}
}
