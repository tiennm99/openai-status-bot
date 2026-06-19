package config

import (
	"fmt"
	"log/slog"
	"net/url"
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
		TelegramBotToken:    strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
		OpenAIStatusBaseURL: strings.TrimRight(getEnv("OPENAI_STATUS_BASE_URL", "https://status.openai.com"), "/"),
	}
	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if err := validateBaseURL("OPENAI_STATUS_BASE_URL", cfg.OpenAIStatusBaseURL); err != nil {
		return Config{}, err
	}

	var err error
	cfg.RedisDB, err = parseBoundedIntEnv("REDIS_DB", 0, minRedisDB, maxRedisDB)
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

func parseBoundedIntEnv(key string, fallback, minValue, maxValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	if parsed < minValue || parsed > maxValue {
		return 0, fmt.Errorf("%s must be between %d and %d", key, minValue, maxValue)
	}
	return parsed, nil
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

func validateBaseURL(key, value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", key, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", key)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include a host", key)
	}
	return nil
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
