package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	"github.com/tiennm99/openai-status-bot/internal/mongostore"
)

type messageSender interface {
	SendMessage(ctx context.Context, params *tgbot.SendMessageParams) (*tgmodels.Message, error)
}

type Sender struct {
	bot     messageSender
	token   string
	timeout time.Duration
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

func NewSender(bot messageSender, token string, timeout time.Duration) *Sender {
	return &Sender{bot: bot, token: token, timeout: timeout}
}

func (s *Sender) SendMessage(ctx context.Context, sub mongostore.Subscriber, text string) error {
	return s.SendText(ctx, sub.ChatID, sub.ThreadID, text)
}

func (s *Sender) SendText(ctx context.Context, chatID int64, threadID *int, text string) error {
	if s == nil || s.bot == nil {
		return errors.New("telegram sender is not configured")
	}

	disablePreview := true
	params := &tgbot.SendMessageParams{
		ChatID:             chatID,
		Text:               text,
		ParseMode:          tgmodels.ParseModeHTML,
		LinkPreviewOptions: &tgmodels.LinkPreviewOptions{IsDisabled: &disablePreview},
	}
	if threadID != nil {
		params.MessageThreadID = *threadID
	}

	requestCtx := ctx
	if s.timeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	_, err := s.bot.SendMessage(requestCtx, params)
	return s.mapSendError(err)
}

func (s *Sender) mapSendError(err error) error {
	if err == nil {
		return nil
	}

	description := s.redactErrorText(err)
	switch {
	case errors.Is(err, tgbot.ErrorForbidden):
		return &APIError{StatusCode: http.StatusOK, ErrorCode: http.StatusForbidden, Description: description}
	case errors.Is(err, tgbot.ErrorBadRequest):
		return &APIError{StatusCode: http.StatusOK, ErrorCode: http.StatusBadRequest, Description: description}
	default:
		if description != err.Error() {
			return errors.New(description)
		}
		return err
	}
}

func (s *Sender) redactErrorText(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if s == nil || s.token == "" || !strings.Contains(msg, s.token) {
		return msg
	}
	return strings.ReplaceAll(msg, s.token, "<redacted>")
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
