---
phase: 4
title: "Config Wiring & Deployment"
status: complete
effort: "S"
---

# Phase 4: Config Wiring & Deployment

## Overview

Replace Redis configuration, connection bootstrap, and deployment artifacts with MongoDB equivalents. After this phase `main.go` builds the Mongo client; consumer type swap is Phase 5.

<!-- Updated: Validation Session 1 - Atlas for both envs; one cluster, two DBs selected by MONGODB_DATABASE; no local mongo service -->

## Requirements

- New env: `MONGODB_URI` (Atlas `mongodb+srv://...`, **required**, no localhost default — there is no local DB), `MONGODB_DATABASE` (default `openai_status_bot`; dev env sets a separate name e.g. `openai_status_bot_dev`). Remove `REDIS_URL`.
- `main.go` connects, pings, ensures indexes, constructs `mongostore.New(client, dbName)`.
- Deployment (validated): **Atlas for both prod and dev** — single cluster, two databases differentiated by `MONGODB_DATABASE`. **No local `mongo` service** in any compose file.

## Architecture

`config.Config`: replace `RedisOptions *redis.Options` with `MongoURI string` + `MongoDatabase string`. Drop `parseRedisURL`, `minRedisDB`/`maxRedisDB`. Validation: `MONGODB_URI` **required** (error if empty, like `TELEGRAM_BOT_TOKEN` at `config.go:35-38` — no localhost fallback since both envs use Atlas); `MONGODB_DATABASE` defaults to `openai_status_bot`. Keep validation minimal; the driver validates the URI on connect/ping.

## Related Code Files

- Modify: `internal/config/config.go` — swap Redis fields/parsing for Mongo fields; keep `getEnv` default pattern.
- Modify: `internal/config/config_test.go` — replace Redis URL cases with Mongo URI/DB cases.
- Modify: `cmd/openai-status-bot/main.go` — replace `redis.NewClient`+`Ping` (`main.go:37-42`) with `mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))`, `client.Ping`, `defer client.Disconnect(ctx)`; call `store.EnsureIndexes(ctx)`; `store := mongostore.New(client, cfg.MongoDatabase)`. Update the connect-error log fields (drop redis-specific `Network/Addr/DB/TLS`).
- Modify: `.env.example` — `MONGODB_URI=mongodb+srv://<user>:<pass>@<cluster>/` (placeholder), `MONGODB_DATABASE=openai_status_bot`; remove `REDIS_URL`.
- Modify: `docker-compose.yml` — **remove the `redis` service and `redis-data` volume entirely** (no local DB). Becomes bot-only, connecting to Atlas via `.env`; this is the **development** path, so set `MONGODB_DATABASE` to the dev database (e.g. `openai_status_bot_dev`). No `depends_on`.
- Modify: `docker-compose.bot.yml` — bot-only, connects to Atlas via `.env` with the **production** `MONGODB_DATABASE` (`openai_status_bot`). With no local DB, this and `docker-compose.yml` differ only by database name; keep both for the dev/prod split. Optionally collapse to one file — but keeping both matches the existing two-file convention (YAGNI: don't restructure beyond the DB-name change).
- Modify: `README.md` — Configuration table (`MONGODB_URI`, `MONGODB_DATABASE` rows; drop `REDIS_URL` and the percent-encoding note, or rewrite the note for Atlas SRV URIs), feature bullet "Uses Redis for…" → MongoDB, Compose/Quick-Start description (Atlas, no local DB; dev vs prod database), intro line. Note that no local datastore runs in Compose — an Atlas URI is required to start.

## Implementation Steps

1. Edit `config.go` + `config_test.go`; `go test ./internal/config/`.
2. Edit `main.go` for Mongo client lifecycle + `EnsureIndexes`. (Will not fully build until Phase 5 swaps `redisstore`→`mongostore` in poller/bot — acceptable; Phases 4-5 land together.)
3. Update `.env.example`, both compose files, README.

## Success Criteria

- [ ] `config` package + tests pass with Mongo env vars; no Redis symbols remain in config.
- [ ] `main.go` opens/pings Mongo, ensures indexes, builds `mongostore`.
- [ ] Compose, `.env.example`, README reference only MongoDB.

## Risk Assessment

- Risk: managed URIs carry credentials/`+srv`. Mitigation: `ApplyURI` handles `mongodb+srv://`; never log the URI (log db name only); keep `.env` out of git (already in `.gitignore`).
- Risk: dropping `REDIS_URL` breaks existing deploys silently. Mitigation: README "Notes" + this plan flag the env rename; no compatibility shim (clean cutover, intended).
