# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make local-up        # Start local Postgres (Docker Compose, seeds from local-development/init.sql)
make local-down
make watch           # Run API with live reload (air); sources .env
make start           # Run API without live reload; sources .env
make build           # go build -o bin/api
make fmt             # go fmt ./...
make lint            # golangci-lint run ./...
make test            # go test ./...
make test-cover      # coverage report in browser
make sync            # Person change sync job (last successful end date → today)
make sync START=2026-01-01 END=2026-01-05
go run cmd/cleanup/main.go   # Expired-row purge job (no Makefile target)
make seed                    # bin/seed: catalog seed CLI (download, save, hydrate, update, status; --help); local only
```

Single package / single test:

```sh
go test ./internal/store/...
go test ./internal/api/handlers/ -run TestCreateWatchEvent -v
```

**Docker is required** for `internal/store` and `internal/achievements` tests. Each of those packages has a `TestMain` that boots a `pgvector/pgvector:0.8.6-pg18` testcontainer (the same image as `local-development/docker-compose.yml`) seeded with `local-development/init.sql`. Handler, middleware, age, and auth tests use in-memory fakes and run without Docker.

Local Postgres is 18 with pgvector. The 18 image moved PGDATA, so the compose volume mounts at `/var/lib/postgresql`; a volume created by an older image cannot be reused and must be removed with `docker compose -f local-development/docker-compose.yml down -v` before `make local-up`.

After Go changes run `make fmt`, `make lint`, and `go mod tidy` if imports changed. `cmd/guest-jwt-token/` is an empty placeholder directory.

## Configuration

All config is read from env vars in `config/config.go` via `config.Load()`. Copy `env.example` to `.env`; the Makefile sources it. `config.Load()` itself only requires `TMDB_ACCESS_TOKEN` and `DATABASE_URL` (enough for the sync and cleanup jobs). The API server additionally enforces in `validateConfig` (`cmd/api/main.go`): `MINI_MOVIE_UI_SECRET`, `SESSION_SECRET` (base64, ≥32 bytes; first 16 bytes become the OIDC cookie hash key, next 16 the encryption key), `TOKEN_ENCRYPTION_KEY` (base64, exactly 32 bytes), at least one OAuth provider, and `AUTH_BASE_URL` / `AUTH_UI_BASE_URL` (HTTPS when `ENV=production`). `ANTHROPIC_API_KEY` empty means the augur resolver is `nil` and the interesting-info endpoint is disabled.

## Architecture

### Wiring

`cmd/api/main.go` is the composition root. It builds the pgx pool, every store, the BigCache adapters, the TMDB client, the resolvers, the OIDC provider registry, and the achievement worker, then passes everything into `handlers.NewHandlers(handlers.HandlerDeps{...})`. All HTTP handlers are methods on the single `handlers.Handlers` struct. `api.NewRouter` mounts them.

Handler dependencies are declared as interfaces where tests need to swap them: `store.*Repository` (one per store file), `tmdb.MediaClient`, `auth.ProviderRegistry`, `store.SeasonCastCache`. `internal/api/handlers/testutil_test.go` holds the fake implementations and `newTestHandlers(t)`; add a method to the fake whenever you add one to the interface. A few deps are concrete pointers (`*age.Resolver`, `*series.Service`, `*augur.Resolver`, `*store.SeriesMetadataStore`) and are left `nil` in handler tests, so handlers must nil-check them.

### Router auth layers (`internal/api/router.go`)

1. **UI JWT** (`Authorization: Bearer`, HS256 signed with `MINI_MOVIE_UI_SECRET`): public catalog `/search`, `/movies`, `/series`, `/people`, with `TIMEOUT` seconds; `/interesting/person/{id}` gets `AUGUR_TIMEOUT` instead. `POST /auth/token` also uses this JWT.
2. **Session cookie** (`RequireSession` + `RequireOrigin` + `NoStoreCache`): `/auth/session`, `/auth/logout`, and everything under `/users/me`. `RequireOrigin` rejects non-GET requests whose `Origin` is not in `allowedUIOrigins`; that list is hardcoded in `router.go`. The middleware puts `*store.User` and the session hash into the request context; handlers read them with `getUserFromContext`.
3. **Unauthenticated**: `/auth/{provider}`, `/auth/{provider}/callback` (GET and POST), `/auth/apple/notifications`, `/ping`.

Cookie name is `__Secure-mm_session` in production and `mm_session_dev` otherwise.

### Login flow

`GET /auth/{provider}` uses zitadel `rp.AuthURLHandler` (PKCE, encrypted state cookie). The callback upserts the user by `(provider, subject)`, creates a session row storing only the HMAC hash of a random token, creates a one-time **auth code** that holds the raw session token encrypted with `TOKEN_ENCRYPTION_KEY`, and redirects to `AUTH_UI_BASE_URL/auth/finish?code=...`. The UI then calls `POST /auth/token` with the code and sets the cookie itself. Apple uses `response_mode=form_post`, so its callback is a cross-site POST and its state cookie is `SameSite=None`; Apple also only sends the user's name on first authorization in the form body. Apple server-to-server notifications (`/auth/apple/notifications`) are deduplicated by JWT `jti` in `provider_notifications_seen`.

### Catalog tables and the seed

TMDB data is being moved into Postgres (`movies`, `series`, `seasons`, `episodes`, `collections`, `people`; see `local-development/init.sql`). Every catalog row has an identity `id` of our own, the provider's id in `source_id` (the only column that knows a provider exists; everything that talks to TMDB resolves through it), and a `slug` generated by the database from id and title (`155-the-dark-knight`, resolved by the leading id via `catalog.ParseSlugID`). Typed columns are projections of the payload written in the same upsert; `payload` is the pruned TMDB document (US watch providers only); `payload is null` means a skeleton or list-grade row that has not been hydrated. `popularity` on a hydrated row is the API's value and only a hydration changes it; skeleton rows carry the export's or a credit list's value. Lifecycle: `stale` (set by the changes feed) and `fetched_at` (written only by a hydration; null on skeleton rows, which the 180-day purge therefore never touches), with `store.RefreshAfter` (150 days) and `store.ExpireAfter` (180 days) enforcing TMDB's six-month cap. Nothing derived from another row is stored on a media row: ages are computed at read time from `people`, and reshaped views of a row's own payload (`SeasonStore.PersonEpisodeCounts`, `RuntimeMinutes`) are SQL over the JSON.

`internal/catalog` is the only write path: `fetchMovie`/`fetchSeries`/`fetchPerson` fetch, prune, map (`payload.go`), upsert, and seed credited people at list grade, then `fetchPriorityPeople` hydrates up to `PeoplePerHydration` of the director, writers, and top-10 cast that are still unhydrated (the rest of the cast and crew stay at list grade). `Hydrate` claims rows by work class (`store.WorkStale`, `WorkExpiring`, `WorkUnhydrated`) in 25-row transactions, five batches at a time, never reclaiming a row that already failed in the run: each batch's documents are fetched concurrently and written with a savepoint per row; list-grade rows of other entities and the credited-people fetch follow the commit so concurrent batches cannot deadlock over shared rows. `SeedExports` streams a gzipped id export into skeleton rows. `SyncChanges` reads a changes feed and flags the rows we hold; `seed update` runs it per entity from the last successful `sync_job_status` row of that entity's `SyncJobType` (or the oldest hydrated row before any) to today, then refreshes stale and expiring rows, and the daily job will call the same two functions. Hydrating a series writes skeleton season rows from its document and hydrating a season writes skeleton episode rows, so anything the UI can reference has a row. `cmd/seed` (`download`, `save`, `hydrate`, `update`, `status`; entities are spelled like their tables: `movies`, `series`, `people`) drives both from a laptop with a status line, a JSON log file, and a per-kind state file for `--resume`; it is never deployed. `tmdb.Client` paces every process through `TMDB_RATE_LIMIT`, holds at most 20 requests open at once (TMDB caps connections per IP as well as rate), retries 429/5xx and transport errors up to three times, and pauses every caller for the backoff when TMDB answers 429.

The read path (handlers) still calls TMDB directly until Phase 2a of the catalog plan; `PersonStore` keeps `GetPeople`/`UpsertPersonBatch`/`MarkPeopleStale` as shims for the current resolver until then. Store tests for the removed tables and the watchlist/achievements tests that insert removed columns are expected to fail on this branch until 2a.

### Three-tier caching pattern

The same shape appears three times: **BigCache (in-process, 24h) → Postgres → TMDB**, with TMDB results written to BigCache synchronously and persisted to Postgres in a goroutine using `context.WithoutCancel(ctx)` plus a short timeout so the write survives request cancellation.

- **Person birthdays** (`internal/age/resolver.go`): `Resolve` takes `[]PersonRef` with priorities assigned in `handlers/credits.go` (director 1, writer 2, top cast 3, cast 4, crew 5), sorts by priority, and caps TMDB fetches at `MAX_TMDB_FETCH_PER_REQUEST`. Fetches go through a `singleflight.Group` keyed by person ID. Rows with `fetched=false` are treated as misses; the sync job flips that flag when TMDB reports a change.
- **Season cast maps** (`store.SeasonCastTieredCache`): used by `GET /series/{id}/person/{id}/credits` to avoid refetching every season's aggregate credits. Postgres rows carry `expires_at`.
- **Series metadata** (`internal/series/service.go` + `series_metadata` table): episode counts per season, status, air dates. Handlers never read it directly; `WatchlistStore.List` and `UpdateSummary` join to it in SQL to derive series progress and the `in_progress` / `watched` status. `UpdateSeries` is a fire-and-forget refresh called from the series and season handlers so the row exists before a user marks progress.

**Augur** (`internal/augur/`) skips BigCache and caches the full LLM JSON result in the `interesting_info` table; fields below `AUGUR_MIN_CONFIDENCE` are dropped at read time, not at write time. Also uses singleflight.

### Watch events (the most cross-cutting flow)

`POST /users/me/watch-events` returns **202 immediately** and does the work in a goroutine with a 30s background context. IDs are deterministic UUIDv5 values derived from `user + mediaType + mediaId` (and `watchlist- + user + type + id` for the watchlist row), so the client gets stable IDs before the rows exist and retries are idempotent upserts. The background path: resolve TMDB metadata via `tmdb.MetadataResolver` → write `watch_event` (`MarkSeason` transactionally deletes that season's per-episode rows so aggregates never double count) → auto-create a `watchlist_item` with status `watched` if none exists → `WatchlistRepository.UpdateSummary` recomputes the watchlist row's counts, status (`want_to_watch` / `in_progress` / `watched`), and timestamps from `watch_event` → enqueue the achievement worker. Series-level watch events are rejected; episode and season events roll up to the parent series watchlist item via `getWatchlistTarget`.

`GET /users/me/media-state` returns the consolidated watchlist + watch-event state for one media item so the UI can make a single call.

### Achievements

`achievements.Worker` is a buffered channel (1024) drained by 8 goroutines; `Enqueue` drops on a full queue. Each job re-checks every entry in `AllDefinitions` (`definitions.go`) that the user has not already earned, using the `Check` func in `checkers.go`. To add an achievement, append a `Definition` and a checker; idempotency comes from the `Exists` check plus the `(user_id, achievement_id)` unique constraint.

### Background jobs and expiry

`cmd/sync` and `cmd/cleanup` both record runs in `sync_job_status` (types `person_sync` and `cache_cleanup`); sync uses the last successful run's `end_date` as the next start. Any store with expiring rows implements `store.Purgeable` (`DeleteExpired`, `TableName`) and must be added to the `stores` slice in `cmd/cleanup/main.go`.

### Conventions worth knowing

- **Schema** lives only in `local-development/init.sql`. There is no migration tool. When adding a table, also add it to `truncateAll` in `internal/store/testhelper_test.go` and `internal/achievements/checkers_test.go`.
- **Responses**: `httputil.JSON(w, status, data)` sets `Cache-Control: public, max-age=CACHE_MAX_AGE` by default. Pass `0` as the trailing arg for anything user-specific or mutating. `httputil.Error` always passes `0`.
- **Metrics**: `metrics.M` is a package global that is `nil` until `metrics.Init` runs. Always guard with `if metrics.M != nil`. Stores time queries with `defer metrics.TrackDbDuration(ctx, "store.op")()`.
- **Response types** for the catalog (`Person`, `Credits`, `MovieDetails`, etc.) live in `internal/api/handlers`, not in `internal/tmdb`. `credits.go` maps TMDB crew job titles into role buckets and assigns age-lookup priorities.
- **TMDB errors**: the client returns sentinel `tmdb.ErrNotFound`, `ErrRateLimited`, `ErrServerError`; handlers map `ErrNotFound` to 404.
- `local-development/api-examples.md` has curl examples for every endpoint, and `openapi/minimovie-api.yaml` is the API spec; update the spec when routes or response shapes change.
