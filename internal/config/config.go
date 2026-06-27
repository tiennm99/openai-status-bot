package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	TelegramBotToken string
	MongoURI         string
	MongoDatabase    string
	PollInterval     time.Duration
	HTTPTimeout      time.Duration
	LogLevel         slog.Level
}

const (
	minPollInterval      = 5 * time.Second
	maxPollInterval      = time.Hour
	minHTTPTimeout       = time.Second
	maxHTTPTimeout       = 5 * time.Minute
	defaultMongoDatabase = "openai-status-bot"
)

func LoadFromEnv() (Config, error) {
	cfg := Config{
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
	}
	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	cfg.MongoURI = strings.TrimSpace(os.Getenv("MONGODB_URI"))
	if cfg.MongoURI == "" {
		return Config{}, fmt.Errorf("MONGODB_URI is required")
	}
	cfg.MongoDatabase = getEnv("MONGODB_DATABASE", defaultMongoDatabase)

	var err error
	cfg.PollInterval, err = parseDurationEnv("POLL_INTERVAL", time.Minute, minPollInterval, maxPollInterval)
	if err != nil {
		return Config{}, err
	}
	cfg.HTTPTimeout, err = parseDurationEnv("HTTP_TIMEOUT", 10*time.Second, minHTTPTimeout, maxHTTPTimeout)
	if err != nil {
		return Config{}, err
	}
	cfg.LogLevel, err = parseLogLevel(getEnv("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseDurationEnv(key string, fallback, minValue, maxValue time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	if parsed < minValue || parsed > maxValue {
		return 0, fmt.Errorf("%s must be between %s and %s", key, minValue, maxValue)
	}
	return parsed, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, warning, or error")
	}
}
