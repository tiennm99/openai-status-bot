package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tiennm99/openai-status-bot/internal/redisstore"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type APIError struct {
	StatusCode  int
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram API %d: %s", e.StatusCode, e.Description)
}

func IsTerminalSendError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode == http.StatusForbidden {
		return true
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		return false
	}
	description := strings.ToLower(apiErr.Description)
	terminalPhrases := []string{
		"chat not found",
		"message thread not found",
		"message thread is closed",
		"bot was kicked",
		"bot is not a member",
	}
	for _, phrase := range terminalPhrases {
		if strings.Contains(description, phrase) {
			return true
		}
	}
	return false
}

func NewClient(token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: "https://api.telegram.org/bot" + token,
		httpClient: &http.Client{
			Timeout: timeout + 60*time.Second,
		},
	}
}

func (c *Client) DeleteWebhook(ctx context.Context) error {
	payload := map[string]any{"drop_pending_updates": false}
	var result json.RawMessage
	return c.postJSON(ctx, "/deleteWebhook", payload, &result)
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	var user User
	if err := c.postJSON(ctx, "/getMe", map[string]any{}, &user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	payload := map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message"},
	}

	var updates []Update
	if err := c.postJSON(ctx, "/getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) SendMessage(ctx context.Context, sub redisstore.Subscriber, text string) error {
	return c.SendText(ctx, sub.ChatID, sub.ThreadID, text)
}

func (c *Client) SendText(ctx context.Context, chatID int64, threadID *int, text string) error {
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if threadID != nil {
		payload["message_thread_id"] = *threadID
	}

	var result json.RawMessage
	return c.postJSON(ctx, "/sendMessage", payload, &result)
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}
	if !envelope.OK {
		return &APIError{StatusCode: res.StatusCode, Description: envelope.Description}
	}
	if target != nil {
		if err := json.Unmarshal(envelope.Result, target); err != nil {
			return fmt.Errorf("decode telegram result: %w", err)
		}
	}
	return nil
}
