# MiniMovie API

Simple API backend for _MiniMovie_ written in Go.

## Proposed Structure

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
