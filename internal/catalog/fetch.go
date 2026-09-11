package catalog

import (
	"context"
	"errors"

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
			return s.seedPeople(ctx, db, refs)
		},
	}
	return m, w, nil
}

// TODO(2a): the request-path miss calls this; until then only the seed's get/writes split is used.
//
//nolint:unused
func (s *Service) fetchMovie(ctx context.Context, db store.DBTX, sourceID int) (*tmdb.Movie, []PersonRef, error) {
	m, w, err := s.getMovie(ctx, sourceID)
	if errors.Is(err, tmdb.ErrNotFound) {
		return nil, nil, errors.Join(err, s.movies.DeleteBySourceID(ctx, db, sourceID))
	}
	if err != nil {
		return nil, nil, err
	}
	if err := w.apply(ctx, db); err != nil {
		return nil, nil, err
	}
	return m, w.refs, nil
}

func (s *Service) getSeries(ctx context.Context, sourceID int) (*tmdb.Series, writes, error) {
	sr, err := s.tmdb.GetSeries(ctx, sourceID)
	if err != nil {
		return nil, writes{}, err
	}
	sr.WatchProviders.Prune(providerCountry)

	refs := personRefsFromAggregateCredits(sr.AggregateCredits)
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

// TODO(2a): the request-path miss calls this; until then only the seed's get/writes split is used.
//
//nolint:unused
func (s *Service) fetchSeries(ctx context.Context, db store.DBTX, sourceID int) (*tmdb.Series, []PersonRef, error) {
	sr, w, err := s.getSeries(ctx, sourceID)
	if errors.Is(err, tmdb.ErrNotFound) {
		return nil, nil, errors.Join(err, s.series.DeleteBySourceID(ctx, db, sourceID))
	}
	if err != nil {
		return nil, nil, err
	}
	if err := w.apply(ctx, db); err != nil {
		return nil, nil, err
	}
	return sr, w.refs, nil
}

// fetchSeason stores one season of an already stored series, plus a skeleton row per episode it lists.
func (s *Service) fetchSeason(
	ctx context.Context,
	db store.DBTX,
	series *store.Series,
	seasonNumber int,
) (*tmdb.SeasonDetails, []PersonRef, error) {
	sd, err := s.tmdb.GetSeason(ctx, series.SourceID, seasonNumber)
	if errors.Is(err, tmdb.ErrNotFound) {
		return nil, nil, errors.Join(err, s.seasons.Delete(ctx, db, series.ID, seasonNumber))
	}
	if err != nil {
		return nil, nil, err
	}
	sd.WatchProviders.Prune(providerCountry)

	row, err := seasonRow(series.ID, sd)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.seasons.Upsert(ctx, db, row); err != nil {
		return nil, nil, err
	}
	if err := s.episodes.UpsertSkeletons(ctx, db, series.ID, seasonNumber, episodeSkeletons(sd)); err != nil {
		return nil, nil, err
	}
	refs := personRefsFromAggregateCredits(sd.AggregateCredits)
	for _, ep := range sd.Episodes {
		refs = append(refs, personRefsFromCast(ep.GuestStars, PriorityCast)...)
	}
	if err := s.seedPeople(ctx, db, refs); err != nil {
		return nil, nil, err
	}
	return sd, refs, nil
}

// TODO(2a): the episode accessor calls this.
//
//nolint:unused
func (s *Service) fetchEpisode(
	ctx context.Context,
	db store.DBTX,
	series *store.Series,
	seasonNumber, episodeNumber int,
) (*tmdb.EpisodeDetails, []PersonRef, error) {
	ep, err := s.tmdb.GetEpisode(ctx, series.SourceID, seasonNumber, episodeNumber)
	if errors.Is(err, tmdb.ErrNotFound) {
		return nil, nil, errors.Join(err, s.episodes.Delete(ctx, db, series.ID, seasonNumber, episodeNumber))
	}
	if err != nil {
		return nil, nil, err
	}

	row, err := episodeRow(series.ID, ep)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.episodes.Upsert(ctx, db, row); err != nil {
		return nil, nil, err
	}
	refs := personRefsFromCredits(tmdb.Credits{Cast: ep.Credits.Cast, Crew: ep.Credits.Crew})
	refs = append(refs, personRefsFromCast(ep.Credits.GuestStars, PriorityCast)...)
	if err := s.seedPeople(ctx, db, refs); err != nil {
		return nil, nil, err
	}
	return ep, refs, nil
}

// upsertCollection stores a fetched collection and returns the row's id.
func (s *Service) upsertCollection(ctx context.Context, db store.DBTX, c *tmdb.Collection) (int, error) {
	row, err := collectionRow(c)
	if err != nil {
		return 0, err
	}
	return s.collections.Upsert(ctx, db, row)
}
