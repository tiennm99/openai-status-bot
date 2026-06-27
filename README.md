# openai-status-bot

Telegram bot that watches [OpenAI Status](https://status.openai.com/) every minute and sends updates to subscribed chats. State and subscriptions are stored in MongoDB.

## Features

- Checks OpenAI status on a configurable interval, default `1m`
- Notifies subscribers about new incident updates
- Notifies subscribers when component status changes
- Supports incident-only, component-only, and component-filtered subscriptions
- Uses MongoDB for subscribers, delivery state, component checkpoints, and seen incident update versions
- Supports Telegram supergroup topics via `message_thread_id`
- Clears an existing Telegram webhook before long polling, for migration from webhook deployments
- Registers the Telegram command menu on startup
- Exposes a local health endpoint for container health checks
- Includes Docker Compose for the bot runtime (datastore is managed MongoDB Atlas)

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

There is no local datastore. Both Compose files run only the bot and connect to
a managed MongoDB Atlas cluster, so a reachable `MONGODB_URI` is required to
start. The development and production setups share one cluster and differ only
by database name (`MONGODB_DATABASE`).

Production runtime (default Compose file; uses the `MONGODB_DATABASE` from `.env`, default `openai_status_bot`):

```bash
cp .env.example .env
# edit .env and set TELEGRAM_BOT_TOKEN, MONGODB_URI, and MONGODB_DATABASE
docker compose up -d --build
```

Development runtime (targets the `development` database):

```bash
cp .env.example .env
# edit .env and set TELEGRAM_BOT_TOKEN and MONGODB_URI (Atlas connection string)
docker compose -f compose.dev.yaml up --build
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
| `MONGODB_URI` | required | MongoDB connection string, e.g. an Atlas `mongodb+srv://user:pass@cluster/` URI |
| `MONGODB_DATABASE` | `openai_status_bot` | Database name; use a separate name (e.g. `development`) to split dev and prod on one cluster |
| `POLL_INTERVAL` | `1m` | Status check interval, valid `5s`-`1h` |
| `HTTP_TIMEOUT` | `10s` | HTTP request timeout, valid `1s`-`5m` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`/`warning`, or `error` |

Percent-encode MongoDB usernames or passwords that contain URL-reserved characters such as `@`, `:`, `/`, `#`, or `%`.

The bot always reads OpenAI status from `https://status.openai.com`.

## Notes

The first successful poll seeds the database and does not send historical incidents. Notifications start from later changes.

Switching from a prior Redis deployment starts from empty state: there is no data migration, so subscribers must re-issue `/start`, component checkpoints reseed on the first poll, and the stored Telegram update offset is not migrated, so retained Telegram updates can be reprocessed once after cutover.

Incident update dedupe tracks the update content/version, so edited Statuspage updates can notify again. Each event is checkpointed independently once it has fully fanned out, so a retryable Telegram failure on one event only defers that event for retry on a later poll and never blocks checkpoints for other events delivered in the same poll.

A 7-day TTL index on the `delivery` collection expires per-event delivery markers automatically; the bot creates required indexes on startup.

Integration tests for the MongoDB store run a real `mongod` via testcontainers and are excluded from the default `go test ./...`. Run them with Docker available: `go test -tags=integration ./internal/mongostore/...`.
