---
phase: 3
title: "Notification sender migration"
status: completed
priority: P1
dependencies: [1]
---

# Phase 3: Notification sender migration

## Overview

Replace custom notification sends with a `go-telegram/bot` backed sender while preserving poller retry isolation and terminal subscriber cleanup.

## Requirements

- Functional: `poller.Runner` can still call `SendMessage(ctx, subscriber, text) error`.
- Functional: HTML parse mode and disabled link previews remain.
- Functional: `message_thread_id` remains for topic subscribers.
- Functional: terminal send errors still remove unreachable subscribers.
- Non-functional: `internal/poller` must stay independent from framework types.

## Architecture

Keep a local adapter package:

```go
package telegram

type Sender struct {
    bot *tgbot.Bot
}

func (s *Sender) SendMessage(ctx context.Context, sub mongostore.Subscriber, text string) error
func IsTerminalSendError(err error) bool
```

Map framework/API errors into a local `APIError` shape if needed. Preserve the existing semantic contract used by `internal/poller/delivery.go`: 403 is terminal; selected 400 descriptions are terminal; parse/entity errors are retryable.

## Related Code Files

| Action | File | Notes |
|---|---|---|
| Modify/create | `internal/telegram/sender.go` | Framework-backed poller sender |
| Modify/create | `internal/telegram/errors.go` | API error mapping and terminal classification |
| Modify | `internal/poller/delivery.go` | Should require little or no change |
| Modify | `cmd/openai-status-bot/main.go` | Pass sender adapter to `poller.NewRunner` |
| Modify | `internal/poller/poller_test.go` | Keep fake notifier tests working |
| Modify/delete | `internal/telegram/client_test.go` | Replace HTTP-client tests with sender/error tests |

## Implementation Steps

1. Create `telegram.Sender` around `*tgbot.Bot`.
2. Implement send params:
   - `ChatID: subscriber.ChatID`;
   - `Text: text`;
   - `ParseMode: models.ParseModeHTML`;
   - `LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: true}`;
   - `MessageThreadID: *subscriber.ThreadID` when non-nil.
3. Preserve a local `APIError` type if framework errors do not expose the exact fields poller needs.
4. Implement error redaction for logs:
   - no debug request logging;
   - framework errors passed through `redactToken(err, token)` before logging where token may appear.
5. Keep `telegram.IsTerminalSendError` signature unchanged so `poller` does not care about framework internals.
6. Update `main.go` to instantiate one framework bot and one notification sender from it.

## Success Criteria

- [x] Poller notification tests still prove per-subscriber retry behavior.
- [x] Terminal 403 and terminal 400 messages still remove unreachable subscribers.
- [x] Non-terminal 400 parse errors remain retryable.
- [x] Topic notification sends include `message_thread_id`.
- [x] `internal/poller` imports no `github.com/go-telegram/bot` package.

## Risk Assessment

- Risk: framework error type does not include HTTP status and Telegram `error_code` in the same shape.
  Mitigation: inspect returned errors and map by `errors.As` or message fallback with tests.
- Risk: link preview field changed from deprecated `disable_web_page_preview` to `link_preview_options`.
  Mitigation: test the framework params object, not raw JSON field, and verify Telegram docs support disabled previews.
- Risk: bot token appears in transport errors.
  Mitigation: carry the token into adapter only for redaction; never log raw framework debug output.

