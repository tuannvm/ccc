package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Listen command handler helpers.

// handleStopCommand handles the /stop command - interrupt current Claude execution
func handleStopCommand(config *Config, chatID, threadID int64, isGroup bool) {
	if !isGroup {
		sendMessage(config, chatID, threadID, "ℹ️ /stop only works in group topics. Switch to a session topic to use this command.")
		return
	}
	if threadID == 0 {
		sendMessage(config, chatID, threadID, "ℹ️ /stop only works in session topics. Switch to a session topic (thread) to use this command.")
		return
	}

	sessName := getSessionByTopic(config, threadID)
	if sessName == "" {
		sendMessage(config, chatID, threadID, "❌ No session mapped to this topic.")
		return
	}

	if !cccSessionExists() {
		sendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	windowName := tmuxSafeName(sessName)
	cmd := exec.Command(tmuxPath, "list-windows", "-t", cccSessionName, "-F", "#{window_name}\t#{window_id}")
	out, err := cmd.Output()
	if err != nil {
		sendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	var target string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		if len(parts) == 2 && parts[0] == windowName {
			target = cccSessionName + ":" + windowName
			break
		}
	}
	if err := scanner.Err(); err != nil {
		sendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}
	if target == "" {
		sendMessage(config, chatID, threadID, "❌ No active tmux window for this session.")
		return
	}

	if err := exec.Command(tmuxPath, "send-keys", "-t", target, "C-[").Run(); err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to send interrupt: %v", err))
		return
	}

	sendMessage(config, chatID, threadID, "⏹️ Interrupt sent")
}
