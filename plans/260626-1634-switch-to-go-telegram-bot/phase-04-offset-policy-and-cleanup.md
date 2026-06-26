---
phase: 4
title: "Offset policy and cleanup"
status: completed
priority: P2
dependencies: [2, 3]
---

# Phase 4: Offset policy and cleanup

## Overview

Adapt Mongo `telegramOffset` to the framework polling model and remove custom client leftovers after command and notification paths are migrated.

## Requirements

- Functional: restarts should not intentionally replay old command updates when a stored offset exists.
- Functional: the app must tolerate missing/zero offset on fresh databases.
- Non-functional: document the weaker offset guarantee introduced by framework-owned polling.

## Architecture

`go-telegram/bot` stores `lastUpdateID` internally and calls `getUpdates` with `lastUpdateID + 1`. Current Mongo `telegramOffset` stores the next update ID to fetch. Therefore:

- if stored offset > 0, configure `WithInitialOffset(storedOffset - 1)`;
- if stored offset == 0, omit `WithInitialOffset`;
- after handling an update, optionally call `SaveTelegramOffset(update.ID + 1)` as a restart seed;
- do not rely on Mongo offset as a strict Telegram confirmation boundary.

Use low-buffer sequential options to reduce the gap between framework receiving updates and app handling them:

```go
tgbot.WithWorkers(1)
tgbot.WithUpdatesChannelCap(1)
tgbot.WithNotAsyncHandlers()
```

## Related Code Files

| Action | File | Notes |
|---|---|---|
| Modify | `cmd/openai-status-bot/main.go` | Load offset and configure `WithInitialOffset` |
| Modify | `internal/bot/app.go` or `internal/bot/bot.go` | Save restart seed after handling update |
| Modify | `internal/mongostore/checkpoint.go` | Keep offset methods unless a later decision removes them |
| Modify | `docs/system-architecture.md` | Update runtime/failure behavior |
| Delete | `internal/telegram/client.go` | Remove custom `postJSON`, `GetUpdates`, `DeleteWebhook`, `SetMyCommands`, `GetMe` once unused |
| Delete | `internal/telegram/types.go` | Remove custom Telegram API model types once unused |

## Implementation Steps

1. Load `store.TelegramOffset(ctx)` in `main.go` before creating framework bot.
2. Convert stored next-offset to framework initial last-update:
   - `initialLastUpdateID := offset - 1` when offset > 0;
   - no option when offset <= 0.
3. Add `SaveTelegramOffset(update.ID + 1)` after command handler completion.
4. Log offset-save failures as warnings, same as current code.
5. Remove old `Bot.Run` manual polling path and any unused fake `GetUpdates` interfaces.
6. Remove `internal/telegram/client.go` and `types.go` only after sender/error replacements compile.
7. Re-run `rg "GetUpdates|telegram.Update|telegram.Message|telegram.BotCommand|telegram.User" internal cmd` and remove stale references.

## Success Criteria

- [x] Fresh DB starts without offset errors.
- [x] Existing DB with `telegramOffset=N` starts framework polling from update `N`.
- [x] Command handler saves `update.ID + 1` as restart seed after handling.
- [x] No custom HTTP Telegram API client remains.
- [x] Architecture docs describe framework-owned polling and restart-seed offset semantics.

## Risk Assessment

- Risk: exact post-handler Telegram confirmation is no longer guaranteed.
  Mitigation: document trade-off; use sequential/low-buffer framework options; keep offset as restart seed.
- Risk: off-by-one offset skip/replay.
  Mitigation: add tests for offset 0, offset 1, and offset N conversion to `WithInitialOffset`.
- Risk: unused Mongo offset state becomes misleading.
  Mitigation: rename docs/comments to "restart seed"; consider later migration removing `telegramOffset` if not useful.

