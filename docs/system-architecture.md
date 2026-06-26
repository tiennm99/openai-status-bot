# System Architecture

## Overview

`openai-status-bot` is one Go process with two loops:

- Telegram long polling loop for user commands.
- OpenAI status polling loop, default every minute.

MongoDB stores subscribers, subscription settings, polling checkpoints, delivery retry state, and the Telegram update offset.

## Data Flow

1. User sends `/start` in Telegram.
2. Bot stores chat ID, optional topic thread ID, and default subscription settings in MongoDB.
3. Users can adjust settings with `/subscribe` and inspect them with `/info`.
4. Poller fetches OpenAI status JSON:
   - `GET https://status.openai.com/api/v2/summary.json`
   - `GET https://status.openai.com/api/v2/incidents.json`
5. Poller compares fetched state with MongoDB checkpoints and builds notification events without mutating checkpoints.
6. Events are sent to eligible subscribers, respecting incident/component preferences and component ID filters.
7. Component and incident checkpoints are written only after delivery succeeds or terminal subscriber failures are removed.

## MongoDB Collections

| Collection | Document ID | Fields | Purpose |
|-----------|------------|--------|---------|
| `subscribers` | `chatID` or `chatID:threadID` | chatID, threadID, types, components | Telegram chat or topic subscribers with subscription settings |
| `component_statuses` | component ID | status | Last seen component status by component ID |
| `pending_component_events` | component ID | — | Component changes saved before fan-out so retryable delivery failures can be resumed |
| `incident_update_versions` | update ID | version | Seen incident update version by update ID |
| `delivery` | `eventKey\|subscriber` | eventKey, subscriber, expiresAt (TTL 7 days) | Temporary per-event subscriber delivery state for retry isolation |
| `meta` | `initialized` or `telegramOffset` | value | Baseline seed marker and last processed Telegram update offset |

Subscriber document IDs are `chatID` or `chatID:threadID`. Each subscriber document includes subscription types and component ID filters as fields within the document.

## Runtime

The service uses Telegram `getUpdates`, so it does not need a public webhook URL. On startup it starts a local health endpoint at `127.0.0.1:8080/healthz`, calls `deleteWebhook` before long polling, registers the Telegram command menu, and then starts polling. MongoDB is configured with `MONGODB_URI` and `MONGODB_DATABASE` (default `openai_status_bot`). The OpenAI status source is fixed to `https://status.openai.com`. Docker Compose starts the bot connecting to a managed MongoDB Atlas cluster; there is no bundled local MongoDB service.

## Failure Behavior

- First successful poll seeds state without notification.
- OpenAI fetch failures are logged and retried on the next interval.
- Retryable Telegram failures do not advance component or incident checkpoints.
- Successful per-subscriber deliveries are tracked temporarily, so retrying one failed subscriber does not resend to already-delivered subscribers.
- Pending component events are stored before delivery and removed only after successful delivery or terminal subscriber cleanup.
- Telegram 403 and selected terminal 400 errors remove the unreachable subscriber, then delivery continues.
- Malformed subscriber document IDs are removed from MongoDB and surfaced as an error for the current poll instead of being skipped silently.
- MongoDB connection failure at startup exits the process.
