package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"golang.org/x/sync/singleflight"
)

const (
	providerCountry           = "US"
	defaultPeoplePerHydration = 10
	personFetchConcurrency    = 15
)

const (
	EntityUnknown Entity = iota
	EntityMovie
	EntitySeries
	EntitySeason
	EntityEpisode
	EntityPerson
)

var ErrNotFound = errors.New("catalog: not found")

type Entity int

type Deps struct {
	Pool               *pgxpool.Pool
	Movies             *store.MovieStore
	Series             *store.SeriesStore
	Seasons            *store.SeasonStore
	Episodes           *store.EpisodeStore
	People             *store.PersonStore
	Collections        *store.CollectionStore
	TMDB               tmdb.MediaClient
	PeoplePerHydration int
}

type Service struct {
	pool        *pgxpool.Pool
	movies      *store.MovieStore
	series      *store.SeriesStore
	seasons     *store.SeasonStore
	episodes    *store.EpisodeStore
	people      *store.PersonStore
	collections *store.CollectionStore
	tmdb        tmdb.MediaClient

	peoplePerHydration int
	sf                 singleflight.Group
}

func New(d Deps) *Service {
	if d.PeoplePerHydration <= 0 {
		d.PeoplePerHydration = defaultPeoplePerHydration
	}
	return &Service{
		pool:               d.Pool,
		movies:             d.Movies,
		series:             d.Series,
		seasons:            d.Seasons,
		episodes:           d.Episodes,
		people:             d.People,
		collections:        d.Collections,
		tmdb:               d.TMDB,
		peoplePerHydration: d.PeoplePerHydration,
	}
}

// ParseEntity accepts the table names: movies, series, seasons, episodes, people.
func ParseEntity(s string) (Entity, error) {
	for _, e := range []Entity{EntityMovie, EntitySeries, EntitySeason, EntityEpisode, EntityPerson} {
		if e.String() == s {
			return e, nil
		}
	}
	return EntityUnknown, fmt.Errorf("catalog: unknown entity %q", s)
}

// Stats returns one row per id-keyed catalog table, in the order the seed and the daily job work them.
func (s *Service) Stats(ctx context.Context) ([]store.CatalogStats, error) {
	tables := []func(context.Context) (store.CatalogStats, error){s.people.Stats, s.movies.Stats, s.series.Stats}
	out := make([]store.CatalogStats, 0, len(tables))
	for _, stats := range tables {
		st, err := stats(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (e Entity) SyncJobType() string {
	switch e {
	case EntityMovie:
		return "movie_sync"
	case EntitySeries:
		return "tv_sync"
	case EntityPerson:
		return "person_sync"
	}
	return "unknown_sync"
}

// String is the entity's table name.
func (e Entity) String() string {
	switch e {
	case EntityMovie:
		return "movies"
	case EntitySeries:
		return "series"
	case EntitySeason:
		return "seasons"
	case EntityEpisode:
		return "episodes"
	case EntityPerson:
		return "people"
	}
	return "unknown"
}
