package handlers

import (
	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Handlers struct {
	tmdbClient      *tmdb.Client
	ageResolver     *age.Resolver
	seasonCastCache store.SeasonCastCache
}

func NewHandlers(tmdbClient *tmdb.Client, ageResolver *age.Resolver, seasonCastCache store.SeasonCastCache) *Handlers {
	return &Handlers{
		tmdbClient:      tmdbClient,
		ageResolver:     ageResolver,
		seasonCastCache: seasonCastCache,
	}
}
