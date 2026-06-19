# System Architecture

## Overview

`openai-status-bot` is one Go process with two loops:

- Telegram long polling loop for user commands.
- OpenAI status polling loop, default every minute.

Redis stores subscribers, subscription settings, polling checkpoints, delivery retry state, and the Telegram update offset.

## Data Flow

1. User sends `/start` in Telegram.
2. Bot stores chat ID, optional topic thread ID, and default subscription settings in Redis.
3. Users can adjust settings with `/subscribe` and inspect them with `/info`.
4. Poller fetches OpenAI status JSON:
   - `GET /api/v2/summary.json`
   - `GET /api/v2/incidents.json`
5. Poller compares fetched state with Redis checkpoints and builds notification events without mutating checkpoints.
6. Events are sent to eligible subscribers, respecting incident/component preferences and component ID filters.
7. Component and incident checkpoints are written only after delivery succeeds or terminal subscriber failures are removed.

## Redis Keys

| Key | Type | Purpose |
|-----|------|---------|
| `openai-status:subscribers` | set | Telegram chat or topic subscribers |
| `openai-status:subscriber-settings` | hash | Subscription types and component ID filters by subscriber key |
| `openai-status:component-statuses` | hash | Last seen component status by component ID |
| `openai-status:incident-updates` | set | Legacy seen incident update IDs, retained for migration |
| `openai-status:incident-update-versions` | hash | Seen incident update version by update ID |
| `openai-status:event-delivery:<hash>` | set | Temporary per-event subscriber delivery state for retry isolation |
| `openai-status:telegram-offset` | string | Last processed Telegram update offset |
| `openai-status:initialized` | string | Baseline seed marker |

Subscriber set members are `chatID` or `chatID:threadID`. Missing settings default to incident and component notifications for backward compatibility with older Redis state.

## Runtime

The service uses Telegram `getUpdates`, so it does not need a public webhook URL. On startup it calls `deleteWebhook` before long polling, which allows migration from a webhook-based deployment using the same bot token. Docker Compose starts Redis and the bot.

## Failure Behavior

- First successful poll seeds state without notification.
- OpenAI fetch failures are logged and retried on the next interval.
- Retryable Telegram failures do not advance component or incident checkpoints.
- Successful per-subscriber deliveries are tracked temporarily, so retrying one failed subscriber does not resend to already-delivered subscribers.
- Telegram 403 and selected terminal 400 errors remove the unreachable subscriber, then delivery continues.
- Malformed subscriber keys are skipped instead of blocking all fan-out.
- Redis connection failure at startup exits the process.
