package handlers

import (
	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Handlers struct {
	tmdbClient  *tmdb.Client
	ageResolver *age.Resolver
}

func NewHandlers(tmdbClient *tmdb.Client, ageResolver *age.Resolver) *Handlers {
	return &Handlers{
		tmdbClient:  tmdbClient,
		ageResolver: ageResolver,
	}
}
