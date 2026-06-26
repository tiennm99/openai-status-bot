package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeleteWebhookKeepsPendingUpdates(t *testing.T) {
	var gotPath string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client()}
	if err := client.DeleteWebhook(context.Background()); err != nil {
		t.Fatalf("DeleteWebhook returned error: %v", err)
	}
	if gotPath != "/deleteWebhook" {
		t.Fatalf("path = %s", gotPath)
	}
	if payload["drop_pending_updates"] != false {
		t.Fatalf("drop_pending_updates = %v", payload["drop_pending_updates"])
	}
}

func TestSendTextUsesHTMLAndDisablesPreview(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sendMessage" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client()}
	if err := client.SendText(context.Background(), 123, nil, "<b>hello</b>"); err != nil {
		t.Fatalf("SendText returned error: %v", err)
	}
	if payload["parse_mode"] != "HTML" {
		t.Fatalf("parse_mode = %v", payload["parse_mode"])
	}
	if payload["disable_web_page_preview"] != true {
		t.Fatalf("disable_web_page_preview = %v", payload["disable_web_page_preview"])
	}
}

func TestSetMyCommandsRegistersCommands(t *testing.T) {
	var gotPath string
	var payload struct {
		Commands []BotCommand `json:"commands"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client()}
	commands := []BotCommand{
		{Command: "start", Description: "Subscribe"},
		{Command: "status", Description: "Show status"},
	}
	if err := client.SetMyCommands(context.Background(), commands); err != nil {
		t.Fatalf("SetMyCommands returned error: %v", err)
	}
	if gotPath != "/setMyCommands" {
		t.Fatalf("path = %s", gotPath)
	}
	if len(payload.Commands) != len(commands) {
		t.Fatalf("commands = %v, want %v", payload.Commands, commands)
	}
	for i := range commands {
		if payload.Commands[i] != commands[i] {
			t.Fatalf("commands[%d] = %+v, want %+v", i, payload.Commands[i], commands[i])
		}
	}
}

func TestIsTerminalSendError(t *testing.T) {
	terminal := &APIError{StatusCode: 200, ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"}
	if !IsTerminalSendError(terminal) {
		t.Fatal("403 blocked should be terminal")
	}
	chatNotFound := &APIError{StatusCode: 200, ErrorCode: 400, Description: "Bad Request: chat not found"}
	if !IsTerminalSendError(chatNotFound) {
		t.Fatal("chat not found should be terminal")
	}
	parseError := &APIError{StatusCode: 200, ErrorCode: 400, Description: "Bad Request: can't parse entities"}
	if IsTerminalSendError(parseError) {
		t.Fatal("parse errors should not remove subscribers")
	}
	legacyForbidden := &APIError{StatusCode: 403, Description: "Forbidden"}
	if !IsTerminalSendError(legacyForbidden) {
		t.Fatal("status-code-only 403 should remain terminal")
	}
}

func TestSendTextCapturesTelegramErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error_code":403,"description":"Forbidden: bot was kicked"}`))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client()}
	err := client.SendText(context.Background(), 123, nil, "hello")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("SendText error = %v, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusOK || apiErr.ErrorCode != http.StatusForbidden {
		t.Fatalf("APIError = %+v", apiErr)
	}
	if !IsTerminalSendError(err) {
		t.Fatal("telegram error_code 403 should be terminal")
	}
}

func TestSendTextPreservesNonJSONHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client()}
	err := client.SendText(context.Background(), 123, nil, "hello")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("SendText error = %v, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadGateway)
	}
}

func TestSendTextRejectsNonSuccessHTTPStatusEvenWhenEnvelopeOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client()}
	err := client.SendText(context.Background(), 123, nil, "hello")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("SendText error = %v, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
}

func TestTransportErrorRedactsBotToken(t *testing.T) {
	const token = "123456:super-secret-token"
	// Port 0 is reserved and never listens, forcing a transport-level dial
	// error whose *url.Error message embeds the token-bearing URL.
	client := &Client{
		baseURL:        "http://127.0.0.1:0/bot" + token,
		token:          token,
		httpClient:     &http.Client{},
		requestTimeout: time.Second,
	}
	err := client.SendText(context.Background(), 123, nil, "hello")
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

func TestNewClientUsesConfiguredTimeoutForOrdinaryRequests(t *testing.T) {
	client := NewClient("token", 10*time.Second)
	if client.requestTimeout != 10*time.Second {
		t.Fatalf("requestTimeout = %s, want 10s", client.requestTimeout)
	}
	if client.httpClient.Timeout != 0 {
		t.Fatalf("httpClient.Timeout = %s, want context-scoped timeout", client.httpClient.Timeout)
	}
	if got := client.longPollRequestTimeout(50); got != time.Minute {
		t.Fatalf("longPollRequestTimeout = %s, want 1m", got)
	}
}
