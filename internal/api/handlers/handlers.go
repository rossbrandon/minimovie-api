package handlers

import (
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Handlers struct {
	tmdbClient *tmdb.Client
}

func NewHandlers(tmdbClient *tmdb.Client) *Handlers {
	return &Handlers{tmdbClient: tmdbClient}
}
