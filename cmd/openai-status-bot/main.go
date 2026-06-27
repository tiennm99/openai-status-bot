package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	"github.com/tiennm99/openai-status-bot/internal/bot"
	"github.com/tiennm99/openai-status-bot/internal/config"
	"github.com/tiennm99/openai-status-bot/internal/health"
	"github.com/tiennm99/openai-status-bot/internal/mongostore"
	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/poller"
	"github.com/tiennm99/openai-status-bot/internal/telegram"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type telegramWebhookManager interface {
	DeleteWebhook(context.Context, *tgbot.DeleteWebhookParams) (bool, error)
	GetWebhookInfo(context.Context) (*tgmodels.WebhookInfo, error)
}

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// The health endpoint comes up immediately but reports 503 until startup
	// finishes, then pings MongoDB on each probe so a dependency outage is
	// reflected instead of a static 200.
	var (
		ready       atomic.Bool
		mongoClient *mongo.Client
	)
	go health.Run(ctx, logger, func(ctx context.Context) error {
		if !ready.Load() {
			return errors.New("bot is still starting")
		}
		return mongoClient.Ping(ctx, nil)
	})

	mongoClient, err = mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		logger.Error("connect mongodb", "database", cfg.MongoDatabase, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := mongoClient.Disconnect(context.Background()); err != nil {
			logger.Warn("disconnect mongodb", "error", err)
		}
	}()
	if err := mongoClient.Ping(ctx, nil); err != nil {
		logger.Error("ping mongodb", "database", cfg.MongoDatabase, "error", err)
		os.Exit(1)
	}

	store := mongostore.New(mongoClient, cfg.MongoDatabase)
	if err := store.EnsureIndexes(ctx); err != nil {
		logger.Error("ensure mongodb indexes", "database", cfg.MongoDatabase, "error", err)
		os.Exit(1)
	}
	statusClient := openai.NewClient(cfg.HTTPTimeout)

	telegramOffset, err := store.TelegramOffset(ctx)
	if err != nil {
		logger.Error("load telegram offset", "error", err)
		os.Exit(1)
	}

	telegramPollTimeout := 51 * time.Second
	telegramOptions := []tgbot.Option{
		tgbot.WithSkipGetMe(),
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates{"message"}),
		tgbot.WithWorkers(1),
		tgbot.WithUpdatesChannelCap(1),
		tgbot.WithNotAsyncHandlers(),
		tgbot.WithHTTPClient(telegramPollTimeout, &http.Client{Timeout: telegramPollTimeout + cfg.HTTPTimeout}),
		tgbot.WithErrorsHandler(func(err error) {
			logger.Warn("telegram runtime", "error", redactTelegramRuntimeError(cfg.TelegramBotToken, err))
		}),
	}
	if initialOffset, ok := frameworkInitialOffset(telegramOffset); ok {
		telegramOptions = append(telegramOptions, tgbot.WithInitialOffset(initialOffset))
	}

	telegramBot, err := tgbot.New(cfg.TelegramBotToken, telegramOptions...)
	if err != nil {
		logger.Error("create telegram bot", "error", redactTelegramRuntimeError(cfg.TelegramBotToken, err))
		os.Exit(1)
	}
	requestCtx, cancelRequest := contextWithOptionalTimeout(ctx, cfg.HTTPTimeout)
	err = clearTelegramWebhook(requestCtx, telegramBot)
	cancelRequest()
	if err != nil {
		logger.Error("delete telegram webhook", "error", redactTelegramRuntimeError(cfg.TelegramBotToken, err))
		os.Exit(1)
	}

	requestCtx, cancelRequest = contextWithOptionalTimeout(ctx, cfg.HTTPTimeout)
	_, err = telegramBot.SetMyCommands(requestCtx, &tgbot.SetMyCommandsParams{Commands: bot.MenuCommands()})
	cancelRequest()
	if err != nil {
		logger.Warn("set telegram bot commands", "error", redactTelegramRuntimeError(cfg.TelegramBotToken, err))
	}

	botUsername := ""
	requestCtx, cancelRequest = contextWithOptionalTimeout(ctx, cfg.HTTPTimeout)
	me, err := telegramBot.GetMe(requestCtx)
	cancelRequest()
	if err != nil {
		logger.Warn("get telegram bot profile", "error", redactTelegramRuntimeError(cfg.TelegramBotToken, err))
	} else {
		botUsername = me.Username
	}

	telegramSender := telegram.NewSender(telegramBot, cfg.TelegramBotToken, cfg.HTTPTimeout)
	commandApp := bot.New(telegramSender, statusClient, store, logger, botUsername)
	commandApp.RegisterHandlers(telegramBot)
	statusPoller := poller.NewRunner(statusClient, store, telegramSender, cfg.PollInterval, logger)

	go statusPoller.Run(ctx)

	ready.Store(true)
	logger.Info("openai status bot started", "poll_interval", cfg.PollInterval.String(), "telegram_offset", telegramOffset)
	telegramBot.Start(ctx)
}

func redactTelegramRuntimeError(token string, err error) error {
	if err == nil || token == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, token) {
		return err
	}
	return errors.New(strings.ReplaceAll(msg, token, "<redacted>"))
}

func contextWithOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func frameworkInitialOffset(nextOffset int64) (int64, bool) {
	if nextOffset <= 0 {
		return 0, false
	}
	return nextOffset - 1, true
}

func clearTelegramWebhook(ctx context.Context, telegramBot telegramWebhookManager) error {
	_, err := telegramBot.DeleteWebhook(ctx, &tgbot.DeleteWebhookParams{DropPendingUpdates: false})
	if err == nil {
		return nil
	}
	if !isDeleteWebhookEmptyResponseError(err) {
		return err
	}

	info, infoErr := telegramBot.GetWebhookInfo(ctx)
	if infoErr != nil {
		return fmt.Errorf("deleteWebhook returned an empty response and getWebhookInfo failed: %w", errors.Join(err, infoErr))
	}
	if info == nil {
		return fmt.Errorf("deleteWebhook returned an empty response and getWebhookInfo returned no webhook info: %w", err)
	}
	if info.URL != "" {
		return fmt.Errorf("deleteWebhook returned an empty response and webhook is still configured: %w", err)
	}
	return nil
}

func isDeleteWebhookEmptyResponseError(err error) bool {
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		return false
	}
	return strings.Contains(err.Error(), "error decode response body for method deleteWebhook, ,")
}
