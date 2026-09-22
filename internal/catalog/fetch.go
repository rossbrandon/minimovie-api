package catalog

import (
	"context"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

type writes struct {
	refs  []PersonRef
	write func(ctx context.Context, db store.DBTX) error
	seed  func(ctx context.Context, db store.DBTX) error
}

func (w writes) apply(ctx context.Context, db store.DBTX) error {
	if err := w.write(ctx, db); err != nil {
		return err
	}
	return w.seed(ctx, db)
}

func (s *Service) getMovie(ctx context.Context, sourceID int) (*tmdb.Movie, writes, error) {
	m, err := s.tmdb.GetMovie(ctx, sourceID)
	if err != nil {
		return nil, writes{}, err
	}
	m.WatchProviders.Prune(providerCountry)

	var collection *tmdb.Collection
	if m.BelongsToCollection != nil {
		collection, err = s.tmdb.GetCollection(ctx, m.BelongsToCollection.ID)
		if err != nil {
			log.Warn().Err(err).Int("collection_source_id", m.BelongsToCollection.ID).
				Msg("catalog: collection fetch failed")
			collection = nil
		}
	}

	refs := personRefsFromCredits(m.Credits)
	w := writes{
		refs: refs,
		write: func(ctx context.Context, db store.DBTX) error {
			var collectionID *int
			if collection != nil {
				// Collections are shared between movies, so like the seeds they are written outside the
				// caller's transaction: a batch holding one collection's lock while waiting for another
				// deadlocks a sibling batch doing the reverse.
				id, err := s.upsertCollection(ctx, s.pool, collection)
				if err != nil {
					return err
				}
				collectionID = &id
			}
			row, err := movieRow(m, collectionID)
			if err != nil {
				return err
			}
			_, err = s.movies.UpsertHydrated(ctx, db, row)
			return err
		},
		seed: func(ctx context.Context, db store.DBTX) error {
			if err := s.seedPeople(ctx, db, refs); err != nil {
				return err
			}
			if collection == nil {
				return nil
			}
			return s.movies.UpsertSkeleton(ctx, db, partSkeletons(collection))
		},
	}
	return m, w, nil
}

func (s *Service) getSeries(ctx context.Context, sourceID int) (*tmdb.Series, writes, error) {
	sr, err := s.tmdb.GetSeries(ctx, sourceID)
	if err != nil {
		return nil, writes{}, err
	}
	sr.WatchProviders.Prune(providerCountry)

	refs := personRefsFromSeries(sr)
	w := writes{
		refs: refs,
		write: func(ctx context.Context, db store.DBTX) error {
			row, err := seriesRow(sr)
			if err != nil {
				return err
			}
			seriesID, err := s.series.UpsertHydrated(ctx, db, row)
			if err != nil {
				return err
			}
			return s.seasons.UpsertSkeletons(ctx, db, seriesID, seasonSkeletons(sr))
		},
		seed: func(ctx context.Context, db store.DBTX) error {
			return s.seedPeople(ctx, db, refs)
		},
	}
	return sr, w, nil
}

// getSeason fetches one season of a stored series; write stores it with a skeleton row per episode it lists.
func (s *Service) getSeason(
	ctx context.Context,
	series *store.Series,
	seasonNumber int,
) (*tmdb.SeasonDetails, writes, error) {
	sd, err := s.tmdb.GetSeason(ctx, series.SourceID, seasonNumber)
	if err != nil {
		return nil, writes{}, err
	}
	sd.WatchProviders.Prune(providerCountry)

	refs := personRefsFromSeason(sd)
	w := writes{
		refs: refs,
		write: func(ctx context.Context, db store.DBTX) error {
			row, err := seasonRow(series.ID, sd)
			if err != nil {
				return err
			}
			if _, err := s.seasons.Upsert(ctx, db, row); err != nil {
				return err
			}
			return s.episodes.UpsertSkeletons(ctx, db, series.ID, seasonNumber, episodeSkeletons(sd))
		},
		seed: func(ctx context.Context, db store.DBTX) error {
			return s.seedPeople(ctx, db, refs)
		},
	}
	return sd, w, nil
}

func (s *Service) getEpisode(
	ctx context.Context,
	series *store.Series,
	seasonNumber, episodeNumber int,
) (*tmdb.EpisodeDetails, writes, error) {
	ep, err := s.tmdb.GetEpisode(ctx, series.SourceID, seasonNumber, episodeNumber)
	if err != nil {
		return nil, writes{}, err
	}

	refs := personRefsFromEpisode(ep)
	w := writes{
		refs: refs,
		write: func(ctx context.Context, db store.DBTX) error {
			row, err := episodeRow(series.ID, ep)
			if err != nil {
				return err
			}
			_, err = s.episodes.Upsert(ctx, db, row)
			return err
		},
		seed: func(ctx context.Context, db store.DBTX) error {
			return s.seedPeople(ctx, db, refs)
		},
	}
	return ep, w, nil
}

// upsertCollection stores a fetched collection and returns the row's id.
func (s *Service) upsertCollection(ctx context.Context, db store.DBTX, c *tmdb.Collection) (int, error) {
	row, err := collectionRow(c)
	if err != nil {
		return 0, err
	}
	return s.collections.Upsert(ctx, db, row)
}
