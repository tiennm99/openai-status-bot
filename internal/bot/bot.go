package bot

import (
	"context"
	"log/slog"
	"strings"

	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	"github.com/tiennm99/openai-status-bot/internal/mongostore"
	openai "github.com/tiennm99/openai-status-bot/internal/openai"
)

type ReplySender interface {
	SendText(ctx context.Context, chatID int64, threadID *int, text string) error
}

type StatusClient interface {
	FetchSummary(ctx context.Context) (openai.Summary, error)
	FetchIncidents(ctx context.Context) (openai.IncidentsResponse, error)
}

type Store interface {
	AddSubscriber(ctx context.Context, sub mongostore.Subscriber) error
	GetSubscriber(ctx context.Context, sub mongostore.Subscriber) (mongostore.Subscriber, bool, error)
	RemoveSubscriber(ctx context.Context, sub mongostore.Subscriber) error
	SaveTelegramOffset(ctx context.Context, offset int64) error
	UpdateSubscriberSettings(ctx context.Context, sub mongostore.Subscriber, types, components []string) (bool, error)
	UpdateSubscriberTypes(ctx context.Context, sub mongostore.Subscriber, types []string) (bool, error)
}

type App struct {
	sender       ReplySender
	statusClient StatusClient
	store        Store
	logger       *slog.Logger
	username     string
}

type MessageContext struct {
	ChatID   int64
	ThreadID *int
	Text     string
}

func New(sender ReplySender, statusClient StatusClient, store Store, logger *slog.Logger, username string) *App {
	return &App{
		sender:       sender,
		statusClient: statusClient,
		store:        store,
		logger:       logger,
		username:     username,
	}
}

func (b *App) RegisterHandlers(telegramBot *tgbot.Bot) {
	telegramBot.RegisterHandlerMatchFunc(hasMessage, b.HandleUpdate)
}

func hasMessage(update *tgmodels.Update) bool {
	return update != nil && update.Message != nil
}

func (b *App) HandleUpdate(ctx context.Context, _ *tgbot.Bot, update *tgmodels.Update) {
	message, ok := messageContextFromUpdate(update)
	if !ok {
		return
	}

	b.handleMessage(ctx, message)

	if err := b.store.SaveTelegramOffset(ctx, update.ID+1); err != nil {
		b.logger.Warn("save telegram offset", "offset", update.ID+1, "error", err)
	}
}

func messageContextFromUpdate(update *tgmodels.Update) (MessageContext, bool) {
	if update == nil || update.Message == nil {
		return MessageContext{}, false
	}

	message := update.Message
	var threadID *int
	if message.MessageThreadID != 0 {
		id := message.MessageThreadID
		threadID = &id
	}

	return MessageContext{
		ChatID:   message.Chat.ID,
		ThreadID: threadID,
		Text:     message.Text,
	}, true
}

func (b *App) handleMessage(ctx context.Context, message MessageContext) {
	if !strings.HasPrefix(strings.TrimSpace(message.Text), "/") {
		return
	}

	command, fields := normalizeCommand(message.Text, b.username)
	switch command {
	case "":
		return
	case "/start":
		b.subscribe(ctx, message)
	case "/stop":
		b.unsubscribe(ctx, message)
	case "/status":
		b.replyStatus(ctx, message, fields)
	case "/components":
		b.replyComponents(ctx, message)
	case "/subscribe":
		b.replySubscribe(ctx, message, fields)
	case "/history":
		b.replyHistory(ctx, message, parseHistoryCount(fields))
	case "/uptime":
		b.replyUptime(ctx, message)
	case "/info":
		b.replyInfo(ctx, message)
	case "/help":
		b.reply(ctx, message, helpText)
	default:
		b.reply(ctx, message, "Unknown command. Use /help.")
	}
}

func (b *App) subscribe(ctx context.Context, message MessageContext) {
	sub := mongostore.NewSubscriber(message.ChatID, message.ThreadID)
	if err := b.store.AddSubscriber(ctx, sub); err != nil {
		b.logger.Error("subscribe", "error", err)
		b.reply(ctx, message, "Could not subscribe right now.")
		return
	}
	b.reply(ctx, message, "Subscribed to OpenAI status updates. Use /subscribe to change preferences.")
}

func (b *App) unsubscribe(ctx context.Context, message MessageContext) {
	sub := mongostore.NewSubscriber(message.ChatID, message.ThreadID)
	if err := b.store.RemoveSubscriber(ctx, sub); err != nil {
		b.logger.Error("unsubscribe", "error", err)
		b.reply(ctx, message, "Could not unsubscribe right now.")
		return
	}
	b.reply(ctx, message, "Unsubscribed from OpenAI status updates. Use /start to resubscribe.")
}
