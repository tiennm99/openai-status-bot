# Setup Guide

## Prerequisites

- Go 1.24+
- MongoDB Atlas cluster (managed, no local MongoDB service required)
- Telegram bot token from BotFather

## Local Docker Run

```bash
cp .env.example .env
# set TELEGRAM_BOT_TOKEN in .env
# set MONGODB_URI to your MongoDB Atlas connection string
# docker compose sets MONGODB_DATABASE=openai_status_bot_dev for local development
docker compose up --build
```

The Docker Compose service connects to MongoDB Atlas; there is no bundled local MongoDB. Ensure `MONGODB_URI` is set in `.env`.

## Bot-Only Docker Run

Use this for production deployment:

```bash
cp .env.example .env
# set TELEGRAM_BOT_TOKEN in .env
# set MONGODB_URI to your MongoDB Atlas connection string
# MONGODB_DATABASE defaults to openai_status_bot (production)
docker compose -f docker-compose.bot.yml up -d --build
```

## Local Go Run

```bash
cp .env.example .env
# set TELEGRAM_BOT_TOKEN in .env
# set MONGODB_URI to your MongoDB Atlas connection string
# export TELEGRAM_BOT_TOKEN or source .env with your shell workflow
# MONGODB_DATABASE defaults to openai_status_bot
go run ./cmd/openai-status-bot
```

MongoDB Atlas connection is required; there is no local MongoDB fallback.

## Verification

```bash
go test ./...
go build -o /tmp/openai-status-bot ./cmd/openai-status-bot
```

Then send `/start` to the Telegram bot and run `/status`. If the same token was previously used by a webhook deployment, startup clears that webhook before long polling begins.

Docker images include a health check against `http://127.0.0.1:8080/healthz` inside the container.
