package handlers

import (
	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/augur"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Handlers struct {
	tmdbClient      *tmdb.Client
	ageResolver     *age.Resolver
	seasonCastCache store.SeasonCastCache
	augurResolver   *augur.Resolver
}

func NewHandlers(tmdbClient *tmdb.Client, ageResolver *age.Resolver, seasonCastCache store.SeasonCastCache, augurResolver *augur.Resolver) *Handlers {
	return &Handlers{
		tmdbClient:      tmdbClient,
		ageResolver:     ageResolver,
		seasonCastCache: seasonCastCache,
		augurResolver:   augurResolver,
	}
}
