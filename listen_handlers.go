package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tuannvm/ccc/pkg/telegram"
	"github.com/tuannvm/ccc/pkg/tmux"
)

// Listen command handler helpers.

// handleStopCommand handles the /stop command - interrupt current Claude execution
func handleStopCommand(config *Config, chatID, threadID int64, isGroup bool) {
	if !isGroup {
		telegram.SendMessage(config, chatID, threadID, "ℹ️ /stop only works in group topics. Switch to a session topic to use this command.")
		return
	}
	if threadID == 0 {
		telegram.SendMessage(config, chatID, threadID, "ℹ️ /stop only works in session topics. Switch to a session topic (thread) to use this command.")
		return
	}

	sessName := getSessionByTopic(config, threadID)
	if sessName == "" {
		telegram.SendMessage(config, chatID, threadID, "❌ No session mapped to this topic.")
		return
	}

	if !tmux.SessionExists() {
		telegram.SendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	windowName := tmux.SafeName(sessName)
	cmd := exec.Command(tmux.TmuxPath, "list-windows", "-t", tmux.SessionName, "-F", "#{window_name}\t#{window_id}")
	out, err := cmd.Output()
	if err != nil {
		telegram.SendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	var target string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		if len(parts) == 2 && parts[0] == windowName {
			target = tmux.SessionName + ":" + windowName
			break
		}
	}
	if err := scanner.Err(); err != nil {
		telegram.SendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}
	if target == "" {
		telegram.SendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	if err := exec.Command(tmux.TmuxPath, "send-keys", "-t", target, "C-[").Run(); err != nil {
		telegram.SendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to send interrupt: %v", err))
		return
	}

	telegram.SendMessage(config, chatID, threadID, "⏹️ Interrupt sent")
}
