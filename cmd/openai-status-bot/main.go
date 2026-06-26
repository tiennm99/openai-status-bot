package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

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
	telegramClient := telegram.NewClient(cfg.TelegramBotToken, cfg.HTTPTimeout)
	if err := telegramClient.DeleteWebhook(ctx); err != nil {
		logger.Error("delete telegram webhook", "error", err)
		os.Exit(1)
	}
	if err := telegramClient.SetMyCommands(ctx, bot.MenuCommands()); err != nil {
		logger.Warn("set telegram bot commands", "error", err)
	}

	botUsername := ""
	if me, err := telegramClient.GetMe(ctx); err != nil {
		logger.Warn("get telegram bot profile", "error", err)
	} else {
		botUsername = me.Username
	}

	statusPoller := poller.NewRunner(statusClient, store, telegramClient, cfg.PollInterval, logger)
	commandBot := bot.New(telegramClient, statusClient, store, logger, botUsername)

	go statusPoller.Run(ctx)

	ready.Store(true)
	logger.Info("openai status bot started", "poll_interval", cfg.PollInterval.String())
	if err := commandBot.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("telegram bot stopped", "error", err)
		os.Exit(1)
	}
}
