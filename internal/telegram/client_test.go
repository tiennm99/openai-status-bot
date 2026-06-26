package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

type fakeFrameworkSender struct {
	params *tgbot.SendMessageParams
	err    error
}

func (f *fakeFrameworkSender) SendMessage(_ context.Context, params *tgbot.SendMessageParams) (*tgmodels.Message, error) {
	f.params = params
	return &tgmodels.Message{}, f.err
}

func TestSendTextUsesHTMLAndDisablesPreview(t *testing.T) {
	framework := &fakeFrameworkSender{}
	sender := NewSender(framework, "token", 0)

	if err := sender.SendText(context.Background(), 123, nil, "<b>hello</b>"); err != nil {
		t.Fatalf("SendText returned error: %v", err)
	}
	if framework.params == nil {
		t.Fatal("SendMessage params were not captured")
	}
	if framework.params.ChatID != int64(123) {
		t.Fatalf("ChatID = %v, want 123", framework.params.ChatID)
	}
	if framework.params.ParseMode != tgmodels.ParseModeHTML {
		t.Fatalf("ParseMode = %v, want HTML", framework.params.ParseMode)
	}
	if framework.params.LinkPreviewOptions == nil || framework.params.LinkPreviewOptions.IsDisabled == nil || !*framework.params.LinkPreviewOptions.IsDisabled {
		t.Fatalf("LinkPreviewOptions = %+v, want disabled preview", framework.params.LinkPreviewOptions)
	}
}

func TestSendTextPassesThreadID(t *testing.T) {
	framework := &fakeFrameworkSender{}
	sender := NewSender(framework, "token", 0)
	threadID := 42

	if err := sender.SendText(context.Background(), 123, &threadID, "hello"); err != nil {
		t.Fatalf("SendText returned error: %v", err)
	}
	if framework.params.MessageThreadID != threadID {
		t.Fatalf("MessageThreadID = %d, want %d", framework.params.MessageThreadID, threadID)
	}
}

func TestIsTerminalSendError(t *testing.T) {
	terminal := &APIError{StatusCode: http.StatusOK, ErrorCode: http.StatusForbidden, Description: "Forbidden: bot was blocked by the user"}
	if !IsTerminalSendError(terminal) {
		t.Fatal("403 blocked should be terminal")
	}
	chatNotFound := &APIError{StatusCode: http.StatusOK, ErrorCode: http.StatusBadRequest, Description: "Bad Request: chat not found"}
	if !IsTerminalSendError(chatNotFound) {
		t.Fatal("chat not found should be terminal")
	}
	parseError := &APIError{StatusCode: http.StatusOK, ErrorCode: http.StatusBadRequest, Description: "Bad Request: can't parse entities"}
	if IsTerminalSendError(parseError) {
		t.Fatal("parse errors should not remove subscribers")
	}
	legacyForbidden := &APIError{StatusCode: http.StatusForbidden, Description: "Forbidden"}
	if !IsTerminalSendError(legacyForbidden) {
		t.Fatal("status-code-only 403 should remain terminal")
	}
}

func TestSendTextMapsFrameworkForbiddenToTerminalAPIError(t *testing.T) {
	framework := &fakeFrameworkSender{err: fmt.Errorf("%w, Forbidden: bot was kicked", tgbot.ErrorForbidden)}
	sender := NewSender(framework, "token", 0)

	err := sender.SendText(context.Background(), 123, nil, "hello")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("SendText error = %v, want APIError", err)
	}
	if apiErr.ErrorCode != http.StatusForbidden {
		t.Fatalf("ErrorCode = %d, want %d", apiErr.ErrorCode, http.StatusForbidden)
	}
	if !IsTerminalSendError(err) {
		t.Fatal("framework forbidden error should be terminal")
	}
}

func TestSendTextMapsFrameworkBadRequestWithoutRemovingSubscriber(t *testing.T) {
	framework := &fakeFrameworkSender{err: fmt.Errorf("%w, Bad Request: can't parse entities", tgbot.ErrorBadRequest)}
	sender := NewSender(framework, "token", 0)

	err := sender.SendText(context.Background(), 123, nil, "hello")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("SendText error = %v, want APIError", err)
	}
	if apiErr.ErrorCode != http.StatusBadRequest {
		t.Fatalf("ErrorCode = %d, want %d", apiErr.ErrorCode, http.StatusBadRequest)
	}
	if IsTerminalSendError(err) {
		t.Fatal("parse errors should not remove subscribers")
	}
}

func TestTransportErrorRedactsBotToken(t *testing.T) {
	const token = "123456:super-secret-token"
	framework := &fakeFrameworkSender{err: errors.New("Post https://api.telegram.org/bot" + token + "/sendMessage: dial tcp")}
	sender := NewSender(framework, token, 0)

	err := sender.SendText(context.Background(), 123, nil, "hello")
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked bot token: %v", err)
	}
	if !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("error missing redaction placeholder: %v", err)
	}
}
