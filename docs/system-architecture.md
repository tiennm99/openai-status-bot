# System Architecture

## Overview

`openai-status-bot` is one Go process with two loops:

- Telegram long polling loop for user commands.
- OpenAI status polling loop, default every minute.

Redis stores subscribers and polling checkpoints.

## Data Flow

1. User sends `/start` in Telegram.
2. Bot stores chat ID and optional topic thread ID in Redis.
3. Poller fetches OpenAI status JSON:
   - `GET /api/v2/summary.json`
   - `GET /api/v2/incidents.json`
4. Poller compares fetched state with Redis checkpoints.
5. New incident updates or component status changes are sent to all subscribers.

## Redis Keys

| Key | Type | Purpose |
|-----|------|---------|
| `openai-status:subscribers` | set | Telegram chat or topic subscribers |
| `openai-status:component-statuses` | hash | Last seen component status by component ID |
| `openai-status:incident-updates` | set | Seen incident update IDs |
| `openai-status:initialized` | string | Baseline seed marker |

Subscriber set members are `chatID` or `chatID:threadID`.

## Runtime

The service uses Telegram `getUpdates`, so it does not need a public webhook URL. Docker Compose starts Redis and the bot.

## Failure Behavior

- First successful poll seeds state without notification.
- OpenAI fetch failures are logged and retried on the next interval.
- Telegram send failures are logged; other subscribers still receive messages.
- Redis connection failure at startup exits the process.
