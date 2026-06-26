---
phase: 2
title: "Store Package & Connection"
status: complete
effort: "S"
---

# Phase 2: Store Package & Connection

## Overview

Create `internal/mongostore` with the connection plumbing, collection handles, index bootstrap, and the storage-agnostic domain types/logic moved over verbatim from `redisstore`. No consumer wiring yet (Phase 4); `redisstore` still exists in parallel until Phase 5.

## Requirements

- Functional: open a MongoDB client from a URI, ping on startup, expose a `Store` holding collection handles, ensure required indexes exist idempotently.
- Non-functional: keep exported type/constant names identical to `redisstore` so Phase 5 is a qualifier swap, not a rewrite of consumers.

## Architecture

Collections (database name from config, default `openai_status_bot`):

- `subscribers` — `{_id: "<chatID>"|"<chatID:threadID>", chatID, threadID, types[], components[]}`
- `component_statuses` — `{_id: componentID, status}`
- `pending_component_events` — `{_id: componentID, componentName, status, updatedAt, position, previousStatus, deliveryKey}`
- `incident_update_versions` — `{_id: updateID, version}`
- `delivery` — `{_id: "<eventKey>|<subscriber>", eventKey, subscriber, expiresAt}` (TTL index on `expiresAt`)
- `meta` — `{_id: "initialized"|"telegramOffset", value}`

Indexes ensured on startup:
- `delivery`: TTL index `{expiresAt:1}` with `ExpireAfterSeconds(604800)` (7 days). The compound key is encoded in `_id`, so no extra unique index needed; an additional non-unique `{eventKey:1}` index speeds `DeliveredSubscribers`/`ClearDelivery` lookups.
- All other collections use `_id` only — no extra indexes.

## Related Code Files

- Create: `internal/mongostore/store.go` — `Store` struct, `New(client *mongo.Client, dbName string) *Store`, collection handles, collection-name constants (replacing the `*Key` constants in `redisstore/store.go`), `SubscriptionType*` constants.
- Create: `internal/mongostore/indexes.go` — `EnsureIndexes(ctx) error` (TTL + eventKey index).
- Create: `internal/mongostore/subscriber.go` — move `Subscriber`, `NewSubscriber`, `ParseSubscriberKey`, `Key()`, `Accepts`, `DefaultSubscriptionTypes` **verbatim** (pure logic, storage-agnostic).
- Create: `internal/mongostore/subscriber_normalization.go` — move `normalizeTypes`, `normalizeComponents`, `containsFold` **verbatim**.
- Reference (driver, Phase 4 adds to go.mod): `go.mongodb.org/mongo-driver/v2/mongo`, `.../mongo/options`, `.../bson`.

## Implementation Steps

1. `go get go.mongodb.org/mongo-driver/v2@latest` (adds to go.mod; full dep cleanup in Phase 4).
2. Write `store.go`: define collection-name constants, `Store` struct holding `*mongo.Database` + per-collection `*mongo.Collection` handles, and `New(client, dbName)`.
3. Write `subscriber.go` and `subscriber_normalization.go` by copying the storage-agnostic code from the matching `redisstore` files unchanged except the package name.
4. Write `indexes.go` with `EnsureIndexes` creating the TTL index (`options.Index().SetExpireAfterSeconds(604800)`) and the `{eventKey:1}` index via `Indexes().CreateMany`. Idempotent — re-creating an existing index is a no-op/ignored error.
5. `go build ./internal/mongostore/...` to confirm the package compiles in isolation.

## Success Criteria

- [ ] `internal/mongostore` compiles standalone.
- [ ] Exported names match `redisstore` (`Subscriber`, `PendingComponentEvent` [added Phase 3], `NewSubscriber`, `DefaultSubscriptionTypes`, `SubscriptionTypeIncident/Component`).
- [ ] `EnsureIndexes` creates the TTL index with 604800s expiry.

## Risk Assessment

- Risk: driver v2 import path differs from v1 examples. Mitigation: pin `go.mongodb.org/mongo-driver/v2`; `mongo.Connect(options.Client().ApplyURI(uri))` (no separate `ctx` arg in v2 `Connect`).
- Risk: index creation race on multi-instance deploy. Mitigation: single-instance bot; `CreateMany` is idempotent regardless.
