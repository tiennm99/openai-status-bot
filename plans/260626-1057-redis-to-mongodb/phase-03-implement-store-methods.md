---
phase: 3
title: "Implement Store Methods"
status: complete
effort: "M"
---

# Phase 3: Implement Store Methods

## Overview

Implement every method required by `poller.Store` and `bot.Store` against MongoDB, matching the existing semantics exactly (return values, "exists" booleans, defaults, self-heal-on-corrupt behavior where applicable).

## Requirements

Satisfy the full interface surface (from `internal/poller/poller.go:17` and `internal/bot/bot.go:25`):

Subscribers/settings: `AddSubscriber`, `RemoveSubscriber`, `ListSubscribers`, `GetSubscriber`, `UpdateSubscriberTypes`, `UpdateSubscriberComponents`, `UpdateSubscriberSettings`.
Checkpoint/state: `ComponentStatuses`, `SaveComponentStatus`, `PendingComponentEvents`, `SavePendingComponentEvent`, `RemovePendingComponentEvent`, `HasIncidentUpdateVersion`, `MarkIncidentUpdateVersion`, `DeliveredSubscribers`, `MarkDelivered`, `ClearDelivery`, `IsInitialized`, `SetInitialized`, `TelegramOffset`, `SaveTelegramOffset`.

## Architecture / semantics mapping

- `PendingComponentEvent` struct moves to `mongostore` with BSON tags; stored as native fields (drop the manual `json.Marshal`/`Unmarshal` from `checkpoint.go:58-75`).
- **AddSubscriber**: `UpdateOne({_id:key}, {$set:{chatID,threadID}, $setOnInsert:{types:defaults, components:[]}}, upsert)`. Preserves existing settings on re-`/start` (matches current load-then-save behavior at `subscriber.go:73-83`).
- **RemoveSubscriber**: `DeleteOne({_id:key})` — single op (was SRem+HDel TxPipeline).
- **ListSubscribers**: `Find({})` + `cursor.All`. Decode straight into `Subscriber` (was SMEMBERS + HGETALL); keep the malformed-key self-heal: on decode/parse failure for a doc, `DeleteOne` it and continue or error (preserve current behavior at `subscriber.go:108-125`).
- **GetSubscriber**: `FindOne({_id:key})`; `ErrNoDocuments` ⇒ `(zero,false,nil)`.
- **UpdateSubscriberTypes/Components/Settings**: `UpdateOne({_id:key}, {$set:{...}})`; `MatchedCount==0` ⇒ `(false,nil)` (replaces the Lua check-then-set). Apply `normalizeTypes`/`normalizeComponents` before write, same as today.
- **ComponentStatuses**: `Find({})` → `map[string]string{_id: status}`.
- **SaveComponentStatus**: `UpdateOne({_id}, {$set:{status}}, upsert)`.
- **PendingComponentEvents**: `Find({})` → `map[string]PendingComponentEvent`.
- **SavePendingComponentEvent / RemovePendingComponentEvent**: upsert / `DeleteOne` by `_id=componentID`.
- **HasIncidentUpdateVersion**: `FindOne({_id:updateID})`; compare stored `version`. **Drop the legacy `incident-updates` SET fallback** (`checkpoint.go:97-106`) — no legacy data on a fresh DB.
- **MarkIncidentUpdateVersion**: `UpdateOne({_id:updateID}, {$set:{version}}, upsert)` — single op (was HSet+SAdd TxPipeline).
- **DeliveredSubscribers**: `Find({eventKey})` → `map[string]bool{subscriber:true}`.
- **MarkDelivered**: `UpdateOne({_id: eventKey+"|"+subscriber}, {$set:{eventKey,subscriber,expiresAt: now+7d}}, upsert)` — single op with TTL (was SAdd+Expire TxPipeline). `now` from `time.Now()`.
- **ClearDelivery**: `DeleteMany({eventKey})`. Drop sha256 key-hashing (`deliveryStateKey`, `checkpoint.go:168`).
- **IsInitialized / SetInitialized**: `meta` doc `_id:"initialized"` — `FindOne` exists / upsert `{value:true}`.
- **TelegramOffset / SaveTelegramOffset**: `meta` doc `_id:"telegramOffset"` — `FindOne` (default 0 on `ErrNoDocuments`, and clear-on-invalid like `checkpoint.go:155-160` is no longer needed since BSON stores an int natively) / upsert `{value:offset}`.

## Related Code Files

- Create: `internal/mongostore/subscriber.go` (extend Phase 2 file with the subscriber CRUD methods).
- Create: `internal/mongostore/subscriber_settings.go` — `subscriberSettings` decode/normalize helpers (drop the Lua script; keep normalization), or fold into subscriber.go if small (KISS).
- Create: `internal/mongostore/checkpoint.go` — `PendingComponentEvent` (BSON tags) + all checkpoint/delivery/meta/component methods.
- Reference: `internal/poller/poller.go:17`, `internal/bot/bot.go:25` (interface contracts to satisfy).

## Implementation Steps

1. Add `PendingComponentEvent` with BSON tags to `checkpoint.go`.
2. Implement subscriber CRUD + settings methods; verify against `bot.Store` signatures.
3. Implement checkpoint/delivery/meta/component methods; verify against `poller.Store` signatures.
4. Add a compile-time assertion file or inline `var _ poller.Store = (*Store)(nil)` / `var _ bot.Store = (*Store)(nil)` (only after Phase 5 imports align — or assert against locally redeclared interfaces during this phase) to catch signature drift.
5. `go build ./internal/mongostore/...`.

## Success Criteria

- [ ] All listed methods implemented with matching signatures and semantics.
- [ ] No `TxPipeline`/Lua equivalents needed — each former multi-key op is a single document op.
- [ ] Legacy incident-dedup fallback and sha256 delivery-key hashing removed.
- [ ] Package compiles.

## Risk Assessment

- Risk: `MatchedCount` vs `ModifiedCount` confusion in conditional updates. Mitigation: use `MatchedCount` for "did the subscriber exist" (a no-op `$set` to identical values still matches but may not modify).
- Risk: telegram offset stored as BSON int32 vs int64. Mitigation: store/read as `int64` explicitly in BSON.
- Risk: decode of self-healed malformed subscriber docs. Mitigation: replicate current delete-and-continue/error path; cover in Phase 5 tests.
