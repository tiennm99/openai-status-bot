package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnvDefaultsAndTrimming(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("TELEGRAM_BOT_TOKEN", " token ")
	t.Setenv("REDIS_PASSWORD", " redis-pass ")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.TelegramBotToken != "token" {
		t.Fatalf("TelegramBotToken = %q", cfg.TelegramBotToken)
	}
	if cfg.RedisPassword != "redis-pass" {
		t.Fatalf("RedisPassword = %q", cfg.RedisPassword)
	}
	if cfg.RedisDB != 0 || cfg.PollInterval != time.Minute || cfg.HTTPTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadFromEnvAcceptsWarningLogAlias(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("LOG_LEVEL", "warning")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Fatalf("LogLevel = %v, want warn", cfg.LogLevel)
	}
}

func TestLoadFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
	}{
		{name: "redis db below range", key: "REDIS_DB", value: "-1", wantErr: "REDIS_DB must be between 0 and 15"},
		{name: "redis db above range", key: "REDIS_DB", value: "16", wantErr: "REDIS_DB must be between 0 and 15"},
		{name: "poll interval too small", key: "POLL_INTERVAL", value: "1ns", wantErr: "POLL_INTERVAL must be between 5s and 1h0m0s"},
		{name: "http timeout too small", key: "HTTP_TIMEOUT", value: "500ms", wantErr: "HTTP_TIMEOUT must be between 1s and 5m0s"},
		{name: "base url missing scheme", key: "OPENAI_STATUS_BASE_URL", value: "status.openai.com", wantErr: "OPENAI_STATUS_BASE_URL must use http or https"},
		{name: "invalid log level", key: "LOG_LEVEL", value: "verbose", wantErr: "LOG_LEVEL must be one of"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMinimalEnv(t)
			t.Setenv(tt.key, tt.value)

			_, err := LoadFromEnv()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func setMinimalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "")
	t.Setenv("OPENAI_STATUS_BASE_URL", "")
	t.Setenv("POLL_INTERVAL", "")
	t.Setenv("HTTP_TIMEOUT", "")
	t.Setenv("LOG_LEVEL", "")
}
