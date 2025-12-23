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
│   └── api/
│       └── main.go                 # Entry point
│
├── config/
│   └── config.go                   # Config definitions and loader
│
├── internal/
│   ├── api/
│   │   ├── router.go               # Chi router setup, registers all routes
│   │   └── handlers/
│   │       ├── handlers.go         # Chi API Handlers
│   │       ├── credits.go          # Credits types and functions
│   │       ├── watch.go            # WatchProviders types and functions
│   │       ├── search.go           # SearchMulti handler
│   │       ├── movie.go            # GetMovie handler
│   │       ├── series.go           # GetSeries handler
│   │       ├── season.go           # GetSeason handler
│   │       ├── episode.go          # GetEpisode handler
│   │       └── person.go           # GetPerson handler
│   │
│   ├── httputil/
│   │   └── response.go             # JSON(w, status, data), Error(w, status, msg)
│   │
│   └── tmdb/
│       ├── client.go               # TMDB Client
│       ├── credits.go              # Credits, AggregateCredits, CombinedCredits types
│       ├── metadata.go             # Shared types
│       ├── watch.go                # WatchProviders types
│       ├── search.go               # SearchMulti()
│       ├── movie.go                # GetMovie()
│       ├── series.go               # GetSeries()
│       ├── season.go               # GetSeason()
│       ├── episode.go              # GetEpisode()
│       └── person.go               # GetPerson()
│
├── .env
├── .gitignore
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

## Data Enrichments

### Age Enrichment

Credits for movies, series, seasons, and episodes are enriched with age data calculated from cast/crew birthdays.

#### Flow

```mermaid
flowchart TD
    subgraph request [API Request Flow]
        A[Movie/Series Handler] --> B[Build Credits]
        B --> C[Enrich with Ages]
    end

    subgraph lookup [Birthday Lookup - Priority Order]
        C --> D{BigCache?}
        D -->|Hit| H[Calculate Age]
        D -->|Miss| E{Postgres?}
        E -->|Found| F[Update BigCache]
        F --> H
        E -->|Not Found or fetched=false| G{Under Limit?}
        G -->|Yes| I[Fetch from TMDB]
        G -->|No| J[Skip - No Age]
        I --> K[Update Postgres + BigCache]
        K --> H
    end

    subgraph priority [Priority-Based TMDB Fetching]
        L[Directors] --> M[Writers]
        M --> N[Top 10 Cast]
        N --> O[Remaining Cast]
        O --> P[Other Crew]
    end
```

#### Data Flow

1. **BigCache** (in-memory, 24h TTL) — fastest, checked first
2. **Postgres** — persistent storage, checked on cache miss
3. **TMDB API** — external source, fetched only when needed

### Person Priority System

TMDB API calls are limited per request to avoid N+1 problems. When fetching is required, people are prioritized:

| Priority | Role        | Notes                     |
| -------- | ----------- | ------------------------- |
| 1        | Directors   | Always fetched first      |
| 2        | Writers     | Screenplay, Story, Writer |
| 3        | Top 10 Cast | By billing order          |
| 4        | Cast 11-25  | Lower priority            |
| 5        | Other Crew  | Fetched last              |

#### Enrichment Output

- **Movies/Episodes**: Single age at release (`ageAtRelease: 32`)
- **Series**: Age range from first to last air date (`ageRange: "25-32"`)

## Deployment

### Observability

- [Grafana Dashboard](https://rossbrandon.grafana.net/d/rofg2q7/minimovie-api-service-metrics?orgId=1&from=now-30m&to=now&timezone=browser)
- [Railway Dashboard](https://railway.com/project/fff7464f-52c9-4e91-b358-632b1e4202fb/observability?environmentId=9f32b1f7-f66e-4146-9982-1ec9aef6f573)
- [Railway Logs](https://railway.com/project/fff7464f-52c9-4e91-b358-632b1e4202fb/logs?environmentId=9f32b1f7-f66e-4146-9982-1ec9aef6f573)

### Cloudflare

#### API Allowlist

The following [Security Rule](https://dash.cloudflare.com/dd630cbdf4b6a4502d25f006d309725c/minimovie.info/security/security-rules) has been defined for the `api.minimovie.info` domain.

```
(http.host eq "api.minimovie.info"
and not http.request.uri.path in {"/ping" "/search"}
and not starts_with(http.request.uri.path, "/series/")
and not starts_with(http.request.uri.path, "/people/")
and not starts_with(http.request.uri.path, "/movies/")
and not starts_with(http.request.uri.path, "/series/")
)
```
