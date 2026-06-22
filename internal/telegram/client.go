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
	baseURL        string
	httpClient     *http.Client
	requestTimeout time.Duration
}

type APIError struct {
	StatusCode  int
	ErrorCode   int
	Description string
}

func (e *APIError) Error() string {
	if e.ErrorCode != 0 {
		return fmt.Sprintf("telegram API error_code %d (HTTP %d): %s", e.ErrorCode, e.StatusCode, e.Description)
	}
	return fmt.Sprintf("telegram API HTTP %d: %s", e.StatusCode, e.Description)
}

func IsTerminalSendError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	code := apiErr.ErrorCode
	if code == 0 {
		code = apiErr.StatusCode
	}
	if code == http.StatusForbidden {
		return true
	}
	if code != http.StatusBadRequest {
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
		baseURL:        "https://api.telegram.org/bot" + token,
		httpClient:     &http.Client{},
		requestTimeout: timeout,
	}
}

func (c *Client) DeleteWebhook(ctx context.Context) error {
	payload := map[string]any{"drop_pending_updates": false}
	var result json.RawMessage
	return c.postJSON(ctx, "/deleteWebhook", payload, &result, c.requestTimeout)
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	var user User
	if err := c.postJSON(ctx, "/getMe", map[string]any{}, &user, c.requestTimeout); err != nil {
		return User{}, err
	}
	return user, nil
}

func (c *Client) SetMyCommands(ctx context.Context, commands []BotCommand) error {
	payload := map[string]any{"commands": commands}
	var result json.RawMessage
	return c.postJSON(ctx, "/setMyCommands", payload, &result, c.requestTimeout)
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	payload := map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message"},
	}

	var updates []Update
	if err := c.postJSON(ctx, "/getUpdates", payload, &updates, c.longPollRequestTimeout(timeoutSeconds)); err != nil {
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
	return c.postJSON(ctx, "/sendMessage", payload, &result, c.requestTimeout)
}

func (c *Client) longPollRequestTimeout(timeoutSeconds int) time.Duration {
	telegramWait := time.Duration(timeoutSeconds) * time.Second
	if telegramWait < 0 {
		telegramWait = 0
	}
	if c.requestTimeout <= 0 {
		return telegramWait
	}
	return telegramWait + c.requestTimeout
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, target any, requestTimeout time.Duration) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	requestCtx := ctx
	if requestTimeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, requestTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
		ErrorCode   int             `json:"error_code"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return &APIError{StatusCode: res.StatusCode, Description: res.Status}
		}
		return fmt.Errorf("decode telegram response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		description := envelope.Description
		if description == "" {
			description = res.Status
		}
		return &APIError{StatusCode: res.StatusCode, ErrorCode: envelope.ErrorCode, Description: description}
	}
	if !envelope.OK {
		return &APIError{StatusCode: res.StatusCode, ErrorCode: envelope.ErrorCode, Description: envelope.Description}
	}
	if target != nil {
		if err := json.Unmarshal(envelope.Result, target); err != nil {
			return fmt.Errorf("decode telegram result: %w", err)
		}
	}
	return nil
}
