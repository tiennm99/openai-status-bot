# Setup Guide

## Prerequisites

- Go 1.24+
- Redis 7+
- Telegram bot token from BotFather

## Local Docker Run

```bash
cp .env.example .env
# set TELEGRAM_BOT_TOKEN in .env
# REDIS_URL is overridden to redis://redis:6379/0 by docker compose
docker compose up --build
```

The bundled Redis service is for local development and binds to `127.0.0.1:6379` on the host.

## Bot-Only Docker Run

Use this when Redis is hosted outside Docker Compose:

```bash
cp .env.example .env
# set TELEGRAM_BOT_TOKEN in .env
# set REDIS_URL to the external Redis URL, not localhost
docker compose -f docker-compose.bot.yml up -d --build
```

## Local Go Run

Start Redis:

```bash
docker run --rm -p 127.0.0.1:6379:6379 redis:7-alpine
```

Run bot:

```bash
cp .env.example .env
# export TELEGRAM_BOT_TOKEN or source .env with your shell workflow
# REDIS_URL defaults to redis://localhost:6379/0
go run ./cmd/openai-status-bot
```

## Verification

```bash
go test ./...
go build -o /tmp/openai-status-bot ./cmd/openai-status-bot
```

Then send `/start` to the Telegram bot and run `/status`. If the same token was previously used by a webhook deployment, startup clears that webhook before long polling begins.

Docker images include a health check against `http://127.0.0.1:8080/healthz` inside the container.
