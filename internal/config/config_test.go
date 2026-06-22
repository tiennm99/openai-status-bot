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

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.TelegramBotToken != "token" {
		t.Fatalf("TelegramBotToken = %q", cfg.TelegramBotToken)
	}
	if cfg.RedisOptions == nil || cfg.RedisOptions.Addr != "localhost:6379" || cfg.RedisOptions.DB != 0 {
		t.Fatalf("unexpected Redis options: %+v", cfg.RedisOptions)
	}
	if cfg.PollInterval != time.Minute || cfg.HTTPTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadFromEnvIgnoresLegacyRedisVariables(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("REDIS_ADDR", "legacy-host:6380")
	t.Setenv("REDIS_PASSWORD", "legacy-password")
	t.Setenv("REDIS_DB", "5")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.RedisOptions == nil || cfg.RedisOptions.Addr != "localhost:6379" || cfg.RedisOptions.Password != "" || cfg.RedisOptions.DB != 0 {
		t.Fatalf("legacy Redis variables affected options: %+v", cfg.RedisOptions)
	}
}

func TestLoadFromEnvAcceptsRedisURLCredentials(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("REDIS_URL", " redis://user:redis%40pass%3Awith%2Freserved@localhost:6380/2 ")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.RedisOptions == nil || cfg.RedisOptions.Addr != "localhost:6380" || cfg.RedisOptions.Username != "user" || cfg.RedisOptions.Password != "redis@pass:with/reserved" || cfg.RedisOptions.DB != 2 {
		t.Fatalf("unexpected Redis options: %+v", cfg.RedisOptions)
	}
}

func TestLoadFromEnvDoesNotLeakRedisURLCredentialsInParseErrors(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("REDIS_URL", "redis://:secret%zz@localhost:6379/0")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "secret%zz") {
		t.Fatalf("error leaked Redis URL credentials: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "REDIS_URL must be a valid Redis URL") {
		t.Fatalf("error = %q, want generic Redis URL error", err.Error())
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
		{name: "redis url missing scheme", key: "REDIS_URL", value: "localhost:6379", wantErr: "REDIS_URL must be a valid Redis URL"},
		{name: "redis url invalid db", key: "REDIS_URL", value: "redis://localhost:6379/not-a-number", wantErr: "REDIS_URL must be a valid Redis URL"},
		{name: "redis url db below range", key: "REDIS_URL", value: "redis://localhost:6379/-1", wantErr: "REDIS_URL database must be between 0 and 15"},
		{name: "redis url db above range", key: "REDIS_URL", value: "redis://localhost:6379/16", wantErr: "REDIS_URL database must be between 0 and 15"},
		{name: "poll interval too small", key: "POLL_INTERVAL", value: "1ns", wantErr: "POLL_INTERVAL must be between 5s and 1h0m0s"},
		{name: "http timeout too small", key: "HTTP_TIMEOUT", value: "500ms", wantErr: "HTTP_TIMEOUT must be between 1s and 5m0s"},
		{name: "base url missing scheme", key: "OPENAI_STATUS_BASE_URL", value: "status.openai.com", wantErr: "OPENAI_STATUS_BASE_URL must use http or https"},
		{name: "base url with bare query marker", key: "OPENAI_STATUS_BASE_URL", value: "https://status.openai.com?", wantErr: "OPENAI_STATUS_BASE_URL must not include query or fragment"},
		{name: "base url with query", key: "OPENAI_STATUS_BASE_URL", value: "https://status.openai.com?foo=bar", wantErr: "OPENAI_STATUS_BASE_URL must not include query or fragment"},
		{name: "base url with fragment", key: "OPENAI_STATUS_BASE_URL", value: "https://status.openai.com#api", wantErr: "OPENAI_STATUS_BASE_URL must not include query or fragment"},
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
	t.Setenv("REDIS_URL", "")
	t.Setenv("OPENAI_STATUS_BASE_URL", "")
	t.Setenv("POLL_INTERVAL", "")
	t.Setenv("HTTP_TIMEOUT", "")
	t.Setenv("LOG_LEVEL", "")
}
