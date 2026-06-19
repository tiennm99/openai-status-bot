package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tiennm99/openai-status-bot/internal/redisstore"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: "https://api.telegram.org/bot" + token,
		httpClient: &http.Client{
			Timeout: timeout + 60*time.Second,
		},
	}
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
		"chat_id": chatID,
		"text":    text,
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
		return fmt.Errorf("telegram API %s: %s", res.Status, envelope.Description)
	}
	if target != nil {
		if err := json.Unmarshal(envelope.Result, target); err != nil {
			return fmt.Errorf("decode telegram result: %w", err)
		}
	}
	return nil
}
