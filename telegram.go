package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxResponseSize = 10 * 1024 * 1024 // 10MB

// redactTokenError replaces the bot token in error messages with "***"
func redactTokenError(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "***"))
}

// telegramGet performs an HTTP GET and redacts the bot token from any errors
func telegramGet(token string, url string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, redactTokenError(err, token)
	}
	return resp, nil
}

// telegramClientGet performs an HTTP GET with a custom client and redacts the bot token from any errors
func telegramClientGet(client *http.Client, token string, url string) (*http.Response, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, redactTokenError(err, token)
	}
	return resp, nil
}

// updateCCC downloads the latest ccc binary from GitHub releases and restarts
func updateCCC(config *Config, chatID, threadID int64, offset int) {
	sendMessage(config, chatID, threadID, "🔄 Updating ccc...")

	binaryName := fmt.Sprintf("ccc-%s-%s", runtime.GOOS, runtime.GOARCH)
	downloadURL := fmt.Sprintf("https://github.com/kidandcat/ccc/releases/latest/download/%s", binaryName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Download failed: HTTP %d (no release for %s?)", resp.StatusCode, binaryName))
		return
	}

	tmpPath := cccPath + ".new"
	f, err := os.Create(tmpPath)
	if err != nil {
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to create temp file: %v", err))
		return
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to write binary: %v", err))
		return
	}

	// Validate downloaded binary size (ccc should be > 1MB)
	if written < 1000000 {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Downloaded file too small (%d bytes), aborting", written))
		return
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to chmod: %v", err))
		return
	}

	// Test the new binary before replacing
	testCmd := exec.Command(tmpPath, "version")
	if err := testCmd.Run(); err != nil {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ New binary failed validation: %v", err))
		return
	}

	// Backup old binary
	backupPath := cccPath + ".bak"
	os.Remove(backupPath) // Remove old backup if exists
	if err := os.Rename(cccPath, backupPath); err != nil {
		os.Remove(tmpPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to backup old binary: %v", err))
		return
	}

	// Replace with new binary
	if err := os.Rename(tmpPath, cccPath); err != nil {
		// Restore backup
		os.Rename(backupPath, cccPath)
		sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Failed to replace binary: %v", err))
		return
	}

	// Codesign on macOS
	if runtime.GOOS == "darwin" {
		if err := exec.Command("codesign", "-f", "-s", "-", cccPath).Run(); err != nil {
			// Restore backup if codesign fails
			os.Remove(cccPath)
			os.Rename(backupPath, cccPath)
			sendMessage(config, chatID, threadID, fmt.Sprintf("❌ Codesign failed: %v", err))
			return
		}
	}

	// Success - remove backup
	os.Remove(backupPath)

	sendMessage(config, chatID, threadID, "✅ Updated. Restarting...")
	// Confirm offset so the /update message is not reprocessed after restart
	http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=1", config.BotToken, offset))
	os.Exit(0)
}

func telegramAPI(config *Config, method string, params url.Values) (*TelegramResponse, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", config.BotToken, method)
	resp, err := http.PostForm(apiURL, params)
	if err != nil {
		return nil, redactTokenError(err, config.BotToken)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	var result TelegramResponse
	json.Unmarshal(body, &result)
	return &result, nil
}

// EscapeMarkdownV2 escapes special characters for Telegram MarkdownV2 format
// Special characters that must be escaped: _ * [ ] ( ) ~ ` > # + - = | { } . !
func EscapeMarkdownV2(text string) string {
	// Must escape in this order to avoid double-escaping
	// Backslash must be first to avoid escaping the escapes we add
	replacements := []struct{ old, new string }{
		{"\\", "\\\\"}, // Backslash MUST be first
		{"_", "\\_"},
		{"*", "\\*"},
		{"[", "\\["},
		{"]", "\\]"},
		{"(", "\\("},
		{")", "\\)"},
		{"~", "\\~"},
		{"`", "\\`"},
		{">", "\\>"},
		{"#", "\\#"},
		{"+", "\\+"},
		{"-", "\\-"},
		{"=", "\\="},
		{"|", "\\|"},
		{"{", "\\{"},
		{"}", "\\}"},
		{".", "\\."},
		{"!", "\\!"},
	}

	result := text
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.old, r.new)
	}
	return result
}

// escapeURL escapes special characters in URLs for MarkdownV2
func escapeURL(url string) string {
	var result strings.Builder
	for _, ch := range url {
		// Escape characters that need escaping in MarkdownV2 URLs
		switch ch {
		case ')', '\\', '`':
			result.WriteByte('\\')
		}
		result.WriteRune(ch)
	}
	return result.String()
}

// unescapeMarkdownV2 removes MarkdownV2 escape sequences for plain text fallback
func unescapeMarkdownV2(text string) string {
	var result strings.Builder
	i := 0
	for i < len(text) {
		if text[i] == '\\' && i+1 < len(text) {
			// Skip the backslash, keep the next character
			i++
		}
		result.WriteByte(text[i])
		i++
	}
	return result.String()
}

// isBoundaryChar checks if a character is a word boundary (whitespace, punctuation, etc.)
func isBoundaryChar(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '.' ||
		ch == ',' || ch == '!' || ch == '?' || ch == ';' || ch == ':' ||
		ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == '{' ||
		ch == '}' || ch == '<' || ch == '>' || ch == '|' || ch == '`'
}

// MarkdownToTelegramV2 converts standard Markdown to Telegram MarkdownV2 format
// It preserves formatting while escaping special characters in literal text
func MarkdownToTelegramV2(text string) string {
	var result strings.Builder
	i := 0
	n := len(text)

	for i < n {
		ch := text[i]

		// Handle code blocks (fenced)
		if i < n-2 && text[i:i+3] == "```" {
			// Find closing ```
			endIdx := strings.Index(text[i+3:], "```")
			if endIdx != -1 {
				// Extract the code block content (between ``` and ```)
				contentStart := i + 3
				contentEnd := i + 3 + endIdx
				content := text[contentStart:contentEnd]

				// Write opening ```
				result.WriteString("```")
				// Write escaped content
				result.WriteString(escapeCodeContent(content))
				// Write closing ```
				result.WriteString("```")
				i = contentEnd + 3
				continue
			}
		}

		// Handle inline code
		if ch == '`' {
			// Find closing backtick, skipping escaped ones
			j := i + 1
			for j < n {
				if text[j] == '`' {
					// Check if this backtick is escaped
					escaped := false
					backslashCount := 0
					for k := j - 1; k >= i+1 && text[k] == '\\'; k-- {
						backslashCount++
					}
					// If odd number of backslashes before this backtick, it's escaped
					if backslashCount%2 == 1 {
						escaped = true
					}

					if !escaped {
						// Found the real closing backtick
						content := text[i+1 : j]

						// Write opening `
						result.WriteByte('`')
						// Write escaped content
						result.WriteString(escapeCodeContent(content))
						// Write closing `
						result.WriteByte('`')
						i = j + 1
						break
					}
				}
				j++
			}
			if j < n {
				continue
			}
		}

		// Handle HTML tags (<tag>, </tag>, <tag />) - escape them for Telegram MarkdownV2
		if ch == '<' {
			// Find closing >
			endIdx := strings.Index(text[i:], ">")
			if endIdx != -1 {
				tag := text[i : i+endIdx+1]
				// Validate it's actually an HTML tag (not just < and > around text)
				if isHTMLTag(tag) {
					// Escape the entire HTML tag
					result.WriteString(escapeTextOnly(tag))
					i = i + endIdx + 1
					continue
				}
			}
		}

		// Handle bold (**text** or __text__)
		if i < n-1 && text[i:i+2] == "**" {
			endIdx := findClosing(text, i+2, "**")
			if endIdx != -1 {
				result.WriteByte('*') // Telegram uses single * for bold
				// Process content recursively (to handle nested formatting)
				content := MarkdownToTelegramV2(text[i+2 : endIdx])
				result.WriteString(content)
				result.WriteByte('*')
				i = endIdx + 2
				continue
			}
		}
		// Handle __bold__ (underscore variant)
		if i < n-1 && text[i:i+2] == "__" {
			endIdx := findClosing(text, i+2, "__")
			if endIdx != -1 {
				result.WriteByte('*') // Telegram uses single * for bold
				// Process content recursively (to handle nested formatting)
				content := MarkdownToTelegramV2(text[i+2 : endIdx])
				result.WriteString(content)
				result.WriteByte('*')
				i = endIdx + 2
				continue
			}
		}

		// Handle italic (*text*)
		if ch == '*' && (i == 0 || text[i-1] != '*') && (i+1 >= n || text[i+1] != '*') {
			// Check for word boundary before *
			hasBoundaryBefore := i == 0 || isBoundaryChar(text[i-1])
			if hasBoundaryBefore {
				endIdx := findClosing(text, i+1, "*")
				if endIdx != -1 {
					// Check for word boundary after *
					hasBoundaryAfter := endIdx+1 >= n || isBoundaryChar(text[endIdx+1])
					if hasBoundaryAfter {
						result.WriteByte('_') // Telegram uses _ for italic
						// Process content recursively
						content := MarkdownToTelegramV2(text[i+1 : endIdx])
						result.WriteString(content)
						result.WriteByte('_')
						i = endIdx + 1
						continue
					}
				}
			}
		}
		// Handle _italic_ (underscore variant)
		if ch == '_' && (i == 0 || text[i-1] != '_') && (i+1 >= n || text[i+1] != '_') {
			// Check for word boundary before _
			hasBoundaryBefore := i == 0 || isBoundaryChar(text[i-1])
			if hasBoundaryBefore {
				endIdx := findClosing(text, i+1, "_")
				if endIdx != -1 {
					// Check for word boundary after _
					hasBoundaryAfter := endIdx+1 >= n || isBoundaryChar(text[endIdx+1])
					if hasBoundaryAfter {
						result.WriteByte('_') // Telegram uses _ for italic
						// Process content recursively
						content := MarkdownToTelegramV2(text[i+1 : endIdx])
						result.WriteString(content)
						result.WriteByte('_')
						i = endIdx + 1
						continue
					}
				}
			}
		}

		// Handle links [text](url)
		if ch == '[' {
			endIdx := strings.Index(text[i:], "](")
			if endIdx != -1 {
				endIdx += i
				// Find the closing ), accounting for nested parens in URLs
				urlStart := endIdx + 2
				parensDepth := 0
				urlEndIdx := -1
				for j := urlStart; j < n; j++ {
					if text[j] == '(' {
						parensDepth++
					} else if text[j] == ')' {
						if parensDepth == 0 {
							urlEndIdx = j
							break
						}
						parensDepth--
					}
				}
				if urlEndIdx != -1 {
					// Process link text recursively (to handle **bold** inside links)
					linkText := text[i+1 : endIdx]
					processedLinkText := MarkdownToTelegramV2(linkText)
					result.WriteByte('[')
					result.WriteString(processedLinkText)
					result.WriteString("](")
					// Escape special characters in URL
					url := text[urlStart:urlEndIdx]
					result.WriteString(escapeURL(url))
					result.WriteByte(')')
					i = urlEndIdx + 1
					continue
				}
			}
		}

		// Handle headings (# at start of line)
		if ch == '#' && (i == 0 || text[i-1] == '\n') {
			// Count #s
			count := 0
			for i+count < n && text[i+count] == '#' {
				count++
			}
			// Check if there's whitespace after the #s (required for a valid heading)
			if i+count < n && (text[i+count] == ' ' || text[i+count] == '\t') {
				// Skip whitespace after #
				start := i + count
				for start < n && (text[start] == ' ' || text[start] == '\t') {
					start++
				}
				// Find end of line
				endLine := strings.Index(text[start:], "\n")
				if endLine == -1 {
					endLine = n
				} else {
					endLine += start
				}
				// Convert heading to bold, escaping all special characters
				result.WriteByte('*')
				headingText := text[start:endLine]
				// Escape all special characters in heading text
				for _, ch := range headingText {
				if isSpecialChar(byte(ch)) {
					result.WriteByte('\\')
				}
				result.WriteRune(ch)
			}
			result.WriteByte('*')
				// Only add newline if original had one
				if endLine < n && text[endLine] == '\n' {
					result.WriteByte('\n')
				}
				i = endLine
				if endLine < n && text[endLine] == '\n' {
					i++
				}
				continue
			}
		}

		// Handle blockquotes (> at start of line)
		if ch == '>' && (i == 0 || text[i-1] == '\n') {
			// Skip > and optional space
			start := i + 1
			if start < n && text[start] == ' ' {
				start++
			}
			// Find end of line
			endLine := strings.Index(text[start:], "\n")
			if endLine == -1 {
				endLine = n
			} else {
				endLine += start
			}
			// Strip quote prefix, just output the text with escaping
			quoteText := text[start:endLine]
			// Process the quoted text to escape special chars
			for _, ch := range quoteText {
				if isSpecialChar(byte(ch)) {
					result.WriteByte('\\')
				}
				result.WriteRune(ch)
			}
			result.WriteByte('\n')
			i = endLine
			if endLine < n && text[endLine] == '\n' {
				i++
			}
			continue
		}

		// Handle unordered lists (- at start of line)
		if ch == '-' && (i == 0 || text[i-1] == '\n') {
			result.WriteString("\\-") // Escape the bullet
			i++
			continue
		}

		// Handle ordered lists (digit. at start of line)
		if ch >= '0' && ch <= '9' && (i == 0 || text[i-1] == '\n') {
			if i+1 < n && text[i+1] == '.' {
				result.WriteByte(ch)
				result.WriteString("\\.") // Escape the dot
				i += 2
				continue
			}
		}

		// Handle horizontal rules (--- or ***)
		if i < n-2 && (text[i:i+3] == "---" || text[i:i+3] == "***") {
			if (i == 0 || text[i-1] == '\n') && (i+3 >= n || text[i+3] == '\n') {
				// Escape dashes for MarkdownV2
				if text[i:i+3] == "---" {
					result.WriteString("\\-\\-\\-")
				} else {
					// For ***, escape the asterisks
					result.WriteString("\\*\\*\\*")
				}
				i += 3
				continue
			}
		}

		// Default: escape special characters
		if isSpecialChar(ch) {
			result.WriteByte('\\')
		}
		result.WriteByte(ch)
		i++
	}

	return result.String()
}

// escapeTextOnly escapes special characters but preserves Markdown syntax
func escapeTextOnly(text string) string {
	var result strings.Builder
	for _, ch := range text {
		// Escape special characters that aren't part of Markdown syntax
		// We preserve: * [ ] ( ) ` as they might be formatting
		// We escape: _ . ! + = | { } ~ - # < >
		switch ch {
		case '_', '.', '!', '+', '=', '|', '{', '}', '~', '-', '#', '<', '>':
			result.WriteByte('\\')
		}
		result.WriteRune(ch)
	}
	return result.String()
}

// isSpecialChar checks if a character needs escaping in MarkdownV2
func isSpecialChar(ch byte) bool {
	return ch == '_' || ch == '*' || ch == '[' || ch == ']' || ch == '(' || ch == ')' ||
		ch == '`' || ch == '>' || ch == '#' || ch == '+' || ch == '-' || ch == '=' ||
		ch == '|' || ch == '{' || ch == '}' || ch == '.' || ch == '!' || ch == '~' || ch == '\\'
}

// findClosing finds the closing delimiter for bold/italic
func findClosing(text string, start int, delim string) int {
	i := start
	delimLen := len(delim)

	for i < len(text) {
		// Skip code blocks
		if i < len(text)-2 && text[i:i+3] == "```" {
			end := strings.Index(text[i+3:], "```")
			if end != -1 {
				i += end + 6
				continue
			}
		}

		// Skip inline code
		if text[i] == '`' {
			end := strings.Index(text[i+1:], "`")
			if end != -1 {
				i += end + 2
				continue
			}
		}

		// Check for closing delimiter
		if i <= len(text)-delimLen && text[i:i+delimLen] == delim {
			return i
		}

		i++
	}

	return -1
}

// isHTMLTag checks if the given text is a valid HTML tag
// It distinguishes between real HTML tags like <div> and comparisons like 1 < 2 > 1
func isHTMLTag(tag string) bool {
	if len(tag) < 3 {
		return false // Minimum valid tag is <a>
	}

	// Tags start with < and end with >
	if tag[0] != '<' || tag[len(tag)-1] != '>' {
		return false
	}

	// Get content between < and >
	content := tag[1 : len(tag)-1]
	if len(content) == 0 {
		return false
	}

	// Check if it starts with valid tag character
	// Valid: letters (a-z, A-Z), / (for closing tags), ! (for comments/DOCTYPE)
	firstChar := content[0]
	if !((firstChar >= 'a' && firstChar <= 'z') ||
		(firstChar >= 'A' && firstChar <= 'Z') ||
		firstChar == '/' || firstChar == '!') {
		return false
	}

	// Check if the tag name contains only valid characters
	// Valid tag characters: letters, digits, hyphen, underscore, slash, colon, dot
	for _, ch := range content {
		valid := (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '/' || ch == ':' || ch == '.' || ch == ' ' || ch == '=' || ch == '"' || ch == '\''
		if !valid {
			return false
		}
	}

	return true
}

// escapeCodeContent escapes special characters within code blocks/inline code for Telegram MarkdownV2
// Within code entities, backslashes and backticks need to be escaped
func escapeCodeContent(content string) string {
	var result strings.Builder
	for _, ch := range content {
		if ch == '\\' || ch == '`' {
			result.WriteByte('\\')
		}
		result.WriteRune(ch)
	}
	return result.String()
}

func sendMessage(config *Config, chatID int64, threadID int64, text string) error {
	_, err := sendMessageGetID(config, chatID, threadID, text)
	return err
}

// sendMessageGetID sends a message and returns the message ID for later editing
// Converts standard Markdown to Telegram MarkdownV2 format
func sendMessageGetID(config *Config, chatID int64, threadID int64, text string) (int64, error) {
	converted := MarkdownToTelegramV2(text)
	return sendMessageWithMode(config, chatID, threadID, converted, "MarkdownV2", false)
}

// sendMessageHTMLGetID sends a message with HTML parse mode (for system messages)
func sendMessageHTMLGetID(config *Config, chatID int64, threadID int64, text string) (int64, error) {
	return sendMessageWithMode(config, chatID, threadID, text, "HTML", false)
}

func sendMessageWithMode(config *Config, chatID int64, threadID int64, text string, parseMode string, escape bool) (int64, error) {
	const maxLen = 4000

	// Escape text if using MarkdownV2
	if escape && parseMode == "MarkdownV2" {
		text = EscapeMarkdownV2(text)
	}

	// Split long messages
	messages := splitMessage(text, maxLen)

	// For MarkdownV2, ensure no chunk ends with a backslash (broken escape sequence)
	if parseMode == "MarkdownV2" {
		messages = fixMarkdownV2Splits(messages)
	}

	var lastMsgID int64

	for _, msg := range messages {
		params := url.Values{
			"chat_id":    {fmt.Sprintf("%d", chatID)},
			"text":       {msg},
			"parse_mode": {parseMode},
		}
		if threadID > 0 {
			params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
		}

		result, err := telegramAPI(config, "sendMessage", params)
		if err != nil {
			return 0, err
		}
		if !result.OK {
			// If Markdown/HTML parsing fails, retry as plain text
			if strings.Contains(result.Description, "parse entities") && parseMode != "" {
				params.Del("parse_mode")
				// Remove escape sequences for MarkdownV2 fallback only
				var plainText string
				if parseMode == "MarkdownV2" {
					plainText = unescapeMarkdownV2(msg)
				} else {
					plainText = msg
				}
				params.Set("text", "⚠️\n[this message displayed as plain text, since markdown parse failed]\n\n"+plainText)
				result, err = telegramAPI(config, "sendMessage", params)
				if err != nil {
					return 0, err
				}
				if !result.OK {
					return 0, fmt.Errorf("telegram error: %s", result.Description)
				}
			} else {
				return 0, fmt.Errorf("telegram error: %s", result.Description)
			}
		}

		// Extract message_id from result
		if len(result.Result) > 0 {
			var msgResult struct {
				MessageID int64 `json:"message_id"`
			}
			if json.Unmarshal(result.Result, &msgResult) == nil {
				lastMsgID = msgResult.MessageID
			}
		}

		// Small delay between messages to maintain order
		if len(messages) > 1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return lastMsgID, nil
}

// fixMarkdownV2Splits ensures no chunk ends with a backslash (which would break escape sequences)
func fixMarkdownV2Splits(messages []string) []string {
	if len(messages) <= 1 {
		return messages
	}

	const maxLen = 4000
	const telegramLimit = 4096 // Actual Telegram API limit
	fixed := make([]string, 0, len(messages))
	for i, msg := range messages {
		// If this is not the last message and ends with backslash, handle it
		if i < len(messages)-1 && strings.HasSuffix(msg, "\\") {
			// Count trailing backslashes
			trailingBackslashes := 0
			for j := len(msg) - 1; j >= 0 && msg[j] == '\\'; j-- {
				trailingBackslashes++
			}
			// Only move if odd number of backslashes (single \, not escaped \\)
			if trailingBackslashes%2 == 1 {
				// Remove trailing backslash from current message
				msg = msg[:len(msg)-1]
				// Prepend backslash to next message FIRST (to preserve order)
				if i+1 < len(messages) {
					messages[i+1] = "\\" + messages[i+1]
					// Check if prepending made next chunk too long
					if len(messages[i+1]) > maxLen {
						// Move excess characters from next chunk to current chunk
						excess := len(messages[i+1]) - maxLen
						// But we must keep the prepended backslash in next chunk!
						// If excess is 1, we'd move just the backslash back, reintroducing the problem
						if excess == 1 && len(messages[i+1]) > 1 {
							// Move the backslash PLUS one more character to preserve it
							// But only if current chunk won't exceed Telegram's limit
							if len(msg)+2 <= telegramLimit {
								excess = 2
							} else {
								// Current chunk would be too long, move even more to balance
								excess = 3
							}
						}
						if excess > 0 && excess <= len(messages[i+1]) && len(msg)+excess <= telegramLimit {
							// Move characters from beginning of next chunk to end of current
							msg += messages[i+1][:excess]
							messages[i+1] = messages[i+1][excess:]
						} else if len(msg)+excess > telegramLimit {
							// Can't move characters without exceeding limit
							// This is a rare edge case where we can't fix the split perfectly
							// In practice, this is unlikely with the 96-char buffer we have
						}
					}
				}
			}
		}
		fixed = append(fixed, msg)
	}
	return fixed
}

// editMessage edits an existing message, sending overflow as new messages
// Converts standard Markdown to Telegram MarkdownV2 format
func editMessage(config *Config, chatID int64, messageID int64, threadID int64, text string) error {
	converted := MarkdownToTelegramV2(text)
	return editMessageWithMode(config, chatID, messageID, threadID, converted, "MarkdownV2", false)
}

// editMessageHTML edits a message using HTML parse mode (for system messages)
func editMessageHTML(config *Config, chatID int64, messageID int64, threadID int64, text string) error {
	return editMessageWithMode(config, chatID, messageID, threadID, text, "HTML", false)
}

func editMessageWithMode(config *Config, chatID int64, messageID int64, threadID int64, text string, parseMode string, escape bool) error {
	const maxLen = 4000

	// Escape text BEFORE splitting to avoid first chunk becoming too long after escaping
	if escape && parseMode == "MarkdownV2" {
		text = EscapeMarkdownV2(text)
	}

	// Split message - first part goes to edit, rest as new messages
	messages := splitMessage(text, maxLen)

	// For MarkdownV2, ensure no chunk ends with a backslash (broken escape sequence)
	if parseMode == "MarkdownV2" {
		messages = fixMarkdownV2Splits(messages)
	}

	// Edit existing message with first part
	params := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"message_id": {fmt.Sprintf("%d", messageID)},
		"text":       {messages[0]},
		"parse_mode": {parseMode},
	}

	result, err := telegramAPI(config, "editMessageText", params)
	if err != nil {
		return err
	}
	if !result.OK {
		// If edit fails (e.g., message not modified), ignore
		return nil
	}

	// Send remaining parts as new messages
	// Note: Text is already escaped, so we send without re-escaping
	for i := 1; i < len(messages); i++ {
		time.Sleep(100 * time.Millisecond)
		// Send already-escaped text without re-escaping
		sendMessageWithMode(config, chatID, threadID, messages[i], parseMode, false)
	}

	return nil
}

func sendMessageWithKeyboard(config *Config, chatID int64, threadID int64, text string, buttons [][]InlineKeyboardButton) error {
	const maxLen = 4000

	// Convert Markdown to MarkdownV2 first (before splitting)
	converted := MarkdownToTelegramV2(text)

	// Split long messages - send all but last as regular messages, last with keyboard
	messages := splitMessage(converted, maxLen)

	// Fix split boundaries to avoid breaking escape sequences
	messages = fixMarkdownV2Splits(messages)

	// Send all but the last message as regular messages
	for i := 0; i < len(messages)-1; i++ {
		sendMessageWithMode(config, chatID, threadID, messages[i], "MarkdownV2", false)
		time.Sleep(100 * time.Millisecond)
	}

	// Send the last message with keyboard - use MarkdownV2 for consistency
	keyboard := map[string]interface{}{
		"inline_keyboard": buttons,
	}
	keyboardJSON, _ := json.Marshal(keyboard)

	params := url.Values{
		"chat_id":      {fmt.Sprintf("%d", chatID)},
		"text":         {messages[len(messages)-1]},
		"parse_mode":   {"MarkdownV2"},
		"reply_markup": {string(keyboardJSON)},
	}
	if threadID > 0 {
		params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
	}

	result, err := telegramAPI(config, "sendMessage", params)
	if err != nil {
		return err
	}
	if !result.OK {
		// If MarkdownV2 parsing fails, retry as plain text
		if strings.Contains(result.Description, "parse entities") {
			params.Del("parse_mode")
			// Unescape MarkdownV2 sequences for plain text fallback
			plainText := unescapeMarkdownV2(params.Get("text"))
			params.Set("text", plainText)
			result, err = telegramAPI(config, "sendMessage", params)
			if err != nil {
				return err
			}
			if !result.OK {
				return fmt.Errorf("telegram error: %s", result.Description)
			}
		} else {
			return fmt.Errorf("telegram error: %s", result.Description)
		}
	}
	return nil
}

func answerCallbackQuery(config *Config, callbackID string) {
	params := url.Values{
		"callback_query_id": {callbackID},
	}
	telegramAPI(config, "answerCallbackQuery", params)
}

func editMessageRemoveKeyboard(config *Config, chatID int64, messageID int, newText string) {
	const maxLen = 4000
	if len(newText) > maxLen {
		newText = newText[:maxLen-3] + "..."
	}

	// Send as plain text to avoid parse errors with arbitrary content
	params := url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"message_id": {fmt.Sprintf("%d", messageID)},
		"text":       {newText},
	}
	telegramAPI(config, "editMessageText", params)
}

func sendTypingAction(config *Config, chatID int64, threadID int64) {
	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", chatID)},
		"action":  {"typing"},
	}
	if threadID > 0 {
		params.Set("message_thread_id", fmt.Sprintf("%d", threadID))
	}
	telegramAPI(config, "sendChatAction", params)
}

func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var messages []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			messages = append(messages, remaining)
			break
		}

		// Find a good split point (newline or space)
		splitAt := maxLen

		// Try to split at a newline first
		if idx := strings.LastIndex(remaining[:maxLen], "\n"); idx > maxLen/2 {
			splitAt = idx + 1
		} else if idx := strings.LastIndex(remaining[:maxLen], " "); idx > maxLen/2 {
			// Fall back to space
			splitAt = idx + 1
		}

		messages = append(messages, strings.TrimRight(remaining[:splitAt], " \n"))
		remaining = remaining[splitAt:]
	}

	return messages
}

// sendFile sends a file to Telegram (max 50MB)
func sendFile(config *Config, chatID int64, threadID int64, filePath string, caption string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add chat_id
	writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if threadID > 0 {
		writer.WriteField("message_thread_id", fmt.Sprintf("%d", threadID))
	}
	if caption != "" {
		writer.WriteField("caption", caption)
	}

	// Add file
	part, err := writer.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		return err
	}
	io.Copy(part, file)
	writer.Close()

	resp, err := http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", config.BotToken),
		writer.FormDataContentType(),
		body,
	)
	if err != nil {
		return redactTokenError(err, config.BotToken)
	}
	defer resp.Body.Close()

	var result TelegramResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}

// downloadTelegramFile downloads a file from Telegram
func downloadTelegramFile(config *Config, fileID string, destPath string) error {
	// Get file path from Telegram
	resp, err := telegramGet(config.BotToken, fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", config.BotToken, fileID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("failed to get file path")
	}

	// Download the file
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", config.BotToken, result.Result.FilePath)
	fileResp, err := telegramGet(config.BotToken, fileURL)
	if err != nil {
		return err
	}
	defer fileResp.Body.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, fileResp.Body)
	return err
}

func createForumTopic(config *Config, name string, providerName string) (int64, error) {
	if config.GroupID == 0 {
		return 0, fmt.Errorf("no group configured. Add bot to a group with topics enabled and run: ccc setgroup")
	}

	// Add first letter of provider as prefix (Telegram uses first char as icon)
	topicName := name
	if providerName != "" && len(providerName) > 0 {
		prefix := strings.ToUpper(string(providerName[0]))
		topicName = fmt.Sprintf("%s %s", prefix, name)
	}

	params := url.Values{
		"chat_id": {fmt.Sprintf("%d", config.GroupID)},
		"name":    {topicName},
	}

	result, err := telegramAPI(config, "createForumTopic", params)
	if err != nil {
		return 0, err
	}
	if !result.OK {
		return 0, fmt.Errorf("failed to create topic: %s", result.Description)
	}

	var topic TopicResult
	if err := json.Unmarshal(result.Result, &topic); err != nil {
		return 0, fmt.Errorf("failed to parse topic result: %w", err)
	}

	return topic.MessageThreadID, nil
}

func deleteForumTopic(config *Config, topicID int64) error {
	if config.GroupID == 0 {
		return fmt.Errorf("no group configured")
	}

	params := url.Values{
		"chat_id":           {fmt.Sprintf("%d", config.GroupID)},
		"message_thread_id": {fmt.Sprintf("%d", topicID)},
	}

	result, err := telegramAPI(config, "deleteForumTopic", params)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("failed to delete topic: %s", result.Description)
	}

	return nil
}

// setBotCommands sets the bot commands in Telegram
func setBotCommands(botToken string) {
	commands := []map[string]string{
		{"command": "new", "description": "Create/restart session: /new <name>"},
		{"command": "continue", "description": "Restart session with history"},
		{"command": "delete", "description": "Delete current session and thread"},
		{"command": "resume", "description": "List/switch Claude sessions: /resume [id]"},
		{"command": "worktree", "description": "Create worktree session: /worktree <base> <name>"},
		{"command": "providers", "description": "List available AI providers"},
		{"command": "cleanup", "description": "Delete ALL sessions, folders and threads"},
		{"command": "c", "description": "Execute shell command: /c <cmd>"},
		{"command": "update", "description": "Update ccc binary from GitHub"},
		{"command": "version", "description": "Show ccc version"},
		{"command": "stats", "description": "Show system stats (RAM, disk, etc)"},
		{"command": "auth", "description": "Re-authenticate Claude OAuth"},
	}

	// Set for default scope
	defaultBody, _ := json.Marshal(map[string]interface{}{
		"commands": commands,
	})
	resp, err := http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", botToken),
		"application/json",
		bytes.NewReader(defaultBody),
	)
	if err == nil {
		resp.Body.Close()
	}

	// Set for all group chats (makes the / button appear)
	groupBody, _ := json.Marshal(map[string]interface{}{
		"commands": commands,
		"scope":    map[string]string{"type": "all_group_chats"},
	})
	resp, err = http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", botToken),
		"application/json",
		bytes.NewReader(groupBody),
	)
	if err == nil {
		resp.Body.Close()
	}
}
