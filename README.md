# MiniMovie API

Simple API backend for _MiniMovie_ written in Go.

## Installation

### Install Go

Install Go (v1.25.5+) via Homebrew.

```sh
brew install go
```

> [!IMPORTANT]
> Don't forget to add your Go binary path to your PATH!

### Install Air

Run the following command to install the [Air Live Reloader](https://github.com/air-verse).

```sh
go install github.com/air-verse/air@latest
```

### Run the Application

With live reloading:

```sh
make watch
```

Without live reloading:

```sh
make start
```

## App Structure

```
minimovie-api/
├── cmd/
│   ├── api/
│   │   └── main.go                 # API server entry point
│   ├── seed/
│   │   ├── main.go                 # Catalog seed CLI (local only): download, save, hydrate, update, status
│   │   ├── root.go                 # Cobra root, --entity flag, exit codes, signal handling
│   │   ├── download.go             # Download the daily TMDB id export files
│   │   ├── save.go                 # Tier 1: skeleton rows from an export file, zero API calls
│   │   ├── hydrate.go              # Tier 2: the next --limit not-yet-hydrated rows, most popular first
│   │   ├── update.go               # Changes feed since the last update, flag, refresh
│   │   ├── status.go               # Table counts and last-run summary
│   │   ├── session.go              # Config, log file, database and TMDB client for one run
│   │   ├── state.go                # Per-entity checkpoint file (--resume)
│   │   ├── progress.go             # Per-row and per-batch callbacks: counters and the status line
│   │   └── term.go                 # In-place status line above the log output
│   └── sync/
│       └── main.go                 # Daily catalog job (cron): changes, refresh, hydrate, purge, stats
│
├── config/
│   └── config.go                   # Config definitions and loader
│
├── internal/
│   ├── achievements/
│   │   ├── checkers.go             # Achievement checkers
│   │   ├── checkers_test.go        # Integration tests
│   │   ├── definitions.go          # Achievement definitions (IDs, names, descriptions)
│   │   └── worker.go               # Background achievement worker
│   │
│   ├── age/
│   │   └── age.go                  # Age calculation utilities
│   │
│   ├── background/
│   │   └── group.go                # Detached tasks with their own timeout; waited for at shutdown
│   │
│   ├── catalog/
│   │   ├── catalog.go              # Service, Entity, Deps: the one path to TMDB data, read and write
│   │   ├── changes.go              # SyncChanges and ChangeWindow: changes feed to stale flags
│   │   ├── fetch.go                # getX: network half, then writes (own row and children, then seeds)
│   │   ├── read.go                 # The read state machine: fresh, stale (served + refreshed), miss (fetched)
│   │   ├── service.go              # Accessors handlers read through: Movie, Series, Season, Episode, Person...
│   │   ├── resolved.go             # ResolveX: the snapshot a watch event keeps
│   │   ├── people.go               # Credited people: seeding, the capped priority fetch, the fetcher
│   │   ├── payload.go              # tmdb.* to store.* row mappers (typed columns + pruned payload)
│   │   ├── slug.go                 # ParseSlugID: the leading id of a slug
│   │   ├── hydrate.go              # Batched, transactional hydration: concurrent batches, claim by work class
│   │   └── seed.go                 # Streams a gzipped id export into skeleton rows
│   │
│   ├── api/
│   │   ├── router.go               # Chi router setup, registers all routes
│   │   ├── middleware/
│   │   │   ├── auth.go             # Session auth middleware
│   │   │   └── cors.go             # CORS configuration
│   │   └── handlers/
│   │       ├── handlers.go         # Handler dependencies
│   │       ├── helpers.go          # Shared handler helpers
│   │       ├── account.go          # DeleteAccount, ExportUserData
│   │       ├── achievements.go     # Achievements list, unseen, mark seen
│   │       ├── auth.go             # BeginAuth, AuthCallback, ExchangeToken, GetSession, Logout
│   │       ├── credits.go          # Credits types and functions
│   │       ├── episode.go          # GetEpisode handler
│   │       ├── interesting.go      # Person interesting info (augur)
│   │       ├── movie.go            # GetMovie handler
│   │       ├── notifications.go    # Apple server-to-server notifications
│   │       ├── person.go           # GetPerson handler
│   │       ├── person_series.go    # Person series credits handler
│   │       ├── revocation.go       # Token revocation
│   │       ├── search.go           # SearchMulti handler
│   │       ├── season.go           # GetSeason handler
│   │       ├── series.go           # GetSeries handler
│   │       ├── stats.go            # User stats handler
│   │       ├── watch.go            # WatchProviders types and functions
│   │       ├── watch_events.go     # Watch event CRUD
│   │       └── watchlist.go        # Watchlist CRUD
│   │
│   ├── auth/
│   │   ├── oidc.go                 # OIDC provider registry (zitadel/oidc)
│   │   └── apple_secret.go         # Apple client_secret JWT generation
│   │
│   ├── httputil/
│   │   └── response.go             # JSON(w, status, data), Error(w, status, msg)
│   │
│   ├── metrics/
│   │   ├── metrics.go              # OpenTelemetry metrics
│   │   └── middleware.go           # HTTP metrics middleware
│   │
│   ├── store/
│   │   ├── pool.go                 # PostgreSQL connection pool
│   │   ├── catalog.go              # Shared catalog table ops: claim, mark stale, ids by source, purge, stats
│   │   ├── movie_store.go          # movies: hydrated and skeleton upserts, reads
│   │   ├── series_store.go         # series
│   │   ├── season_store.go         # seasons (skeletons from the series document)
│   │   ├── episode_store.go        # episodes (skeletons from the season document)
│   │   ├── collection_store.go     # collections
│   │   ├── achievement_store.go    # Achievement store
│   │   ├── auth_code_store.go      # One-time auth code store
│   │   ├── notification_store.go   # Apple notification dedup store
│   │   ├── person_store.go         # people: dates for credits, insights for augur
│   │   ├── session_store.go        # Session store
│   │   ├── stats_store.go          # User stats store
│   │   ├── sync_job.go             # Sync job store operations
│   │   ├── user_store.go           # User + OAuth account store
│   │   ├── watch_event_store.go    # Watch event store
│   │   └── watchlist_store.go      # Watchlist store
│   │
│   └── tmdb/
│       ├── client.go               # TMDB HTTP client
│       ├── changes.go              # GetChanges() over the movie, tv, and person feeds
│       ├── collection.go           # GetCollection()
│       ├── credits.go              # Credits, AggregateCredits, CombinedCredits types
│       ├── episode.go              # GetEpisode()
│       ├── metadata.go             # Shared types
│       ├── movie.go                # GetMovie()
│       ├── person.go               # GetPerson()
│       ├── search.go               # SearchMulti()
│       ├── season.go               # GetSeason()
│       ├── series.go               # GetSeries()
│       └── watch.go                # WatchProviders types
│
├── local-development/
│   ├── api-examples.md             # Curl/HTTP examples for all endpoints
│   ├── docker-compose.yml          # Local Postgres setup
│   └── init.sql                    # Database schema
│
├── openapi/
│   ├── minimovie-api.yaml          # API specification
│   └── tmdb.json                   # TMDB API reference
│
├── .env
├── .gitignore
├── env.example
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Entities and Functionality

- Search
  - Global
  - Movies
  - Shows
  - Games (TBD)
  - People
- Movies
  - Details
  - People
  - Where to Watch
  - Trailer (TBD) - [API Docs](https://developer.themoviedb.org/reference/movie-videos)
- Shows
  - Details
  - People
  - Where to Watch
  - Trailer (TBD) - [API Docs](https://developer.themoviedb.org/reference/tv-series-videos)
  - Seasons
    - Details
    - People
    - Where to Watch
    - Episodes
      - Details
      - People
- People
  - Movies
  - Shows
  - Games (TBD)
- Games (TBD)
  - Details
  - People
  - Where to Play
  - Trailer (TBD)
- User
  - Watchlist (want_to_watch / watched)
  - Watch Events (movie / series / season / episode)
  - Stats (movies watched, series completed, episodes watched, hours, streaks, genre breakdown)
  - Achievements (gamification badges earned via watch activity)
  - Account management (delete, export)

## Authentication

OAuth 2.0 / OpenID Connect via [zitadel/oidc](https://github.com/zitadel/oidc). Supports Google and Apple Sign-in.

## Data Enrichments

### Age Enrichment

Credits for movies, series, seasons, and episodes are enriched with age data calculated from cast/crew birthdays, read from the `people` table with one query per request.

#### Flow

```mermaid
flowchart TD
    subgraph request [API Request Flow]
        A[Movie/Series Handler] --> B[catalog.Movie / Series / Season / Episode]
        B --> C[Row fresh?]
        C -->|Yes| D[Serve the stored document]
        C -->|Stale| E[Serve it, refresh in the background]
        C -->|Missing or expired| F[Fetch from TMDB, write, then serve]
        D --> G[GetDates for every credited person]
        E --> G
        F --> G
    end

    subgraph people [People without a hydrated row]
        G --> H{Director, writer, top 10 cast?}
        H -->|Yes, under MAX_TMDB_FETCH_PER_REQUEST| I[Fetch now]
        H -->|No| J[Queue for the in-process fetcher]
        I --> K[Calculate Age]
        J --> L[Present on the next request]
    end
```

### Person Priority System

Synchronous TMDB calls are capped per request to avoid N+1 problems. When a credited person has no hydrated row, the gap is filled in this order:

| Priority | Role        | Notes                     |
| -------- | ----------- | ------------------------- |
| 1        | Directors   | Always fetched first      |
| 2        | Writers     | Screenplay, Story, Writer |
| 3        | Top 10 Cast | By billing order          |
| 4        | Cast 11+    | Left to the fetcher       |
| 5        | Other Crew  | Left to the fetcher       |

#### Enrichment Output

- **Movies/Episodes**: Single age at release (`ageAtRelease: 32`)
- **Series**: Age range from first to last air date (`ageRange: "25-32"`)

## Catalog Seed

The catalog tables (`movies`, `series`, `seasons`, `episodes`, `collections`, `people`) hold TMDB data in Postgres: an identity `id` of our own, the provider's id as `source_id`, a `slug` generated from id and title (`155-the-dark-knight`; routes resolve it by the leading id), typed columns for what is filtered, sorted, or joined on, and the pruned TMDB document as `payload`. `cmd/seed` fills them from a laptop, never from Railway (only the api and sync binaries deploy).

```sh
make seed
bin/seed download --date $(date -v-1d +%m_%d_%Y)                                # movie_ids, tv_series_ids, person_ids
bin/seed save --entity people --file local-development/exports/person_ids_<date>.json.gz
caffeinate -is -- bin/seed hydrate --entity people --limit 100000                  # the next 100k unhydrated rows; rerun for more; then movies, series
bin/seed update                                                                     # changes feed since the last update, then refresh; --entity to pick one
bin/seed status
```

People go first so titles find their credits already hydrated. `save` is an idempotent upsert with zero API calls; `hydrate` claims never-hydrated rows by popularity, at or above `--min-popularity` (default 0.7), and runs the same fetch path the API uses, up to `--api-rate-limit` requests per second (default 40; TMDB's ceiling is about 50). Every run writes `local-development/exports/seed-<entity>.log` (JSON, debug level) and checkpoints `local-development/exports/state/<entity>.json` after every batch, so Ctrl-C is safe and the same command with `--resume` continues. `hydrate --limit N` hydrates the next N not-yet-hydrated rows; a row that failed or was interrupted is still unhydrated and is simply claimed again next time. `update` asks the changes feed which of our hydrated rows moved since the last update (the window starts at the last completed `sync_job_status` row for that entity, or a day before its oldest hydrated row; `--start`/`--end` override it), flags them, and refreshes every flagged or expiring row; rows TMDB has removed are deleted. Exit codes: 0 done, 1 failed, 2 usage or guard, 130 interrupted.

To move a local seed to another database, by hand: `pg_dump -Fc --no-owner --no-privileges -t movies -t series -t seasons -t episodes -t collections -t people "$DATABASE_URL" -f catalog.dump`, then against the target `psql -v step=extensions -f local-development/upgrade-catalog.sql`, `pg_restore --clean --if-exists --no-owner --no-privileges -j 4 -d "$TARGET" catalog.dump`, and `psql -v step=finish -f local-development/upgrade-catalog.sql` (carry-forward, user-row remap, drops).

## Daily Catalog Job

`cmd/sync` keeps the catalog current with TMDB (a person dies, a series adds a season) and within TMDB's six-month cap. It runs once a day and does five things in order:

1. **Changes**: for movies, series, and people, ask the changes feed which ids moved and flag the rows we hold as `stale`. A changed series also flags its hydrated seasons and episodes. Each entity's window is recorded as a `sync_job_status` row (`movie_sync`, `tv_sync`, `person_sync`); the next run starts where the last completed one ended, or a day before the entity's oldest hydrated row when no run exists. `SYNC_START_DATE`/`SYNC_END_DATE` or `make sync START=2026-01-01 END=2026-01-05` override the window for all three.
2. **Refresh**: refetch every stale row and every row older than 150 days.
3. **Hydrate**: spend `SYNC_HYDRATE_BUDGET` (default 2000) on the most popular skeleton rows, movies first.
4. **Purge**: delete rows older than 180 days from the six catalog tables, and expired sessions, auth codes, and notification ids.
5. **Stats**: log per-table counts; `oldest_days` is the compliance number.

A step that fails is logged and the run continues; the process exits 1 at the end if anything failed. Stale rows the job has not reached yet are still served: a request serves the stored document and refreshes it in the background.

```mermaid
flowchart LR
    subgraph sync_job [cmd/sync - Daily]
        A[Changes feed per entity] --> B[stale = true on held rows]
        B --> C[Refresh stale and expiring rows]
        C --> D[Hydrate popular skeletons within the budget]
        D --> E[Purge rows past 180 days]
        E --> F[Table stats]
    end

    subgraph api [API - On Request]
        G[Request] --> H{Row state}
        H -->|fresh| I[Serve]
        H -->|stale| J[Serve, refresh in the background]
        H -->|missing or expired| K[Fetch, write, serve]
    end

    B -.-> J
```
