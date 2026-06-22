# openai-status-bot

Telegram bot that watches [OpenAI Status](https://status.openai.com/) every minute and sends updates to subscribed chats. State and subscriptions are stored in Redis.

## Features

- Checks OpenAI status on a configurable interval, default `1m`
- Notifies subscribers about new incident updates
- Notifies subscribers when component status changes
- Supports incident-only, component-only, and component-filtered subscriptions
- Uses Redis for subscribers, delivery state, component checkpoints, and seen incident update versions
- Supports Telegram supergroup topics via `message_thread_id`
- Clears an existing Telegram webhook before long polling, for migration from webhook deployments
- Includes Docker Compose for local Redis + bot runtime

## Bot Commands

| Command | Description |
|---------|-------------|
| `/start` | Subscribe current chat or topic |
| `/stop` | Unsubscribe current chat or topic |
| `/status [component]` | Show current OpenAI status, optionally for one component |
| `/components` | Show all OpenAI component statuses |
| `/subscribe <incident|component|all>` | Set notification types |
| `/subscribe component <name|id|all>` | Filter component notifications or clear the filter |
| `/history [count]` | Show recent incidents, default 5, max 10 |
| `/uptime` | Show component health overview |
| `/info` | Show chat ID and subscription settings |
| `/help` | Show command help |

## Quick Start

```bash
cp .env.example .env
# edit .env and set TELEGRAM_BOT_TOKEN
docker compose up --build
```

To deploy only the bot when Redis is hosted elsewhere:

```bash
cp .env.example .env
# edit .env and set TELEGRAM_BOT_TOKEN plus REDIS_URL for the external Redis host
docker compose -f docker-compose.bot.yml up -d --build
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
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection URL, e.g. `redis://:password@localhost:6379/0` |
| `OPENAI_STATUS_BASE_URL` | `https://status.openai.com` | OpenAI status page base URL |
| `POLL_INTERVAL` | `1m` | Status check interval, valid `5s`-`1h` |
| `HTTP_TIMEOUT` | `10s` | HTTP request timeout, valid `1s`-`5m` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`/`warning`, or `error` |

Percent-encode Redis usernames or passwords that contain URL-reserved characters such as `@`, `:`, `/`, `#`, or `%`.

## Notes

The first successful poll seeds Redis and does not send historical incidents. Notifications start from later changes.

Incident update dedupe tracks the update content/version, so edited Statuspage updates can notify again. Delivery is checkpointed after successful fan-out; retryable Telegram failures may be retried on a later poll without advancing the global checkpoint.
