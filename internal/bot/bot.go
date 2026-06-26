package bot

import (
	"context"
	"fmt"
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
	GetSubscriber(ctx context.Context, sub redisstore.Subscriber) (redisstore.Subscriber, bool, error)
	RemoveSubscriber(ctx context.Context, sub redisstore.Subscriber) error
	SaveTelegramOffset(ctx context.Context, offset int64) error
	TelegramOffset(ctx context.Context) (int64, error)
	UpdateSubscriberSettings(ctx context.Context, sub redisstore.Subscriber, types, components []string) (bool, error)
	UpdateSubscriberTypes(ctx context.Context, sub redisstore.Subscriber, types []string) (bool, error)
}

type Bot struct {
	telegramClient TelegramClient
	statusClient   StatusClient
	store          Store
	logger         *slog.Logger
	username       string
}

func New(telegramClient TelegramClient, statusClient StatusClient, store Store, logger *slog.Logger, username string) *Bot {
	return &Bot{
		telegramClient: telegramClient,
		statusClient:   statusClient,
		store:          store,
		logger:         logger,
		username:       username,
	}
}

func (b *Bot) Run(ctx context.Context) error {
	offset, err := b.store.TelegramOffset(ctx)
	if err != nil {
		return fmt.Errorf("load telegram offset: %w", err)
	}

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
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
			}
			continue
		}

		for _, update := range updates {
			if update.UpdateID < offset {
				continue
			}
			if update.Message != nil {
				b.handleMessage(ctx, *update.Message)
			}
			offset = update.UpdateID + 1
			if err := b.store.SaveTelegramOffset(ctx, offset); err != nil {
				b.logger.Warn("save telegram offset", "offset", offset, "error", err)
			}
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, message telegram.Message) {
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

func (b *Bot) subscribe(ctx context.Context, message telegram.Message) {
	sub := redisstore.NewSubscriber(message.Chat.ID, message.MessageThreadID)
	if err := b.store.AddSubscriber(ctx, sub); err != nil {
		b.logger.Error("subscribe", "error", err)
		b.reply(ctx, message, "Could not subscribe right now.")
		return
	}
	b.reply(ctx, message, "Subscribed to OpenAI status updates. Use /subscribe to change preferences.")
}

func (b *Bot) unsubscribe(ctx context.Context, message telegram.Message) {
	sub := redisstore.NewSubscriber(message.Chat.ID, message.MessageThreadID)
	if err := b.store.RemoveSubscriber(ctx, sub); err != nil {
		b.logger.Error("unsubscribe", "error", err)
		b.reply(ctx, message, "Could not unsubscribe right now.")
		return
	}
	b.reply(ctx, message, "Unsubscribed from OpenAI status updates. Use /start to resubscribe.")
}
