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
make sync            # Daily catalog job: changes (last completed window → today), refresh, hydrate, purge, stats
make sync START=2026-01-01 END=2026-01-05
make seed                    # bin/seed: catalog seed CLI (download, save, hydrate, update, status; --help); local only
```

Single package / single test:

```sh
go test ./internal/store/...
go test ./internal/api/handlers/ -run TestCreateWatchEvent -v
```

**Docker is required** for `internal/store`, `internal/catalog`, and `internal/achievements` tests. Each of those packages has a `TestMain` that boots a `pgvector/pgvector:0.8.6-pg18` testcontainer (the same image as `local-development/docker-compose.yml`) seeded with `local-development/init.sql`; the catalog tests add an `httptest` TMDB server (`fakeTMDB`). Handler, middleware, age, background, and auth tests use in-memory fakes and run without Docker. Run the packages with goroutines under `-race`.

Local Postgres is 18 with pgvector. The 18 image moved PGDATA, so the compose volume mounts at `/var/lib/postgresql`; a volume created by an older image cannot be reused and must be removed with `docker compose -f local-development/docker-compose.yml down -v` before `make local-up`.

After Go changes run `make fmt`, `make lint`, and `go mod tidy` if imports changed. `cmd/guest-jwt-token/` is an empty placeholder directory.

## Configuration

All config is read from env vars in `config/config.go` via `config.Load()`, one line per variable through the `env[T]` helper (`strconv.Atoi`, `ParseFloat`, `ParseBool`, base64 as the parse funcs); every parse error is reported at once. Copy `env.example` to `.env`; the Makefile sources it. `config.Load()` itself only requires `TMDB_ACCESS_TOKEN` and `DATABASE_URL` (enough for the sync job and the seed). The API server additionally enforces with `cfg.ValidateAPI()`: `MINI_MOVIE_UI_SECRET`, `SESSION_SECRET` (base64, ≥32 bytes; first 16 bytes become the OIDC cookie hash key, next 16 the encryption key), `TOKEN_ENCRYPTION_KEY` (base64, exactly 32 bytes), at least one OAuth provider, and `AUTH_BASE_URL` / `AUTH_UI_BASE_URL` (HTTPS when `ENV=production`). `ANTHROPIC_API_KEY` empty means the augur resolver is `nil` and the interesting-info endpoint is disabled.

## Architecture

### Wiring

`cmd/api/main.go` is the composition root. It builds the pgx pool, every store, the TMDB client, one `background.Group`, the `catalog.Service` (and starts its people fetcher), the augur resolver, the OIDC provider registry, and the achievement worker, then passes everything into `handlers.NewHandlers(handlers.HandlerDeps{...})`. All HTTP handlers are methods on the single `handlers.Handlers` struct. `api.NewRouter` mounts them. `var _ handlers.Catalog = (*catalog.Service)(nil)` pins the contract. `serve` runs an `http.Server` (`ReadHeaderTimeout` 5 s, `ReadTimeout` 10 s, `IdleTimeout` 60 s, no `WriteTimeout` because the interesting-info route runs up to `AUGUR_TIMEOUT`) until SIGINT or SIGTERM, then drains in order: `Shutdown`, `bg.Wait` (30 s), the achievement worker and the people fetcher, and finally the deferred pool close. Both workers stop by cancelling a context and leave their queue open, so a late enqueue never panics.

Handler dependencies are declared as interfaces where tests need to swap them: `store.*Repository` (one per store file), `handlers.Catalog` (declared in `handlers.go`, deliberately wide: one struct consumes all of it), `tmdb.MediaClient` (search only), `auth.ProviderRegistry`. `internal/api/handlers/testutil_test.go` holds the fake implementations and `newTestHandlers(t)`; `fakeCatalog` embeds `Catalog`, so add a method to it when you add one to the interface. `*augur.Resolver` is a concrete pointer left `nil` in handler tests, so its handler nil-checks it. Detached work goes through the injected `*background.Group`; there is no bare `go func` in a request path.

### Router auth layers (`internal/api/router.go`)

1. **UI JWT** (`Authorization: Bearer`, HS256 signed with `MINI_MOVIE_UI_SECRET`): public catalog `/search`, `/movies`, `/series`, `/people`, with `TIMEOUT` seconds; `/interesting/person/{id}` gets `AUGUR_TIMEOUT` instead. `POST /auth/token` also uses this JWT.
2. **Session cookie** (`RequireSession` + `RequireOrigin` + `NoStoreCache`): `/auth/session`, `/auth/logout`, and everything under `/users/me`. `RequireOrigin` rejects non-GET requests whose `Origin` is not in `allowedUIOrigins`; that list is hardcoded in `router.go`. The middleware puts `*store.User` and the session hash into the request context; handlers read them with `getUserFromContext`.
3. **Unauthenticated**: `/auth/{provider}`, `/auth/{provider}/callback` (GET and POST), `/auth/apple/notifications`, `/ping`.

Cookie name is `__Secure-mm_session` in production and `mm_session_dev` otherwise.

### Login flow

`GET /auth/{provider}` uses zitadel `rp.AuthURLHandler` (PKCE, encrypted state cookie). The callback upserts the user by `(provider, subject)`, creates a session row storing only the HMAC hash of a random token, creates a one-time **auth code** that holds the raw session token encrypted with `TOKEN_ENCRYPTION_KEY`, and redirects to `AUTH_UI_BASE_URL/auth/finish?code=...`. The UI then calls `POST /auth/token` with the code and sets the cookie itself. Apple uses `response_mode=form_post`, so its callback is a cross-site POST and its state cookie is `SameSite=None`; Apple also only sends the user's name on first authorization in the form body. Apple server-to-server notifications (`/auth/apple/notifications`) are deduplicated by JWT `jti` in `provider_notifications_seen`.

### Catalog tables and the seed

TMDB data is being moved into Postgres (`movies`, `series`, `seasons`, `episodes`, `collections`, `people`; see `local-development/init.sql`). Every catalog row has an identity `id` of our own, the provider's id in `source_id` (the only column that knows a provider exists; everything that talks to TMDB resolves through it), and a `slug` generated by the database from id and title (`155-the-dark-knight`, resolved by the leading id via `catalog.ParseSlugID`). Typed columns are projections of the payload written in the same upsert; `payload` is the pruned TMDB document (US watch providers only); `payload is null` means a skeleton or list-grade row that has not been hydrated. `popularity` on a hydrated row is the API's value and only a hydration changes it; skeleton rows carry the export's or a credit list's value. Lifecycle: `stale` (set by the changes feed) and `fetched_at` (written only by a hydration; null on skeleton rows, which the 180-day purge therefore never touches), with `store.RefreshAfter` (150 days) and `store.ExpireAfter` (180 days) enforcing TMDB's six-month cap. Nothing derived from another row is stored on a media row: ages are computed at read time from `people`, and reshaped views of a row's own payload (a season's runtime, a person's episodes in a season) are computed from the document in Go.

`internal/catalog` is the only path to TMDB. `getMovie`/`getSeries`/`getSeason`/`getEpisode`/`getPerson` (`fetch.go`) fetch, prune, and map (`payload.go`) a document into a `writes` value: `write` upserts the row and its own children (season skeletons for a series, episode skeletons for a season, the collection for a movie) and `seed` writes the list-grade rows of other entities (credited people, a person's filmography, a collection's parts). `fetchPriorityPeople` then hydrates up to `PeoplePerHydration` of the director, writers, and top-10 cast that are still unhydrated (the rest of the cast and crew stay at list grade). `Hydrate` claims rows by work class (`store.WorkStale`, `WorkExpiring`, `WorkUnhydrated`) in 25-row transactions, five batches at a time, never reclaiming a row that already failed in the run: each batch's documents are fetched concurrently and written with a savepoint per row; list-grade rows of other entities and the credited-people fetch follow the commit so concurrent batches cannot deadlock over shared rows. `SeedExports` streams a gzipped id export into skeleton rows. `SyncChanges` reads a changes feed and flags the rows we hold (a changed series also flags its hydrated seasons and episodes); `ChangeWindow` is the window rule (from the last completed `sync_job_status` row of the entity's `SyncJobType`, or a day before the oldest hydrated row, to today, never starting past today); `seed update` and `cmd/sync` both use them. Hydrating a series writes skeleton season rows from its document and hydrating a season writes skeleton episode rows, so anything the UI can reference has a row. `cmd/seed` (`download`, `save`, `hydrate`, `update`, `status`; entities are spelled like their tables: `movies`, `series`, `people`) drives both from a laptop with a status line, a JSON log file, and a per-kind state file for `--resume`; it is never deployed. `tmdb.Client` paces every process through `TMDB_RATE_LIMIT`, holds at most 20 requests open at once (TMDB caps connections per IP as well as rate), retries 429/5xx and transport errors up to three times, and pauses every caller for the backoff when TMDB answers 429.

### Read path (`internal/catalog/read.go`, `service.go`)

Every catalog handler reads through `handlers.Catalog`: `Movie`, `Collection`, `Series`, `Season`, `Seasons`, `Episode`, `Person`, `SeedSearch`, and the four `Resolve*` snapshots. Routes accept an id or a slug (`catalog.ParseSlugID`), and every id in a response is ours: `applyPeople` (`credits.go`) maps credited people through `catalog.PeopleDates`, and seasons, episodes, collection parts, filmography, and search results go through the `IDs` maps. **An entry without a catalog row is dropped from the response** (a provider id must never leak, it would name a different row); it appears on the next read once the background seed has written it.

`ensure` is the state machine, once, for the five row types: **fresh** (payload present, within `store.ExpireAfter`, not stale) is served; **stale** is served as stored and one refresh runs on `background.Group` under the singleflight key, so concurrent reads share it; **miss** (no payload, or past `ExpireAfter`) runs the fetch inside the singleflight on a detached 10 s context, writes the row and its children before returning, and hands the seed to the background group. A 404 deletes the row and surfaces as `catalog.ErrNotFound`. `Season` and `Episode` bring the series (and the season) up first, so a deep link into a never-viewed series works. `Seasons` fetches misses five at a time and drops a season it cannot load.

**People**: every accessor ends with `peopleDates`: one `GetDates` query for the credited ids; gaps among the director, writers, and top-10 cast are fetched synchronously up to `MAX_TMDB_FETCH_PER_REQUEST` (through the singleflighted `fetchPerson`, shared with the hydrator); every other gap goes to the in-process **people fetcher** (`Start`/`Stop`, four workers, a 1024 queue that drops when full; ids still queued at `Stop` are rediscovered on the next load). A hydrated person with no birthday is a known blank, never refetched.

**Augur** (`internal/augur/`) caches the full LLM JSON result in `people.insights`, keyed by our person id, through a two-method `insightsStore` interface `PersonStore` satisfies; fields below `AUGUR_MIN_CONFIDENCE` are dropped at read time, not at write time. Also uses singleflight.

### Watch events (the most cross-cutting flow)

`POST /users/me/watch-events` returns **202 immediately** and does the work on the `background.Group` with a 30 s timeout; the task returns its first error and the group logs it. IDs are deterministic UUIDv5 values derived from `user + mediaType + mediaId` (and `watchlist- + user + type + id` for the watchlist row), so the client gets stable IDs before the rows exist and retries are idempotent upserts. The background path: resolve the title's snapshot via `catalog.Resolve*` (from the stored document; a miss fetches it) → write `watch_event` (`MarkSeason` transactionally deletes that season's per-episode rows so aggregates never double count) → auto-create a `watchlist_item` with status `watched` if none exists → `WatchlistRepository.UpdateSummary` recomputes the watchlist row's counts, status (`want_to_watch` / `in_progress` / `watched`), and timestamps from `watch_event` and the `series` row's totals and `payload->'seasons'` → enqueue the achievement worker. `watchlist_item` holds only the user's progress and a fallback title; `List` joins `movies` and `series` for poster, genres, runtime, rating, year, and series totals. Series-level watch events are rejected; episode and season events roll up to the parent series watchlist item via `getWatchlistTarget`.

`GET /users/me/media-state` returns the consolidated watchlist + watch-event state for one media item so the UI can make a single call.

### Achievements

`achievements.Worker` is a buffered channel (1024) drained by 8 goroutines; `Enqueue` drops on a full queue. Each job re-checks every entry in `AllDefinitions` (`definitions.go`) that the user has not already earned, using the `Check` func in `checkers.go`. To add an achievement, append a `Definition` and a checker; idempotency comes from the `Exists` check plus the `(user_id, achievement_id)` unique constraint.

### The daily job and expiry

`cmd/sync` is the one daily job: changes per entity (`movie_sync`, `tv_sync`, `person_sync` rows in `sync_job_status`, window from `catalog.ChangeWindow`), refresh of stale and expiring rows, `SYNC_HYDRATE_BUDGET` new rows (movies, then series, then people), purge, and stats. A failed step is logged and the run continues; the exit code is 1 if anything failed. Any store with expiring rows implements `store.Purgeable` (`DeleteExpired`, `TableName`) and must be added to the `purgeable` list in `cmd/sync/main.go`. Production is not touched until the cutover after Phase 2b; at the cutover the Railway cleanup cron goes away.

### Conventions worth knowing

- **Schema** lives only in `local-development/init.sql`. There is no migration tool. When adding a table, also add it to `truncateAll` in `internal/store/testhelper_test.go` and `internal/achievements/checkers_test.go`.
- **Responses**: `httputil.JSON(w, status, data)` sets `Cache-Control: public, max-age=CACHE_MAX_AGE` by default. Pass `0` as the trailing arg for anything user-specific or mutating. `httputil.Error` always passes `0`.
- **Metrics**: `metrics.M` is a package global that records into the SDK's no-op provider until `metrics.Init` installs the exporter, so it is never nil and never guarded. Stores time queries with `defer metrics.TrackDbDuration(ctx, "store.op")()`.
- **Response types** for the catalog (`Person`, `Credits`, `MovieDetails`, etc.) live in `internal/api/handlers`, not in `internal/tmdb`. `credits.go` maps TMDB crew job titles into role buckets; `catalog/people.go` assigns the fetch priorities (director 1, writer 2, top cast 3, cast 4, crew 5).
- **Errors**: `catalog.ErrNotFound` is what handlers map to 404; the TMDB client's `tmdb.ErrNotFound`, `ErrRateLimited`, `ErrServerError` stay inside `catalog`.
- **Schema**: `store` never sees a `tmdb` type; `catalog` maps between them in `payload.go`.
- `local-development/api-examples.md` has curl examples for every endpoint, and `openapi/minimovie-api.yaml` is the API spec; update the spec when routes or response shapes change.
