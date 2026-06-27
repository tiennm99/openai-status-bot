---
title: "Switch Telegram layer to go-telegram/bot"
description: "Replace the custom Telegram HTTP client and command polling loop with the standard go-telegram/bot framework while preserving subscriptions, topic replies, notification delivery, and startup behavior."
status: completed
priority: P2
branch: "develop"
tags: [refactor, backend, telegram]
blockedBy: []
blocks: []
created: "2026-06-26"
createdBy: "ck:plan"
source: skill
---

# Switch Telegram layer to go-telegram/bot

## Overview

Migrate the Telegram integration from the custom `internal/telegram` HTTP client to `github.com/go-telegram/bot` (`v1.21.0`, Bot API 10.0, zero third-party deps). This is a framework-style refactor: `go-telegram/bot` owns long polling and update dispatch; app code owns command behavior, subscriber state, and notification delivery.

The accepted trade-off: Mongo `telegramOffset` stops being a strict post-handler confirmation checkpoint. It remains a restart seed when present, and command handlers can still save the latest handled update ID for replay reduction. This is acceptable because the user prioritized a good standard library and accepted larger changes.

## Scope Challenge

- Existing code: command parsing/replies are in `internal/bot`; delivery fan-out and terminal subscriber cleanup are in `internal/poller`; Telegram HTTP calls/errors are isolated in `internal/telegram`.
- Minimum changes: add `go-telegram/bot`, refactor startup wiring, convert command handlers to framework update handling, replace notification sender, update tests/docs.
- Complexity: expected >8 files touched. Justified because the current custom client, command loop, model types, tests, and architecture docs all encode Telegram transport assumptions.
- Selected mode: HOLD SCOPE. No webhook migration, no interactive keyboards, no callback queries, no feature expansion.

## Architecture Decision

Use `go-telegram/bot` as the runtime and handler framework. Keep project-specific behavior behind local packages:

```text
cmd/openai-status-bot/main.go
  -> creates *tgbot.Bot with options
  -> deleteWebhook, setMyCommands, getMe
  -> internal/bot.App registers command handlers
  -> internal/telegram.Sender sends poller notifications

internal/bot
  -> owns command dispatch and subscription business logic
  -> consumes go-telegram/bot models at the edge

internal/telegram
  -> owns poller notification sender and terminal-error classification
  -> wraps framework errors so poller logic stays stable

internal/poller
  -> keeps Notifier interface unchanged
```

Runtime options should favor predictable command handling:

- `bot.WithAllowedUpdates(bot.AllowedUpdates{"message"})`
- `bot.WithInitialOffset(storedOffset-1)` only when stored offset > 0
- `bot.WithWorkers(1)`
- `bot.WithUpdatesChannelCap(1)`
- `bot.WithNotAsyncHandlers()`
- custom `WithErrorsHandler` / no debug logging with token redaction

## Cross-Plan Dependencies

None. The prior Redis-to-Mongo plan is complete and does not block this work.

## Phases

| Phase | Name | Status |
|-------|------|--------|
| 1 | [Dependency and architecture setup](./phase-01-dependency-and-architecture-setup.md) | Completed |
| 2 | [Command handler migration](./phase-02-command-handler-migration.md) | Completed |
| 3 | [Notification sender migration](./phase-03-notification-sender-migration.md) | Completed |
| 4 | [Offset policy and cleanup](./phase-04-offset-policy-and-cleanup.md) | Completed |
| 5 | [Test and documentation update](./phase-05-test-and-documentation-update.md) | Completed |

## Acceptance Criteria

- [x] Bot builds with `github.com/go-telegram/bot@v1.21.0`.
- [x] Custom Telegram `getUpdates` loop is removed from app code.
- [x] `/start`, `/stop`, `/status`, `/components`, `/subscribe`, `/history`, `/uptime`, `/info`, `/help`, and unknown-command behavior remain equivalent.
- [x] Supergroup topic support via `message_thread_id` remains for replies and notifications.
- [x] Poller delivery retry behavior and terminal subscriber cleanup remain equivalent.
- [x] Startup still deletes webhook with `drop_pending_updates=false`, registers commands, starts health endpoint, starts OpenAI poller, then starts Telegram long polling.
- [x] Tests cover framework handler dispatch, topic replies, notification sender params, terminal errors, offset seed behavior, and command menu registration.
- [x] `go test ./...` passes; integration tests remain gated behind `-tags=integration`.

## Not In Scope

- Webhook mode.
- New bot commands or UI features.
- Inline keyboards, callback queries, payments, web apps.
- Replacing MongoDB or changing subscription schema beyond optional offset cleanup.

## Research Inputs

- `plans/reports/260626-1602-go-telegram-framework-research.md`
- `github.com/go-telegram/bot@v1.21.0` module metadata: released 2026-05-22, Go directive 1.18.
- Local source confirmed support for `WithInitialOffset`, `WithAllowedUpdates`, `WithNotAsyncHandlers`, `SetMyCommands`, `DeleteWebhook`, `SendMessageParams.MessageThreadID`, and `LinkPreviewOptions`.

## Unresolved Questions

None.

