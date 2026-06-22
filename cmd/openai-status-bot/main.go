package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/tiennm99/openai-status-bot/internal/bot"
	"github.com/tiennm99/openai-status-bot/internal/config"
	"github.com/tiennm99/openai-status-bot/internal/health"
	openai "github.com/tiennm99/openai-status-bot/internal/openai"
	"github.com/tiennm99/openai-status-bot/internal/poller"
	"github.com/tiennm99/openai-status-bot/internal/redisstore"
	"github.com/tiennm99/openai-status-bot/internal/telegram"
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

	go health.Run(ctx, logger)

	redisClient := redis.NewClient(cfg.RedisOptions)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Error("connect redis", "network", cfg.RedisOptions.Network, "addr", cfg.RedisOptions.Addr, "db", cfg.RedisOptions.DB, "tls", cfg.RedisOptions.TLSConfig != nil, "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	store := redisstore.New(redisClient)
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

	logger.Info("openai status bot started", "poll_interval", cfg.PollInterval.String())
	if err := commandBot.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("telegram bot stopped", "error", err)
		os.Exit(1)
	}
}
