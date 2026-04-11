package listen

import "strings"

// StripBotMention removes @botname from commands (e.g., "/ping@botname" -> "/ping").
func StripBotMention(text string) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}
	if idx := strings.Index(text, "@"); idx != -1 {
		spaceIdx := strings.Index(text, " ")
		if spaceIdx == -1 || idx < spaceIdx {
			text = text[:idx] + text[strings.Index(text+" ", " "):]
		}
	}
	return strings.TrimSpace(text)
}
