---
phase: 1
title: "Suitability & Decision"
status: complete
effort: "research-only (no code)"
---

# Phase 1: Suitability & Decision

## Overview

Decide whether MongoDB is the right datastore for this bot before writing code. This phase is the answer to "is it suitable for our case." No code; output is the recorded verdict below.

## Workload facts (from codebase)

- State is tiny: one set of subscribers (count = number of Telegram chats), per-subscriber settings, ~dozens of component statuses, a handful of pending component events, incident-update version markers, one `initialized` flag, one telegram offset, and short-lived per-event delivery markers.
- Access pattern: one poll/minute (default `POLL_INTERVAL=1m`) + occasional command handling. No hot path, no high QPS, no large values.
- Already abstracted behind `poller.Store` / `bot.Store` interfaces (`internal/poller/poller.go:17`, `internal/bot/bot.go:25`).
- Redis-specific mechanisms in use: a Lua script for atomic check-then-set (`internal/redisstore/subscriber_settings.go:16`), `EXPIRE` for 7-day delivery dedup TTL (`internal/redisstore/checkpoint.go:137`), and `TxPipeline` for multi-key atomicity (RemoveSubscriber, MarkDelivered, MarkIncidentUpdateVersion).

## Redis → MongoDB capability mapping

| Redis usage | MongoDB equivalent | Notes |
|---|---|---|
| `subscribers` SET + `subscriber-settings` HASH | single `subscribers` collection, one doc per subscriber (`_id` = subscriber key, `types`/`components` fields) | **Collapses two keys into one doc**; eliminates the SRem+HDel `TxPipeline` in RemoveSubscriber |
| Lua check-then-set on settings | `UpdateOne(filter{_id}, $set)`, read `MatchedCount` | `MatchedCount==0` ⇒ not subscribed; replaces the Lua script |
| `component-statuses` HASH | `component_statuses` collection, doc per component | upsert via `UpdateOne(SetUpsert)` |
| `pending-component-events` HASH (JSON) | `pending_component_events` collection, doc per component | native BSON fields, no manual JSON marshal needed |
| `incident-update-versions` HASH + legacy `incident-updates` SET | `incident_update_versions` collection, doc per updateID | **legacy SET + migration fallback (`checkpoint.go:89-108`) dropped** — fresh DB, no legacy data |
| `event-delivery:<sha>` SET + `EXPIRE 7d` | `delivery` collection, doc per (eventKey, subscriber) + **TTL index** on `expiresAt` | `MarkDelivered` becomes one upsert (no SAdd+Expire pair); sha256 key-hashing dropped (Mongo handles arbitrary string values) |
| `initialized` flag, `telegram-offset` | `meta` collection, doc per key | trivial FindOne/upsert |

All driver specifics confirmed against MongoDB Go driver **v2** (`go.mongodb.org/mongo-driver/v2`, latest v2.7.0, 2026): `ApplyURI` parses `mongodb+srv://` Atlas URIs; TTL via `IndexModel` + `SetExpireAfterSeconds`; `UpdateOne`/`MatchedCount` and `SetUpsert(true)` cover conditional set and upsert.

## Verdict

**Suitable: YES. Justified: YES** (given the stated motivation — team already runs MongoDB, so dropping Redis consolidates ops onto one datastore).

Functionally, MongoDB does everything Redis does here, and the document model **simplifies** the code: three multi-key `TxPipeline` sequences collapse into single-document operations, the Lua script disappears, and the legacy incident-dedup migration path is deleted (no data to migrate). Latency and TTL-sweep granularity (~60s) are irrelevant at one poll/minute with a 7-day dedup window.

### Trade-offs to accept (be honest)

1. **Testing regression (the real cost).** `miniredis` is pure-Go, in-memory, zero-binary, instant. MongoDB has **no equivalent** — every option runs a real `mongod`: `testcontainers-go` (needs Docker in CI, ~2-3s/suite) or `memongo` (downloads a ~100MB `mongod` binary, UNIX-only, primary repo stale — prefer the `tryvium-travels/memongo` fork). CI gains a Docker/binary dependency and slows down. This is a genuine downgrade in test ergonomics and the main argument *against* switching absent the ops motivation.
2. Heavier runtime dependency and image than the tiny `go-redis` client (acceptable; not a hot path).
3. Loss of Redis's trivial single-process local dev (`redis:7-alpine`); replaced by a **required Atlas connection** (no local DB — both dev and prod use Atlas, separated by database name).

### When NOT to switch
If the only driver were "exploring" with no infra reason, staying on Redis would be the KISS choice — Redis is the better technical fit for this exact workload. The switch is recommended **only because** MongoDB is already operated and consolidation has real ops value.

## Implementation Steps

1. Confirm the motivation still holds (already-run MongoDB / managed external host) before committing to Phases 2-5.
2. Accept the testing trade-off. **Decided (Session 1):** testcontainers-go, gated behind `//go:build integration` and run manually later — default `go test` stays Docker-free (Phase 5).
3. Proceed to Phase 2.

## Success Criteria

- [x] Verdict reviewed and motivation reconfirmed (already-run MongoDB / Atlas).
- [x] Test backend decided: testcontainers, gated + deferred (Session 1).
- [x] Testing trade-off accepted.

## Risk Assessment

- Risk: team underestimates CI impact of losing `miniredis`. Mitigation: Phase 5 makes the test backend explicit and a gating decision here.
- Risk: managed MongoDB connectivity/auth from the bot's deploy environment. Mitigation: validate `MONGODB_URI` reachability early (Phase 2 ping on startup, same as current Redis ping in `main.go:38`).
