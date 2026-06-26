---
phase: 1
title: "Dependency and architecture setup"
status: completed
priority: P1
dependencies: []
---

# Phase 1: Dependency and architecture setup

## Overview

Introduce `go-telegram/bot` and define the local architecture boundaries before moving command behavior. This phase should compile toward the new dependency but may keep old runtime code until later phases finish.

## Requirements

- Functional: add `github.com/go-telegram/bot@v1.21.0`; identify startup, command, notification, and error integration points.
- Non-functional: keep dependency footprint small; no webhook mode; no debug logging of Telegram requests.

## Architecture

Keep these boundaries:

- `cmd/openai-status-bot/main.go` owns process wiring and framework lifecycle.
- `internal/bot` owns command behavior and handler registration.
- `internal/telegram` owns notification sending and framework error translation for poller use.
- `internal/poller` should not import `github.com/go-telegram/bot` directly.

Preferred naming:

- Import framework package as `tgbot` inside project package `internal/bot` to avoid `bot.Bot` naming conflicts.
- Import models as `tgmodels`.
- If refactoring internal command runtime, prefer `type App struct` over another `type Bot struct`.

## Related Code Files

| Action | File | Notes |
|---|---|---|
| Modify | `go.mod` / `go.sum` | Add `github.com/go-telegram/bot@v1.21.0` |
| Modify | `cmd/openai-status-bot/main.go` | Prepare framework construction and lifecycle wiring |
| Modify | `internal/bot/bot.go` | Plan rename/split from polling loop to app handler container |
| Modify | `internal/telegram/client.go` | Prepare for deletion or shrink to sender/error adapter |
| Reference | `plans/reports/260626-1602-go-telegram-framework-research.md` | Prior library comparison |

## Implementation Steps

1. Run `go get github.com/go-telegram/bot@v1.21.0`.
2. Confirm `go mod tidy` does not introduce unexpected third-party dependencies.
3. Add local package aliases in planned files: `tgbot "github.com/go-telegram/bot"` and `tgmodels "github.com/go-telegram/bot/models"`.
4. Define the target startup sequence in `main.go` before code movement:
   - load config/logger/context/health/Mongo;
   - build status client/store;
   - load stored Telegram offset if retained;
   - create framework bot with options;
   - delete webhook, set commands, resolve username;
   - create `internal/bot.App`;
   - register handlers;
   - create `internal/telegram.Sender` for poller notifications;
   - start status poller goroutine;
   - mark readiness;
   - call `tg.Start(ctx)`.
5. Decide initial file boundaries:
   - `internal/bot/app.go` for app struct/registration;
   - `internal/bot/message_context.go` or helper functions for model conversion;
   - `internal/telegram/sender.go` for poller notification sending;
   - `internal/telegram/errors.go` for terminal error classification.

## Success Criteria

- [x] `go.mod` contains `github.com/go-telegram/bot v1.21.0`.
- [x] Plan-confirmed architecture avoids importing framework types into `internal/poller`.
- [x] Startup sequence is documented in code comments only where non-obvious.
- [x] No source file grows past 200 lines without a modularization check.

## Risk Assessment

- Risk: package name collision between project `internal/bot` and framework `bot`.
  Mitigation: always alias framework import as `tgbot`.
- Risk: framework debug/error logging leaks token-bearing URLs.
  Mitigation: do not enable `WithDebug`; route framework errors through a redacting slog handler.
- Risk: migration becomes a broad rewrite.
  Mitigation: keep OpenAI status formatting, Mongo store, and poller event logic unchanged.

