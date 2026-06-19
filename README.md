# openai-status-bot

Telegram bot that watches [OpenAI Status](https://status.openai.com/) every minute and sends updates to subscribed chats. State and subscriptions are stored in Redis.

## Features

- Checks OpenAI status on a configurable interval, default `1m`
- Notifies subscribers about new incident updates
- Notifies subscribers when component status changes
- Uses Redis for subscribers, component status checkpoints, and seen incident updates
- Supports Telegram supergroup topics via `message_thread_id`
- Includes Docker Compose for local Redis + bot runtime

## Bot Commands

| Command | Description |
|---------|-------------|
| `/start` | Subscribe current chat or topic |
| `/stop` | Unsubscribe current chat or topic |
| `/status` | Show current OpenAI status |
| `/components` | Show all OpenAI component statuses |
| `/history [count]` | Show recent incidents, default 5, max 10 |
| `/help` | Show command help |

## Quick Start

```bash
cp .env.example .env
# edit .env and set TELEGRAM_BOT_TOKEN
docker compose up --build
```

For local development without Docker:

```bash
go mod tidy
go run ./cmd/openai-status-bot
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | required | Telegram bot token from BotFather |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | empty | Redis password |
| `REDIS_DB` | `0` | Redis database number |
| `OPENAI_STATUS_BASE_URL` | `https://status.openai.com` | OpenAI status page base URL |
| `POLL_INTERVAL` | `1m` | Status check interval |
| `HTTP_TIMEOUT` | `10s` | HTTP request timeout |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

## Notes

The first successful poll seeds Redis and does not send historical incidents. Notifications start from later changes.
