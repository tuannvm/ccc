package main

import "github.com/tuannvm/ccc/pkg/relay"

// handleSendFile sends a file to the current session's Telegram topic
func handleSendFile(filePath string) error {
	return relay.HandleSendFile(filePath)
}

func runRelayServer(port string) {
	relay.RunRelayServer(port)
}
