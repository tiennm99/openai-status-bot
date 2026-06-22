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

## Local Go Run

Start Redis:

```bash
docker run --rm -p 6379:6379 redis:7-alpine
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
