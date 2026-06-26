---
type: research-report
topic: go-telegram-framework-selection
conducted_at: 2026-06-26 16:02 Asia/Saigon
status: complete
---

# Research Report: Go Telegram Framework For OpenAI Status Bot

## Executive Summary

Recommendation: do not replace the current custom Telegram client unless we need faster Bot API feature coverage. Current code uses only five API calls and already has behavior this project cares about: explicit offset persistence after command handling, token redaction, typed API errors, request timeout control, `message_thread_id`, and narrow test fakes.

If we still apply a framework, pick `github.com/mymmrac/telego` behind this repo's existing `internal/telegram` interface. It exposes manual `GetUpdates(ctx, params)`, current Bot API types, `message_thread_id`, `setMyCommands`, `deleteWebhook`, and `sendMessage`. Avoid its `UpdatesViaLongPolling` helper because it advances offset internally before this app can persist offset after handling.

Do not pick `go-telegram-bot-api/v5` for this repo: latest module tag is from 2021 and its tagged source does not expose `message_thread_id`, so it regresses supergroup topic support. `go-telegram/bot` is current and zero-dependency, but its normal polling model owns offset progression internally, which mismatches this app's Mongo-backed offset contract. `telebot.v3` is handler-framework oriented and would force more rewrite than value. `gotgbot/v2` is capable but still release-candidate at latest.

## Research Methodology

- Sources consulted: 13 primary/local sources.
- Date range: 2021-12-13 to 2026-06-14 module releases, checked on 2026-06-26.
- Search terms: `Go Telegram Bot API framework`, `telego GetUpdates`, `go-telegram bot MessageThreadID`, `telegram-bot-api v5 message_thread_id`, `telebot.v3 polling`.
- Source types: Telegram official Bot API docs, pkg.go.dev, Go module metadata via `go list`, downloaded module READMEs/source, current repo source.
- Docs-seeker: checked; context7 had no docs for `go-telegram bot Telegram Bot API Go`, fallback used module sources.

## Table Of Contents

- [Project Fit](#project-fit)
- [Key Findings](#key-findings)
- [Comparative Analysis](#comparative-analysis)
- [Recommendation](#recommendation)
- [Implementation Notes](#implementation-notes)
- [Resources](#resources)
- [Unresolved Questions](#unresolved-questions)

## Project Fit

Current Telegram surface:

- `deleteWebhook(drop_pending_updates=false)`
- `getMe`
- `setMyCommands`
- `getUpdates(offset, timeout, allowed_updates=["message"])`
- `sendMessage(chat_id, text, parse_mode=HTML, disable preview, optional message_thread_id)`

Important local constraints:

- `internal/bot.Bot.Run` loads Telegram offset from MongoDB.
- It handles each update, then saves `update_id + 1`.
- This creates a conservative delivery contract: do not confirm future offset before app state is handled.
- Existing client redacts bot token from transport errors.
- `IsTerminalSendError` maps 403 and selected 400 descriptions to subscriber cleanup.

## Key Findings

### 1. Technology Overview

Telegram Bot API works fine with a thin HTTP wrapper. A framework adds value when the bot needs rich handlers, callback routing, media upload helpers, payments, inline mode, web apps, or faster coverage of new Bot API fields.

This project is not there yet. It is a polling status-notification bot with simple text commands. Most complexity is OpenAI-status dedupe, Mongo state, and delivery semantics, not Telegram routing.

### 2. Current State And Trends

Module metadata checked with `go list -m -json <module>@latest`:

| Module | Latest | Release time | Go directive | Fit |
|---|---:|---|---:|---|
| `github.com/mymmrac/telego` | `v1.10.0` | 2026-06-14 | `1.25.7` | Best external fit |
| `github.com/go-telegram/bot` | `v1.21.0` | 2026-05-22 | `1.18` | Good library, poorer offset fit |
| `github.com/PaulSonOfLars/gotgbot/v2` | `v2.0.0-rc.35` | 2026-05-25 | `1.24` | Current but pre-release |
| `gopkg.in/telebot.v3` | `v3.3.8` stable, `v3.4.2-beta` exists | 2024-08-06 stable | `1.16` | Framework-heavy |
| `github.com/go-telegram-bot-api/telegram-bot-api/v5` | `v5.5.1` | 2021-12-13 | `1.16` | Not suitable for topics |

`go-telegram/bot` README says it supports Bot API 10.0 and is zero-dependency. Good signal. But its exported API is handler/poller oriented.

`telego` is generated/current and exposes manual `GetUpdates`. Bad signal: dependency cost is materially larger (`fasthttp`, custom JSON libs, Sonic, etc.) and its module says Go `1.25.7`, while this repo says Go `1.25.0`.

### 3. Best Practices

- Keep this repo's `internal/telegram` boundary. Do not let a framework leak into `internal/bot` or `internal/poller`.
- Preserve explicit offset persistence. Use manual `GetUpdates`, not framework-owned long polling.
- Preserve token redaction tests.
- Preserve terminal send error classification. If using telego, map `errors.As(err, *telegoapi.Error)` to existing `APIError` or update `IsTerminalSendError`.
- Keep `allowed_updates=["message"]`.
- Keep `deleteWebhook(drop_pending_updates=false)` on startup.
- Use context deadlines per request. For long polling, deadline should be `timeoutSeconds + HTTP_TIMEOUT`, not just `HTTP_TIMEOUT`.

### 4. Security Considerations

- Bot token must never enter logs. Current client explicitly redacts URL-bearing transport errors.
- Framework default loggers can log request failures. Disable or replace framework logger.
- Do not enable debug logging around Telegram requests in production.
- Keep command parsing defensive. Do not trust message text, chat type, or thread ID.
- Avoid webhook mode unless deploying with HTTPS, secret token validation, and replay-safe handler.

### 5. Performance Insights

- Traffic is tiny. Performance should not drive selection.
- `telego` and `gotgbot` are more complete than needed.
- Long polling dominates latency. Any library using `getUpdates` is good enough.
- Dependency count matters more than raw throughput for this app.

## Comparative Analysis

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Keep custom client | Exact behavior, no new deps, easy fakes, offset contract already right | Manual maintenance for new Bot API fields | Best default |
| `mymmrac/telego` | Current, manual `GetUpdates`, topics, typed methods, error type | Heavier deps, Go `1.25.7`, migration still needed | Best if adopting |
| `go-telegram/bot` | Current, zero deps, Bot API 10.0, nice handlers | Polling owns offset, handler model duplicates app command dispatch | Good only if accepting offset model change |
| `gotgbot/v2` | Generated, current, zero third-party deps, manual methods | Latest is RC, generated API can be verbose | Watch, not first choice |
| `telebot.v3` | Mature handler framework, topic support | Framework rewrite, stable tag older, unnecessary features | Not for this bot |
| `telegram-bot-api/v5` | Simple wrapper, classic package | Latest tag old, no topic field in tagged source | Reject |

## Recommendation

### Decision

Keep the custom client now. If user explicitly wants a framework migration, use `telego` only inside `internal/telegram`.

### Why

The app's actual Telegram need is tiny. Migrating only pays off if maintaining Bot API models becomes painful. The current code is closer to the desired operational semantics than most frameworks.

### Acceptance Criteria For A Telego Migration

- `go test ./...` passes.
- `internal/bot` and `internal/poller` interfaces do not import `telego`.
- Existing Telegram client tests still cover:
  - `deleteWebhook` keeps pending updates.
  - `message_thread_id` included for topic subscriptions.
  - token redaction.
  - terminal send errors.
  - long poll request timeout is `telegram timeout + HTTP_TIMEOUT`.
- Offset saved only after `handleMessage` completes.

## Implementation Notes

### Quick Start

```bash
go get github.com/mymmrac/telego@v1.10.0
```

Then rewrite only `internal/telegram/client.go` and `internal/telegram/types.go` conversion helpers.

### Adapter Sketch

```go
package telegram

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
	"github.com/mymmrac/telego/telegoutil"
)

type Client struct {
	bot            *telego.Bot
	requestTimeout time.Duration
}

func NewClient(token string, timeout time.Duration) (*Client, error) {
	b, err := telego.NewBot(
		token,
		telego.WithHTTPClient(&http.Client{}),
		telego.WithDefaultLogger(false, false),
	)
	if err != nil {
		return nil, err
	}
	return &Client{bot: b, requestTimeout: timeout}, nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second+c.requestTimeout)
	defer cancel()

	updates, err := c.bot.GetUpdates(reqCtx, &telego.GetUpdatesParams{
		Offset:         int(offset),
		Timeout:        timeoutSeconds,
		AllowedUpdates: []string{telego.MessageUpdates},
	})
	if err != nil {
		return nil, mapTelegoError(err)
	}
	return convertUpdates(updates), nil
}

func (c *Client) SendText(ctx context.Context, chatID int64, threadID *int, text string) error {
	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	params := &telego.SendMessageParams{
		ChatID:             telegoutil.ID(chatID),
		Text:               text,
		ParseMode:          telego.ModeHTML,
		LinkPreviewOptions: &telego.LinkPreviewOptions{IsDisabled: true},
	}
	if threadID != nil {
		params.MessageThreadID = *threadID
	}

	_, err := c.bot.SendMessage(reqCtx, params)
	return mapTelegoError(err)
}

func mapTelegoError(err error) error {
	var apiErr *telegoapi.Error
	if errors.As(err, &apiErr) {
		return &APIError{ErrorCode: apiErr.ErrorCode, Description: apiErr.Description}
	}
	return err
}
```

This is intentionally a sketch. Validate exact conversion code in tests.

### Common Pitfalls

- Do not use `telego.UpdatesViaLongPolling`; it mutates offset internally before this repo persists offset.
- Do not set `http.Client.Timeout` to `HTTP_TIMEOUT` globally; long polling needs longer.
- Do not leak telego types outside `internal/telegram`.
- Do not drop topic support while converting `MessageThreadID`.
- Do not remove token-redaction coverage without replacing it.

## Resources

### Official Documentation

- Telegram Bot API: https://core.telegram.org/bots/api
- Go package docs, telego: https://pkg.go.dev/github.com/mymmrac/telego
- Go package docs, go-telegram/bot: https://pkg.go.dev/github.com/go-telegram/bot
- Go package docs, gotgbot/v2: https://pkg.go.dev/github.com/PaulSonOfLars/gotgbot/v2
- Go package docs, telebot.v3: https://pkg.go.dev/gopkg.in/telebot.v3
- Go package docs, telegram-bot-api/v5: https://pkg.go.dev/github.com/go-telegram-bot-api/telegram-bot-api/v5

### Repository References

- telego: https://github.com/mymmrac/telego
- go-telegram/bot: https://github.com/go-telegram/bot
- gotgbot: https://github.com/PaulSonOfLars/gotgbot
- telebot: https://github.com/tucnak/telebot
- telegram-bot-api: https://github.com/go-telegram-bot-api/telegram-bot-api

### Local Evidence

- `internal/telegram/client.go`: current custom API wrapper.
- `internal/bot/bot.go`: offset persistence after update handling.
- `README.md`: topic support and long-polling startup behavior.
- `go list -m -json <module>@latest`: current module versions above.

## Appendix A: Glossary

- Bot API: Telegram HTTPS API for bot accounts.
- MTProto: Telegram client protocol. Not needed here.
- Long polling: `getUpdates` request waits for new updates.
- Offset: Telegram update checkpoint. Higher offset confirms older updates.
- Topic: Supergroup forum thread, sent via `message_thread_id`.

## Appendix B: Version Compatibility Matrix

| Project constraint | Custom | telego | go-telegram/bot | gotgbot/v2 | telebot.v3 | telegram-bot-api/v5 |
|---|---:|---:|---:|---:|---:|---:|
| Go 1.25 project | yes | maybe needs 1.25.7 | yes | yes | yes | yes |
| Manual `GetUpdates` | yes | yes | no public manual method found | yes | not primary path | yes |
| Topic send support | yes | yes | yes | yes | yes | no in latest tag |
| Low dependency footprint | yes | no | yes | yes | medium | yes |
| Minimal migration | yes | medium | high | medium | high | medium |

## Next Steps

1. Keep current client unless a concrete Telegram API gap appears.
2. If migrating, create a small telego adapter under `internal/telegram`.
3. Run focused tests first: `go test ./internal/telegram ./internal/bot ./internal/poller`.
4. Then run `go test ./...`.
5. If Go toolchain upgrade appears due telego `go 1.25.7`, decide whether that is acceptable before merge.

## Unresolved Questions

- Is the user goal to reduce maintenance, or to add upcoming Telegram Bot API features?
- Is Go toolchain auto-upgrade to 1.25.7 acceptable if telego requires it?
