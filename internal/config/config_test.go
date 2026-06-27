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
	t.Setenv("MONGODB_URI", " mongodb+srv://user:pass@cluster.example.net/ ")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.TelegramBotToken != "token" {
		t.Fatalf("TelegramBotToken = %q", cfg.TelegramBotToken)
	}
	if cfg.MongoURI != "mongodb+srv://user:pass@cluster.example.net/" {
		t.Fatalf("MongoURI = %q", cfg.MongoURI)
	}
	if cfg.MongoDatabase != defaultMongoDatabase {
		t.Fatalf("MongoDatabase = %q, want %q", cfg.MongoDatabase, defaultMongoDatabase)
	}
	if cfg.PollInterval != time.Minute || cfg.HTTPTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadFromEnvAcceptsCustomMongoDatabase(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("MONGODB_DATABASE", " openai-status-bot-development ")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.MongoDatabase != "openai-status-bot-development" {
		t.Fatalf("MongoDatabase = %q, want openai-status-bot-development", cfg.MongoDatabase)
	}
}

func TestLoadFromEnvRequiresMongoURI(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("MONGODB_URI", "")

	_, err := LoadFromEnv()
	if err == nil || !strings.Contains(err.Error(), "MONGODB_URI is required") {
		t.Fatalf("error = %v, want MONGODB_URI required", err)
	}
}

func TestLoadFromEnvIgnoresOpenAIStatusBaseURL(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("OPENAI_STATUS_BASE_URL", "not a url")

	if _, err := LoadFromEnv(); err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
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
		{name: "poll interval too small", key: "POLL_INTERVAL", value: "1ns", wantErr: "POLL_INTERVAL must be between 5s and 1h0m0s"},
		{name: "http timeout too small", key: "HTTP_TIMEOUT", value: "500ms", wantErr: "HTTP_TIMEOUT must be between 1s and 5m0s"},
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
	t.Setenv("MONGODB_URI", "mongodb+srv://user:pass@cluster.example.net/")
	t.Setenv("MONGODB_DATABASE", "")
	t.Setenv("POLL_INTERVAL", "")
	t.Setenv("HTTP_TIMEOUT", "")
	t.Setenv("LOG_LEVEL", "")
}
