---
phase: 5
title: "Test and documentation update"
status: completed
priority: P1
dependencies: [4]
---

# Phase 5: Test and documentation update

## Overview

Lock the migration down with focused tests, full test execution, and documentation updates for the new runtime model.

## Requirements

- Functional: existing bot commands and notification behavior remain equivalent.
- Non-functional: tests should protect the new framework boundary without requiring a live Telegram token.

## Architecture

Test the app at three levels:

1. Command unit tests for `internal/bot` using local message contexts or framework `models.Update` with fake sender/store/status clients.
2. Telegram sender/error tests for `internal/telegram` using framework params, fake API caller where possible, or local error mapping tests.
3. Startup/compile verification through package tests and `go test ./...`.

Do not add live Telegram network tests.

## Related Code Files

| Action | File | Notes |
|---|---|---|
| Modify | `internal/bot/bot_test.go` | Replace manual polling tests with handler dispatch tests |
| Modify | `internal/telegram/client_test.go` | Convert to sender/error/param tests or replace with new test files |
| Modify | `internal/poller/poller_test.go` | Keep fake notifier behavior; update imports if error type moves |
| Modify | `README.md` | Update dependency/runtime note only if user-visible setup changes |
| Modify | `docs/system-architecture.md` | Required: framework polling, restart-seed offset |
| Modify | `docs/setup-guide.md` | Update only if commands/setup text references custom long polling details |
| Modify | `plans/reports/260626-1602-go-telegram-framework-research.md` | Optional: add note that user chose framework-style migration |

## Implementation Steps

1. Add command dispatch tests:
   - `/start` subscribes chat;
   - `/start` in topic subscribes `chatID:threadID`;
   - `/start@OtherBot` ignored;
   - `/start@OpenAIStatusBot` accepted;
   - unknown command replies with help hint;
   - non-command text ignored.
2. Add reply/send tests:
   - HTML parse mode set;
   - link preview disabled;
   - topic reply includes `MessageThreadID`;
   - send errors are logged but do not panic command handlers.
3. Add terminal error tests:
   - 403 terminal;
   - 400 chat/thread missing terminal;
   - 400 parse entities non-terminal.
4. Add offset conversion tests:
   - no stored offset => no initial offset option;
   - stored offset 1 => initial last update 0;
   - stored offset N => initial last update N-1.
5. Run focused tests:
   - `go test ./internal/bot ./internal/telegram ./internal/poller`
6. Run broad tests:
   - `go test ./...`
7. Run integration tests only when Docker is available and needed:
   - `go test -tags=integration ./internal/mongostore/...`
8. Update docs after behavior is verified.

## Success Criteria

- [x] Focused bot/telegram/poller tests pass.
- [x] `go test ./...` passes.
- [x] Docs mention `go-telegram/bot` runtime and offset restart-seed semantics.
- [x] README remains accurate for local and Docker startup.
- [x] No stale custom Telegram client symbols remain under `internal` or `cmd`.

## Risk Assessment

- Risk: tests overfit framework internals.
  Mitigation: assert project-visible behavior and adapter params, not private framework state.
- Risk: docs over-explain internals to end users.
  Mitigation: README stays user-focused; detailed offset semantics go in `docs/system-architecture.md`.
- Risk: integration tests fail due Docker absence.
  Mitigation: keep Mongo integration tests behind existing build tag and report if not run.

