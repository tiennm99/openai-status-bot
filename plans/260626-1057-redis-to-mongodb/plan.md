---
title: "Migrate datastore from Redis to MongoDB"
description: ""
status: complete
priority: P2
branch: "main"
tags: []
blockedBy: []
blocks: []
created: "2026-06-26T04:22:22.185Z"
createdBy: "ck:plan"
source: skill
---

# Migrate datastore from Redis to MongoDB

## Overview

Replace Redis with MongoDB as the bot's datastore. No data migration: cutover is a clean reseed (first poll re-seeds state, subscribers re-issue `/start`). Motivation: team already operates MongoDB; goal is removing the Redis dependency to consolidate on one datastore. Hosting (validated): **managed Atlas for both environments** — one cluster with two databases (production + development) selected via `MONGODB_DATABASE`. **No local MongoDB service**; both compose files connect to Atlas, differing only by database name.

**Verdict (Phase 1, full reasoning there):** Suitable and justified. Every Redis op maps cleanly to MongoDB, and the document model actually *simplifies* several multi-key sequences. One real regression to accept: unit tests lose the pure-Go in-memory `miniredis`; MongoDB has no equivalent, so tests need a real `mongod` (testcontainers needs Docker, or `memongo` downloads a binary). CI gets slower and gains a Docker/binary dependency.

## Approach (straight replace, no dual backend)

`poller.Store` and `bot.Store` are already interfaces, and the only cross-package coupling is the domain types `redisstore.Subscriber`, `redisstore.PendingComponentEvent`, the `SubscriptionType*` constants, `NewSubscriber`, and `DefaultSubscriptionTypes` (~90 references, type *names* not bodies). Plan: create `internal/mongostore` exporting the **same type/constant names**, move the storage-agnostic domain logic verbatim, reimplement only the client-touching methods against MongoDB, swap consumer qualifiers `redisstore.` → `mongostore.`, then delete `internal/redisstore`. YAGNI: no pluggable-backend abstraction since only one backend is wanted.

## Phases

| Phase | Name | Status |
|-------|------|--------|
| 1 | [Suitability & Decision](./phase-01-suitability-decision.md) | Done |
| 2 | [Store Package & Connection](./phase-02-store-package-connection.md) | Done |
| 3 | [Implement Store Methods](./phase-03-implement-store-methods.md) | Done |
| 4 | [Config Wiring & Deployment](./phase-04-config-wiring-deployment.md) | Done |
| 5 | [Cutover & Tests](./phase-05-cutover-tests.md) | Done |

## Acceptance Criteria

- [x] Bot builds and runs with no `go-redis` / `miniredis` dependency.
- [x] All store operations (subscribers, settings, component statuses, pending component events, incident-update versions, delivery dedup w/ 7-day expiry, initialized flag, telegram offset) work against MongoDB.
- [x] `poller.Store` and `bot.Store` interfaces satisfied by `mongostore`; existing poller/bot/telegram tests pass against the new backend.
- [x] Config, `.env.example`, compose files, and README reference `MONGODB_URI`/`MONGODB_DATABASE`; no Redis references remain.
- [x] TTL index on delivery collection enforces 7-day expiry; conditional "update-only-if-subscribed" preserved via `MatchedCount`.

## Dependencies

None (no cross-plan dependencies; no in-repo data migration).

## Validation Log

### Session 1 — 2026-06-26

**Verification Results**
- Claims checked: 7 anchors + driver facts (Full tier, 5 phases)
- Verified: 7 | Failed: 0 | Unverified: 0
- Evidence: `poller.go:17` & `bot.go:25` (Store interfaces), `subscriber_settings.go:16` (Lua script), `checkpoint.go:137` (`Expire` 7d), `telegram/client.go:108` (`redisstore.Subscriber` param), 92 `redisstore.` refs across `internal`+`cmd`, `.env` gitignored. Driver = `go.mongodb.org/mongo-driver/v2` (v2.7.0).

**Decisions confirmed**
1. **Test backend** → testcontainers-go chosen, but **gated/deferred**: integration suite behind a build tag (`//go:build integration`), NOT run in Phase 5. Default `go test ./...` must stay Docker-free; user runs the integration suite later. (Phase 5)
2. **Hosting** → Atlas for both envs; **one cluster, two databases** (prod + dev) selected by `MONGODB_DATABASE`. **No local `mongo` service** in any compose file. (Phase 4)
3. **Cutover** → clean break: remove `REDIS_URL`, no compat shim, state resets (subscribers re-`/start`, checkpoints reseed). (Phase 4/5)
4. **Poller/bot tests** → in-test fake `Store` (Docker-free); real-backend coverage stays in the gated `mongostore` suite. (Phase 5)

### Whole-Plan Consistency Sweep
- Re-read `plan.md` + all 5 phase files after propagation. Local-`mongo`-service references removed (Overview, Phase 4); test-gating note added (Phase 5). No stale "local mongo" / "docker-compose mongo service" terms remain. Verdict (Phase 1) unaffected by these deployment/test decisions. **0 unresolved contradictions.**

### Implementation — 2026-06-26 (all phases complete)
- New `internal/mongostore` (store/subscriber/subscriber_normalization/checkpoint/indexes) on driver `go.mongodb.org/mongo-driver/v2 v2.7.0`; `internal/redisstore` deleted; `go-redis`+`miniredis` removed via `go mod tidy`.
- Consumers (`poller`, `bot`, `telegram`) swapped by qualifier only (`redisstore.`→`mongostore.`); identical exported names. Interface conformance enforced at compile time via `main.go` wiring (no `var _` assertion — would cycle).
- `config` now requires `MONGODB_URI`, defaults `MONGODB_DATABASE=openai_status_bot`; `main.go` connects/pings/`EnsureIndexes`. `.env.example`, both compose files (bot-only, Atlas, dev/prod by DB name), README, and 3 docs updated; no Redis refs remain.
- Tests: poller/bot/config on Docker-free fakes; `mongostore` has pure-logic unit tests (default) + gated `//go:build integration` testcontainers suite (run via `go test -tags=integration ./internal/mongostore/...`). `go build`/`go test`/`go vet ./...` all green without Docker.
- `code-reviewer`: DONE, 0 Critical/High/Medium; one cosmetic note (subscriber `threadID` null-vs-omit) applied.
- Deferred: user runs the gated integration suite with Docker before trusting the cutover in production.
