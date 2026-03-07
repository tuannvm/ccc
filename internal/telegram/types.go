package telegram

// TelegramMessage represents a Telegram message
type TelegramMessage struct {
	MessageID       int                `json:"message_id"`
	MessageThreadID int64              `json:"message_thread_id,omitempty"` // Topic ID
	Chat            struct {
		ID   int64  `json:"id"`
		Type string `json:"type"` // "private", "group", "supergroup"
	} `json:"chat"`
	From struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
	Text           string              `json:"text"`
	ReplyToMessage *TelegramMessage    `json:"reply_to_message,omitempty"`
	Voice          *TelegramVoice      `json:"voice,omitempty"`
	Photo          []TelegramPhoto     `json:"photo,omitempty"`
	Document       *TelegramDocument   `json:"document,omitempty"`
	Caption        string              `json:"caption,omitempty"`
}

// TelegramVoice represents a voice message
type TelegramVoice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
}

// TelegramPhoto represents a photo
type TelegramPhoto struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int    `json:"file_size"`
}

// TelegramDocument represents a document
type TelegramDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	FileSize int    `json:"file_size"`
}

// CallbackQuery represents a Telegram callback query (button press)
type CallbackQuery struct {
	ID   string `json:"id"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Message *TelegramMessage `json:"message"`
	Data    string           `json:"data"`
}

// TelegramUpdate represents an update from Telegram
type TelegramUpdate struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      []struct {
		UpdateID      int             `json:"update_id"`
		Message       TelegramMessage `json:"message"`
		CallbackQuery *CallbackQuery  `json:"callback_query"`
	} `json:"result"`
}

// TelegramResponse represents a response from Telegram API
type TelegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Result      []byte `json:"result,omitempty"`
}

// TopicResult represents the result of creating a forum topic
type TopicResult struct {
	MessageThreadID int64  `json:"message_thread_id"`
	Name            string `json:"name"`
}

// InlineKeyboardButton represents a Telegram inline keyboard button
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}
