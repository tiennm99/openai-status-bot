package telegram

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	MessageID       int64  `json:"message_id"`
	MessageThreadID *int   `json:"message_thread_id,omitempty"`
	Text            string `json:"text"`
	Chat            Chat   `json:"chat"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}
