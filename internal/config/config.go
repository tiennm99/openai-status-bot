package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	TelegramBotToken string
	RedisOptions     *redis.Options
	PollInterval     time.Duration
	HTTPTimeout      time.Duration
	LogLevel         slog.Level
}

const (
	minPollInterval = 5 * time.Second
	maxPollInterval = time.Hour
	minHTTPTimeout  = time.Second
	maxHTTPTimeout  = 5 * time.Minute
	minRedisDB      = 0
	maxRedisDB      = 15
)

func LoadFromEnv() (Config, error) {
	cfg := Config{
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
	}
	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	var err error
	cfg.RedisOptions, err = parseRedisURL("REDIS_URL", getEnv("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		return Config{}, err
	}

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

func parseRedisURL(key, value string) (*redis.Options, error) {
	options, err := redis.ParseURL(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid Redis URL", key)
	}
	if options.DB < minRedisDB || options.DB > maxRedisDB {
		return nil, fmt.Errorf("%s database must be between %d and %d", key, minRedisDB, maxRedisDB)
	}
	return options, nil
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
