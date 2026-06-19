package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TelegramBotToken    string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	OpenAIStatusBaseURL string
	PollInterval        time.Duration
	HTTPTimeout         time.Duration
	LogLevel            slog.Level
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		TelegramBotToken:    strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       os.Getenv("REDIS_PASSWORD"),
		OpenAIStatusBaseURL: strings.TrimRight(getEnv("OPENAI_STATUS_BASE_URL", "https://status.openai.com"), "/"),
	}
	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	var err error
	cfg.RedisDB, err = parseIntEnv("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}
	cfg.PollInterval, err = parseDurationEnv("POLL_INTERVAL", time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.HTTPTimeout, err = parseDurationEnv("HTTP_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg.LogLevel = parseLogLevel(getEnv("LOG_LEVEL", "info"))

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseIntEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return parsed, nil
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
