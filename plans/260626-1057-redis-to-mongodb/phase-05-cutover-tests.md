---
phase: 5
title: "Cutover & Tests"
status: complete
effort: "M"
---

# Phase 5: Cutover & Tests

## Overview

Swap all consumers from `redisstore` to `mongostore`, delete `redisstore`, purge the Redis dependencies, and re-establish the test suite on a MongoDB-backed harness (replacing `miniredis`). After this phase the project builds and tests green with zero Redis.

## Requirements

- All `redisstore.X` references → `mongostore.X` (type names identical; only the qualifier changes).
- `internal/redisstore` deleted; `go-redis` and `miniredis` removed from go.mod/go.sum.
- Tests pass against a real `mongod` test harness.

## Architecture: test backend

<!-- Updated: Validation Session 1 - testcontainers chosen but gated behind build tag; not run this phase; poller/bot use in-test fake -->

`miniredis` (pure-Go, in-memory, no binary) has **no MongoDB equivalent** — real-backend tests must run a real `mongod`.

**Decision (validated): `testcontainers-go/modules/mongodb`, but gated and deferred.** The `mongostore` integration suite is written behind a build tag (`//go:build integration`) so it is **NOT part of default `go test ./...`** in this phase. Default test runs must stay Docker-free. The user runs `go test -tags=integration ./internal/mongostore/...` later, when Docker is available.

- `internal/mongostore/store_test.go` → tag `//go:build integration`; a `testMongo(t)` helper starts a `mongo:7` container once, returns a `*mongostore.Store` against a unique per-test database, and `t.Cleanup` drops it.
- **Poller/bot tests use an in-test fake `Store`** (hand-written, satisfies the interface) — fast, Docker-free, runs in default `go test`. No real DB in those suites.
- So in this phase: `go test ./...` (no tag) passes with zero Docker — poller/bot/config/etc. on fakes, `mongostore` integration tests skipped by the build tag. CI stays green without Docker; real-backend validation is a deliberate manual/later step.

## Related Code Files

- Modify (qualifier swap `redisstore.` → `mongostore.`, update imports):
  - `internal/poller/poller.go`, `internal/poller/event_collection.go`, `internal/poller/delivery.go`
  - `internal/bot/bot.go`, `internal/bot/commands.go`, `internal/bot/helpers.go`, `internal/bot/subscription_format.go`
  - `internal/telegram/client.go` (`Subscriber` param at `client.go:108`)
- Create: `internal/mongostore/store_test.go` — `//go:build integration` tag; port the 243-line `redisstore/store_test.go` to the testcontainers harness (same assertions: subscriber CRUD, settings normalization/self-heal, delivery dedup + TTL-index presence, incident version dedup, offset, initialized). Excluded from default `go test`.
- Modify: `internal/poller/poller_test.go`, `internal/bot/bot_test.go` — swap `redisstore` import; replace any `miniredis` usage with a lightweight **in-test fake** of the `Store` interface (Docker-free, fast). Do not point these at the real harness.
- Create (if not already present in those tests): a shared fake `Store` (e.g. an in-memory map-backed type in a test helper) satisfying both `poller.Store` and `bot.Store`.
- Delete: `internal/redisstore/` (all 6 files).
- Modify: `go.mod`/`go.sum` — remove `github.com/redis/go-redis/v9`, `github.com/alicebob/miniredis/v2` (+ transitive `gopher-lua`, `go-rendezvous`, `xxhash` if now unused); add `go.mongodb.org/mongo-driver/v2` and the chosen test dep. Run `go mod tidy`.

## Implementation Steps

1. Add compile-time interface assertions in `mongostore` (`var _ poller.Store`, `var _ bot.Store`) — now imports resolve both ways; fix any signature drift from Phase 3.
2. Swap qualifiers across poller/bot/telegram (grep `redisstore\.` → confirm each maps to an identical `mongostore` symbol).
3. `rm -rf internal/redisstore`; `go build ./...` and fix remaining references.
4. Build the in-test fake `Store`; swap poller/bot tests onto it. Port `store_test.go` to the testcontainers harness under `//go:build integration`.
5. `go mod tidy`; confirm Redis deps gone (`grep redis go.mod` empty).
6. `go test ./...` (no tag) — must pass **without Docker** (poller/bot on fakes, integration suite excluded). `go vet ./...`. Optionally `go build -tags=integration ./...` to confirm the gated suite compiles.

## Success Criteria

- [ ] `go build ./...` clean; no `redisstore` references; `internal/redisstore` deleted.
- [ ] `grep -ri redis` finds no code/config/doc references (outside this plan).
- [ ] `go test ./...` (no tag) green **without Docker**; gated `mongostore` suite compiles under `-tags=integration` (run later by the user).
- [ ] `go mod tidy` removed `go-redis` + `miniredis`; added mongo driver + testcontainers (test-only).

## Risk Assessment

- Risk: CI lacks Docker for testcontainers. Resolved by decision: integration suite is behind `//go:build integration` and excluded from default `go test`; user runs it manually later. Document the `-tags=integration` + Docker requirement in README.
- Risk: in-test fake `Store` drifts from real `mongostore` semantics (the fake passes but Mongo behaves differently). Mitigation: the gated integration suite is the real-behavior backstop; user must run it before trusting the cutover in production.
- Risk: TTL behavior not directly assertable (sweep ~60s). Mitigation: assert the TTL **index exists** with `ExpireAfterSeconds==604800` rather than waiting for expiry; trust MongoDB to enforce.
- Risk: hidden semantic drift between fake `Store` and real `mongostore` in poller/bot tests. Mitigation: keep at least the `mongostore` suite exercising real behavior end-to-end for the store contract.

## Post-cutover docs

Update the three docs that reference Redis: `docs/system-architecture.md`, `docs/setup-guide.md`, `docs/component-id-monitoring-report.md` (verify each via `grep -li redis docs/`). Add a README note that switching from a prior Redis deploy starts from empty state (no migration; subscribers re-`/start`).
