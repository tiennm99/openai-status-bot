package bot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/redisstore"
	"github.com/tiennm99/openai-status-bot/internal/telegram"
)

type TelegramClient interface {
	GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]telegram.Update, error)
	SendText(ctx context.Context, chatID int64, threadID *int, text string) error
}

type StatusClient interface {
	FetchSummary(ctx context.Context) (openai.Summary, error)
	FetchIncidents(ctx context.Context) (openai.IncidentsResponse, error)
}

type Store interface {
	AddSubscriber(ctx context.Context, sub redisstore.Subscriber) error
	RemoveSubscriber(ctx context.Context, sub redisstore.Subscriber) error
}

type Bot struct {
	telegramClient TelegramClient
	statusClient   StatusClient
	store          Store
	logger         *slog.Logger
}

func New(telegramClient TelegramClient, statusClient StatusClient, store Store, logger *slog.Logger) *Bot {
	return &Bot{
		telegramClient: telegramClient,
		statusClient:   statusClient,
		store:          store,
		logger:         logger,
	}
}

func (b *Bot) Run(ctx context.Context) error {
	var offset int64

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := b.telegramClient.GetUpdates(ctx, offset, 50)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			b.logger.Warn("get telegram updates", "error", err)
			time.Sleep(3 * time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message == nil {
				continue
			}
			b.handleMessage(ctx, *update.Message)
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, message telegram.Message) {
	if !strings.HasPrefix(strings.TrimSpace(message.Text), "/") {
		return
	}

	command, fields := normalizeCommand(message.Text)
	switch command {
	case "/start":
		b.subscribe(ctx, message)
	case "/stop":
		b.unsubscribe(ctx, message)
	case "/status":
		b.replyStatus(ctx, message)
	case "/components":
		b.replyComponents(ctx, message)
	case "/history":
		b.replyHistory(ctx, message, parseHistoryCount(fields))
	case "/help":
		b.reply(ctx, message, helpText)
	default:
		b.reply(ctx, message, "Unknown command. Use /help.")
	}
}

func (b *Bot) subscribe(ctx context.Context, message telegram.Message) {
	sub := redisstore.NewSubscriber(message.Chat.ID, message.MessageThreadID)
	if err := b.store.AddSubscriber(ctx, sub); err != nil {
		b.logger.Error("subscribe", "error", err)
		b.reply(ctx, message, "Could not subscribe right now.")
		return
	}
	b.reply(ctx, message, "Subscribed to OpenAI status updates.")
}

func (b *Bot) unsubscribe(ctx context.Context, message telegram.Message) {
	sub := redisstore.NewSubscriber(message.Chat.ID, message.MessageThreadID)
	if err := b.store.RemoveSubscriber(ctx, sub); err != nil {
		b.logger.Error("unsubscribe", "error", err)
		b.reply(ctx, message, "Could not unsubscribe right now.")
		return
	}
	b.reply(ctx, message, "Unsubscribed from OpenAI status updates.")
}

func (b *Bot) replyStatus(ctx context.Context, message telegram.Message) {
	summary, err := b.statusClient.FetchSummary(ctx)
	if err != nil {
		b.logger.Error("fetch status", "error", err)
		b.reply(ctx, message, "Could not fetch OpenAI status right now.")
		return
	}
	b.reply(ctx, message, formatStatus(summary))
}

func (b *Bot) replyComponents(ctx context.Context, message telegram.Message) {
	summary, err := b.statusClient.FetchSummary(ctx)
	if err != nil {
		b.logger.Error("fetch components", "error", err)
		b.reply(ctx, message, "Could not fetch OpenAI components right now.")
		return
	}
	b.reply(ctx, message, formatComponents(summary))
}

func (b *Bot) replyHistory(ctx context.Context, message telegram.Message, count int) {
	incidents, err := b.statusClient.FetchIncidents(ctx)
	if err != nil {
		b.logger.Error("fetch incidents", "error", err)
		b.reply(ctx, message, "Could not fetch OpenAI incident history right now.")
		return
	}
	b.reply(ctx, message, formatHistory(incidents.Incidents, count))
}

func (b *Bot) reply(ctx context.Context, message telegram.Message, text string) {
	if err := b.telegramClient.SendText(ctx, message.Chat.ID, message.MessageThreadID, text); err != nil {
		b.logger.Warn("send telegram reply", "chat_id", message.Chat.ID, "error", err)
	}
}
