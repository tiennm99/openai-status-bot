---
phase: 2
title: "Command handler migration"
status: completed
priority: P1
dependencies: [1]
---

# Phase 2: Command handler migration

## Overview

Move command processing from the custom `GetUpdates` loop to `go-telegram/bot` handlers while preserving command behavior and testability.

## Requirements

- Functional: every existing command keeps output and storage behavior.
- Functional: commands with another bot username remain ignored.
- Functional: topic-specific subscriptions still use `message_thread_id`.
- Non-functional: command logic remains unit-testable without live Telegram HTTP calls.

## Architecture

Recommended shape:

```go
type App struct {
    sender ReplySender
    statusClient StatusClient
    store Store
    logger *slog.Logger
    username string
}

func (a *App) RegisterHandlers(tg *tgbot.Bot)
func (a *App) HandleUpdate(ctx context.Context, tg *tgbot.Bot, update *tgmodels.Update)
```

Use one framework default handler or `RegisterHandlerMatchFunc` for message commands, then reuse `normalizeCommand` for consistent `/cmd@BotName` behavior. Direct framework `MatchTypeCommandStartOnly` alone does not normalize own-bot suffixes; if using per-command registration, explicitly test `/start@<username>`.

Use a small local message context instead of threading framework models everywhere:

```go
type MessageContext struct {
    ChatID int64
    ThreadID *int
    Text string
}
```

Existing command methods can then migrate from `telegram.Message` to `MessageContext`.

## Related Code Files

| Action | File | Notes |
|---|---|---|
| Modify | `internal/bot/bot.go` | Remove manual `Run` loop; introduce app handler entrypoint |
| Modify | `internal/bot/helpers.go` | Change reply helpers from custom message type to local context |
| Modify | `internal/bot/commands.go` | Change command methods from custom message type to local context |
| Modify | `internal/bot/menu_commands.go` | Return `[]tgmodels.BotCommand` or build menu in `main.go` |
| Modify | `internal/bot/bot_test.go` | Replace fake `GetUpdates` tests with handler/dispatch tests |
| Delete or shrink | `internal/telegram/types.go` | Custom `Update`, `Message`, `Chat`, `User`, `BotCommand` should disappear if unused |

## Implementation Steps

1. Create `App` and `MessageContext` types.
2. Add helper conversion:
   - ignore nil update/message;
   - ignore empty/non-command text;
   - map `message.Chat.ID`;
   - map `message.MessageThreadID == 0` to `nil`, otherwise copy int pointer.
3. Move `handleMessage` into `App.HandleUpdate` or split into:
   - `handleUpdate(ctx, update)`;
   - `handleCommand(ctx, msgCtx)`.
4. Keep `normalizeCommand(text, username)` unchanged unless tests require framework model-specific adjustments.
5. Convert `reply`, `subscribe`, `unsubscribe`, `replyStatus`, `replySubscribe`, `replyHistory`, `replyUptime`, and `replyInfo` to accept `MessageContext`.
6. Register handlers in `main.go` after username is known:
   - safest path: `WithDefaultHandler(app.HandleUpdate)` or `RegisterHandlerMatchFunc(commandMessage, app.HandleUpdate)`;
   - use `WithNotAsyncHandlers` and `WithWorkers(1)` to preserve simple sequential command behavior.
7. Remove `TelegramClient.GetUpdates` dependency from `internal/bot`.

## Success Criteria

- [x] `internal/bot` no longer calls `GetUpdates`.
- [x] `/start@OtherBot` remains ignored.
- [x] `/start@OpenAIStatusBot` works when username is configured.
- [x] Topic replies send to the original `message_thread_id`.
- [x] Existing command tests still assert core behavior, but they use framework update models or local `MessageContext`.

## Risk Assessment

- Risk: losing own-bot command suffix behavior.
  Mitigation: keep `normalizeCommand`; add tests for own-bot and other-bot suffixes.
- Risk: framework async handler default introduces races.
  Mitigation: configure `WithNotAsyncHandlers()` and `WithWorkers(1)`.
- Risk: tests become HTTP-heavy.
  Mitigation: keep command logic behind local sender interface; unit-test without framework network calls.

