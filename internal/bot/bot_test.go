package bot

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	tgmodels "github.com/go-telegram/bot/models"
	"github.com/tiennm99/openai-status-bot/internal/mongostore"
	openai "github.com/tiennm99/openai-status-bot/internal/openai"
)

type fakeTelegramSender struct {
	replies []sentReply
}

type sentReply struct {
	chatID   int64
	threadID *int
	text     string
}

func (f *fakeTelegramSender) SendText(_ context.Context, chatID int64, threadID *int, text string) error {
	f.replies = append(f.replies, sentReply{chatID: chatID, threadID: threadID, text: text})
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
	sub          mongostore.Subscriber
	subscribed   bool
	types        []string
	components   []string
	savedOffsets []int64
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

func (f *fakeBotStore) SaveTelegramOffset(_ context.Context, offset int64) error {
	f.savedOffsets = append(f.savedOffsets, offset)
	return nil
}

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
	if len(tg.replies) != 1 || !strings.Contains(tg.replies[0].text, "Subscribed to component") {
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

	if len(tg.replies) != 1 || !strings.Contains(tg.replies[0].text, "ambiguous") || !strings.Contains(tg.replies[0].text, "login-a") {
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

func TestHandleUpdateRepliesToThreadAndSavesNextOffset(t *testing.T) {
	bot, store, tg := newTestBot(nil)
	threadID := 42

	bot.HandleUpdate(context.Background(), nil, &tgmodels.Update{
		ID: 77,
		Message: &tgmodels.Message{
			Chat:            tgmodels.Chat{ID: 123},
			MessageThreadID: threadID,
			Text:            "/start",
		},
	})

	if len(store.savedOffsets) != 1 || store.savedOffsets[0] != 78 {
		t.Fatalf("savedOffsets = %v, want [78]", store.savedOffsets)
	}
	if len(tg.replies) != 1 {
		t.Fatalf("replies = %v, want one reply", tg.replies)
	}
	if tg.replies[0].chatID != 123 {
		t.Fatalf("chatID = %d, want 123", tg.replies[0].chatID)
	}
	if tg.replies[0].threadID == nil || *tg.replies[0].threadID != threadID {
		t.Fatalf("threadID = %v, want %d", tg.replies[0].threadID, threadID)
	}
}

func TestHandleUpdateSavesOffsetForNonCommandMessage(t *testing.T) {
	bot, store, tg := newTestBot(nil)

	bot.HandleUpdate(context.Background(), nil, &tgmodels.Update{
		ID:      80,
		Message: &tgmodels.Message{Chat: tgmodels.Chat{ID: 123}, Text: "hello"},
	})

	if len(tg.replies) != 0 {
		t.Fatalf("replies = %v, want none", tg.replies)
	}
	if len(store.savedOffsets) != 1 || store.savedOffsets[0] != 81 {
		t.Fatalf("savedOffsets = %v, want [81]", store.savedOffsets)
	}
}

func newTestBot(components []openai.Component) (*App, *fakeBotStore, *fakeTelegramSender) {
	tg := &fakeTelegramSender{}
	store := &fakeBotStore{}
	statusClient := fakeBotStatusClient{summary: openai.Summary{Components: components}}
	return New(tg, statusClient, store, slog.Default(), "OpenAIStatusBot"), store, tg
}

func messageText(text string) MessageContext {
	return MessageContext{Text: text, ChatID: 123}
}
