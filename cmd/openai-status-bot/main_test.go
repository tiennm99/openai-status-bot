package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

func TestFrameworkInitialOffsetSeedsLastSeenUpdate(t *testing.T) {
	tests := []struct {
		name   string
		stored int64
		want   int64
		ok     bool
	}{
		{name: "empty", stored: 0, ok: false},
		{name: "invalid negative", stored: -1, ok: false},
		{name: "next update", stored: 42, want: 41, ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := frameworkInitialOffset(tt.stored)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("frameworkInitialOffset(%d) = (%d, %v), want (%d, %v)", tt.stored, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestClearTelegramWebhookReturnsNilOnSuccessfulDelete(t *testing.T) {
	manager := &fakeWebhookManager{}

	if err := clearTelegramWebhook(context.Background(), manager); err != nil {
		t.Fatalf("clearTelegramWebhook returned error: %v", err)
	}
	if manager.deleteCalls != 1 || manager.infoCalls != 0 {
		t.Fatalf("calls = delete %d info %d, want delete 1 info 0", manager.deleteCalls, manager.infoCalls)
	}
}

func TestClearTelegramWebhookAcceptsEmptyDeleteResponseWhenWebhookGone(t *testing.T) {
	manager := &fakeWebhookManager{
		deleteErr:   emptyDeleteWebhookResponseError(t),
		webhookInfo: &tgmodels.WebhookInfo{},
	}

	if err := clearTelegramWebhook(context.Background(), manager); err != nil {
		t.Fatalf("clearTelegramWebhook returned error: %v", err)
	}
	if manager.deleteCalls != 1 || manager.infoCalls != 1 {
		t.Fatalf("calls = delete %d info %d, want delete 1 info 1", manager.deleteCalls, manager.infoCalls)
	}
}

func TestClearTelegramWebhookFailsWhenWebhookStillConfiguredAfterEmptyDeleteResponse(t *testing.T) {
	manager := &fakeWebhookManager{
		deleteErr:   emptyDeleteWebhookResponseError(t),
		webhookInfo: &tgmodels.WebhookInfo{URL: "https://example.com/webhook"},
	}

	err := clearTelegramWebhook(context.Background(), manager)
	if err == nil || !strings.Contains(err.Error(), "webhook is still configured") {
		t.Fatalf("error = %v, want webhook still configured", err)
	}
}

func TestClearTelegramWebhookFailsWhenWebhookInfoCheckFails(t *testing.T) {
	infoErr := errors.New("getWebhookInfo unavailable")
	manager := &fakeWebhookManager{
		deleteErr:      emptyDeleteWebhookResponseError(t),
		webhookInfoErr: infoErr,
	}

	err := clearTelegramWebhook(context.Background(), manager)
	if !errors.Is(err, infoErr) {
		t.Fatalf("error = %v, want wrapping info error", err)
	}
}

func TestClearTelegramWebhookReturnsNonEmptyDeleteError(t *testing.T) {
	deleteErr := errors.New("telegram unauthorized")
	manager := &fakeWebhookManager{deleteErr: deleteErr}

	err := clearTelegramWebhook(context.Background(), manager)
	if !errors.Is(err, deleteErr) {
		t.Fatalf("error = %v, want delete error", err)
	}
	if manager.infoCalls != 0 {
		t.Fatalf("infoCalls = %d, want 0", manager.infoCalls)
	}
}

type fakeWebhookManager struct {
	deleteErr      error
	webhookInfo    *tgmodels.WebhookInfo
	webhookInfoErr error
	deleteCalls    int
	infoCalls      int
}

func (f *fakeWebhookManager) DeleteWebhook(context.Context, *tgbot.DeleteWebhookParams) (bool, error) {
	f.deleteCalls++
	return f.deleteErr == nil, f.deleteErr
}

func (f *fakeWebhookManager) GetWebhookInfo(context.Context) (*tgmodels.WebhookInfo, error) {
	f.infoCalls++
	return f.webhookInfo, f.webhookInfoErr
}

func emptyDeleteWebhookResponseError(t *testing.T) error {
	t.Helper()

	var response struct{}
	err := json.Unmarshal([]byte{}, &response)
	if err == nil {
		t.Fatal("expected json decode error")
	}
	return fmt.Errorf("error decode response body for method deleteWebhook, , %w", err)
}
