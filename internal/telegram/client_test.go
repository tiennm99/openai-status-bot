package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestIsTerminalSendError(t *testing.T) {
	terminal := &APIError{StatusCode: 403, Description: "Forbidden: bot was blocked by the user"}
	if !IsTerminalSendError(terminal) {
		t.Fatal("403 blocked should be terminal")
	}
	chatNotFound := &APIError{StatusCode: 400, Description: "Bad Request: chat not found"}
	if !IsTerminalSendError(chatNotFound) {
		t.Fatal("chat not found should be terminal")
	}
	parseError := &APIError{StatusCode: 400, Description: "Bad Request: can't parse entities"}
	if IsTerminalSendError(parseError) {
		t.Fatal("parse errors should not remove subscribers")
	}
}
