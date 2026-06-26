package bot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/mongostore"
	"github.com/tiennm99/openai-status-bot/internal/telegram"
)

type fakeTelegramClient struct {
	replies       []string
	getUpdatesErr error
	onGetUpdates  func()
}

func (f *fakeTelegramClient) GetUpdates(context.Context, int64, int) ([]telegram.Update, error) {
	if f.onGetUpdates != nil {
		f.onGetUpdates()
	}
	return nil, f.getUpdatesErr
}

func (f *fakeTelegramClient) SendText(_ context.Context, _ int64, _ *int, text string) error {
	f.replies = append(f.replies, text)
	return nil
}

type fakeBotStatusClient struct {
	summary openai.Summary
}

func (f fakeBotStatusClient) FetchSummary(context.Context) (openai.Summary, error) {
	return f.summary, nil
}

func (f fakeBotStatusClient) FetchIncidents(context.Context) (openai.IncidentsResponse, error) {
	return openai.IncidentsResponse{}, nil
}

type fakeBotStore struct {
	sub        mongostore.Subscriber
	subscribed bool
	types      []string
	components []string
	offsetErr  error
}

func (f *fakeBotStore) AddSubscriber(_ context.Context, sub mongostore.Subscriber) error {
	f.sub = sub
	f.subscribed = true
	f.types = mongostore.DefaultSubscriptionTypes()
	return nil
}

func (f *fakeBotStore) GetSubscriber(context.Context, mongostore.Subscriber) (mongostore.Subscriber, bool, error) {
	if !f.subscribed {
		return mongostore.Subscriber{}, false, nil
	}
	sub := f.sub
	sub.Types = append([]string{}, f.types...)
	sub.Components = append([]string{}, f.components...)
	return sub, true, nil
}

func (f *fakeBotStore) RemoveSubscriber(context.Context, mongostore.Subscriber) error {
	f.subscribed = false
	return nil
}

func (f *fakeBotStore) SaveTelegramOffset(context.Context, int64) error { return nil }
func (f *fakeBotStore) TelegramOffset(context.Context) (int64, error)   { return 0, f.offsetErr }

func (f *fakeBotStore) UpdateSubscriberTypes(_ context.Context, _ mongostore.Subscriber, types []string) (bool, error) {
	if !f.subscribed {
		return false, nil
	}
	f.types = append([]string{}, types...)
	return true, nil
}

func (f *fakeBotStore) UpdateSubscriberSettings(_ context.Context, _ mongostore.Subscriber, types, components []string) (bool, error) {
	if !f.subscribed {
		return false, nil
	}
	f.types = append([]string{}, types...)
	f.components = append([]string{}, components...)
	return true, nil
}

func TestSubscribeComponentStoresComponentID(t *testing.T) {
	bot, store, tg := newTestBot([]openai.Component{{ID: "c-api", Name: "API", Status: "operational"}})
	store.subscribed = true
	store.sub = mongostore.NewSubscriber(123, nil)
	store.types = []string{mongostore.SubscriptionTypeIncident}

	bot.handleMessage(context.Background(), messageText("/subscribe component api"))

	if len(store.components) != 1 || store.components[0] != "c-api" {
		t.Fatalf("components = %v, want [c-api]", store.components)
	}
	if !containsComponent(store.types, mongostore.SubscriptionTypeComponent) {
		t.Fatalf("types = %v, want component enabled", store.types)
	}
	if len(tg.replies) != 1 || !strings.Contains(tg.replies[0], "Subscribed to component") {
		t.Fatalf("reply = %v", tg.replies)
	}
}

func TestSubscribeComponentAllEnablesComponentType(t *testing.T) {
	bot, store, _ := newTestBot(nil)
	store.subscribed = true
	store.sub = mongostore.NewSubscriber(123, nil)
	store.types = []string{mongostore.SubscriptionTypeIncident}
	store.components = []string{"c-api"}

	bot.handleMessage(context.Background(), messageText("/subscribe component all"))

	if len(store.components) != 0 {
		t.Fatalf("components = %v, want cleared", store.components)
	}
	if !containsComponent(store.types, mongostore.SubscriptionTypeComponent) {
		t.Fatalf("types = %v, want component enabled", store.types)
	}
}

func TestStatusAmbiguousComponentNameRequiresID(t *testing.T) {
	bot, _, tg := newTestBot([]openai.Component{
		{ID: "login-a", Name: "Login", Status: "operational"},
		{ID: "login-b", Name: "Login", Status: "operational"},
	})

	bot.handleMessage(context.Background(), messageText("/status Login"))

	if len(tg.replies) != 1 || !strings.Contains(tg.replies[0], "ambiguous") || !strings.Contains(tg.replies[0], "login-a") {
		t.Fatalf("reply = %v", tg.replies)
	}
}

func TestCommandForOtherBotIsIgnored(t *testing.T) {
	bot, _, tg := newTestBot(nil)

	bot.handleMessage(context.Background(), messageText("/start@OtherBot"))

	if len(tg.replies) != 0 {
		t.Fatalf("replies = %v, want none", tg.replies)
	}
}

func TestRunReturnsPromptlyWhenContextCanceledAfterGetUpdatesError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tg := &fakeTelegramClient{getUpdatesErr: errors.New("network"), onGetUpdates: cancel}
	store := &fakeBotStore{}
	bot := New(tg, fakeBotStatusClient{}, store, slog.Default(), "OpenAIStatusBot")

	started := time.Now()
	if err := bot.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Run took %s after context cancellation", elapsed)
	}
}

func TestRunReturnsTelegramOffsetError(t *testing.T) {
	store := &fakeBotStore{offsetErr: errors.New("invalid telegram offset")}
	bot := New(&fakeTelegramClient{}, fakeBotStatusClient{}, store, slog.Default(), "OpenAIStatusBot")

	err := bot.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "load telegram offset") {
		t.Fatalf("Run error = %v, want telegram offset error", err)
	}
}

func newTestBot(components []openai.Component) (*Bot, *fakeBotStore, *fakeTelegramClient) {
	tg := &fakeTelegramClient{}
	store := &fakeBotStore{}
	statusClient := fakeBotStatusClient{summary: openai.Summary{Components: components}}
	return New(tg, statusClient, store, slog.Default(), "OpenAIStatusBot"), store, tg
}

func messageText(text string) telegram.Message {
	return telegram.Message{Text: text, Chat: telegram.Chat{ID: 123}}
}
