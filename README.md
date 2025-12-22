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
│       └── main.go                 # Entry point, contains application main.go
│
├── config/
│   └── config.go                   # Config definitions and loader
│
├── internal/
│   ├── api/
│   │   ├── router.go               # Chi router setup, registers all routes
│   │   ├── response.go             # JSON(w, status, data), Error(w, status, msg)
│   │   └── handlers/
│   │       ├── handlers.go         # Handlers struct + New(tmdbClient, cfg)
│   │       ├── search.go           # SearchResponse + GetSearch()
│   │       ├── movie.go            # MovieResponse + GetMovie()
│   │       ├── show.go             # ShowResponse + GetShow()
│   │       └── person.go           # PersonResponse + GetPerson()
│   │
│   └── tmdb/
│       ├── client.go               # Client struct + NewClient(accessToken)
│       ├── search.go               # TMDBSearchResponse + SearchMulti()
│       ├── movie.go                # TMDBMovieResponse + GetMovie()
│       ├── show.go                 # TMDBShowResponse + GetShow()
│       └── person.go               # TMDBPersonResponse + GetPerson()
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
