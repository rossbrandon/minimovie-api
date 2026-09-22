package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

const (
	missTimeout    = 10 * time.Second
	refreshTimeout = 30 * time.Second
)

const (
	stateMiss rowState = iota
	stateStale
	stateFresh
)

type rowState int

type rowMeta struct {
	payload   json.RawMessage
	fetchedAt *time.Time
	stale     bool
}

type fetch struct {
	key    string
	get    func(context.Context) (writes, error)
	delete func(context.Context, store.DBTX) error
}

func (m rowMeta) state(now time.Time) rowState {
	switch {
	case m.payload == nil || m.fetchedAt == nil || now.Sub(*m.fetchedAt) > store.ExpireAfter:
		return stateMiss
	case m.stale:
		return stateStale
	}
	return stateFresh
}

func (f fetch) run(ctx context.Context, db store.DBTX) (writes, error) {
	w, err := f.get(ctx)
	if errors.Is(err, tmdb.ErrNotFound) {
		return writes{}, errors.Join(ErrNotFound, err, f.delete(ctx, db))
	}
	return w, err
}

// ensure brings a row to a servable state and reports whether the caller must re-read it.
// A stale row is served as stored while one refresh runs in the background.
// A miss fetches the document now, inside the singleflight.
func (s *Service) ensure(ctx context.Context, m rowMeta, f fetch) (bool, error) {
	switch m.state(time.Now()) {
	case stateFresh:
		return false, nil
	case stateStale:
		s.bg.Go(ctx, "catalog.refresh", refreshTimeout, func(ctx context.Context) error {
			_, err, _ := s.sf.Do(f.key, func() (any, error) {
				w, err := f.run(ctx, s.pool)
				if err != nil {
					return nil, err
				}
				return nil, w.apply(ctx, s.pool)
			})
			return err
		})
		return false, nil
	}
	// The flight runs on its own context so the first caller disconnecting cannot fail every waiter.
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), missTimeout)
	defer cancel()
	_, err, _ := s.sf.Do(f.key, func() (any, error) {
		w, err := f.run(dctx, s.pool)
		if err != nil {
			return nil, err
		}
		if err := w.write(dctx, s.pool); err != nil {
			return nil, err
		}
		s.bg.Go(dctx, "catalog.seed", missTimeout, func(ctx context.Context) error { return w.seed(ctx, s.pool) })
		return nil, nil
	})
	return err == nil, err
}

func (s *Service) loadMovie(ctx context.Context, id int) (*store.Movie, *tmdb.Movie, error) {
	row, err := s.movies.GetByID(ctx, id)
	if err != nil || row == nil {
		return nil, nil, missing(err)
	}
	refetched, err := s.ensure(ctx, metaOf(row.Payload, row.FetchedAt, row.Stale), fetch{
		key: "movie:" + strconv.Itoa(row.SourceID),
		get: func(ctx context.Context) (writes, error) {
			_, w, err := s.getMovie(ctx, row.SourceID)
			return w, err
		},
		delete: func(ctx context.Context, db store.DBTX) error {
			return s.movies.DeleteBySourceID(ctx, db, row.SourceID)
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if refetched {
		// The slug follows the hydrated title and the collection may have appeared.
		if row, err = s.movies.GetByID(ctx, id); err != nil || row == nil {
			return nil, nil, missing(err)
		}
	}
	var m tmdb.Movie
	if err := json.Unmarshal(row.Payload, &m); err != nil {
		return nil, nil, fmt.Errorf("catalog: movie %d: %w", id, err)
	}
	return row, &m, nil
}

func (s *Service) ensureSeries(ctx context.Context, id int) (*store.Series, error) {
	row, err := s.series.GetByID(ctx, id)
	if err != nil || row == nil {
		return nil, missing(err)
	}
	refetched, err := s.ensure(ctx, metaOf(row.Payload, row.FetchedAt, row.Stale), fetch{
		key: "series:" + strconv.Itoa(row.SourceID),
		get: func(ctx context.Context) (writes, error) {
			_, w, err := s.getSeries(ctx, row.SourceID)
			return w, err
		},
		delete: func(ctx context.Context, db store.DBTX) error {
			return s.series.DeleteBySourceID(ctx, db, row.SourceID)
		},
	})
	if err != nil {
		return nil, err
	}
	if refetched {
		if row, err = s.series.GetByID(ctx, id); err != nil || row == nil {
			return nil, missing(err)
		}
	}
	return row, nil
}

func (s *Service) loadSeries(ctx context.Context, id int) (*store.Series, *tmdb.Series, error) {
	row, err := s.ensureSeries(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	var sr tmdb.Series
	if err := json.Unmarshal(row.Payload, &sr); err != nil {
		return nil, nil, fmt.Errorf("catalog: series %d: %w", id, err)
	}
	return row, &sr, nil
}

func (s *Service) ensureSeason(ctx context.Context, series *store.Series, seasonNumber int) (*store.Season, error) {
	row, err := s.seasons.Get(ctx, series.ID, seasonNumber)
	if err != nil || row == nil {
		return nil, missing(err)
	}
	refetched, err := s.ensure(ctx, metaOf(row.Payload, row.FetchedAt, row.Stale), fetch{
		key: "season:" + strconv.Itoa(series.SourceID) + "/" + strconv.Itoa(seasonNumber),
		get: func(ctx context.Context) (writes, error) {
			_, w, err := s.getSeason(ctx, series, seasonNumber)
			return w, err
		},
		delete: func(ctx context.Context, db store.DBTX) error {
			return s.seasons.Delete(ctx, db, series.ID, seasonNumber)
		},
	})
	if err != nil {
		return nil, err
	}
	if refetched {
		if row, err = s.seasons.Get(ctx, series.ID, seasonNumber); err != nil || row == nil {
			return nil, missing(err)
		}
	}
	return row, nil
}

func (s *Service) loadSeason(
	ctx context.Context,
	series *store.Series,
	seasonNumber int,
) (*store.Season, *tmdb.SeasonDetails, error) {
	row, err := s.ensureSeason(ctx, series, seasonNumber)
	if err != nil {
		return nil, nil, err
	}
	var sd tmdb.SeasonDetails
	if err := json.Unmarshal(row.Payload, &sd); err != nil {
		return nil, nil, fmt.Errorf("catalog: season %d/%d: %w", series.ID, seasonNumber, err)
	}
	return row, &sd, nil
}

func (s *Service) loadEpisode(
	ctx context.Context,
	series *store.Series,
	seasonNumber, episodeNumber int,
) (*store.Episode, *tmdb.EpisodeDetails, error) {
	if _, err := s.ensureSeason(ctx, series, seasonNumber); err != nil {
		return nil, nil, err
	}
	row, err := s.episodes.Get(ctx, series.ID, seasonNumber, episodeNumber)
	if err != nil || row == nil {
		return nil, nil, missing(err)
	}
	refetched, err := s.ensure(ctx, metaOf(row.Payload, row.FetchedAt, row.Stale), fetch{
		key: "episode:" + strconv.Itoa(series.SourceID) + "/" + strconv.Itoa(seasonNumber) + "/" +
			strconv.Itoa(episodeNumber),
		get: func(ctx context.Context) (writes, error) {
			_, w, err := s.getEpisode(ctx, series, seasonNumber, episodeNumber)
			return w, err
		},
		delete: func(ctx context.Context, db store.DBTX) error {
			return s.episodes.Delete(ctx, db, series.ID, seasonNumber, episodeNumber)
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if refetched {
		if row, err = s.episodes.Get(ctx, series.ID, seasonNumber, episodeNumber); err != nil || row == nil {
			return nil, nil, missing(err)
		}
	}
	var ep tmdb.EpisodeDetails
	if err := json.Unmarshal(row.Payload, &ep); err != nil {
		return nil, nil, fmt.Errorf("catalog: episode %d/%d/%d: %w", series.ID, seasonNumber, episodeNumber, err)
	}
	return row, &ep, nil
}

func (s *Service) loadPerson(ctx context.Context, id int) (*store.Person, *tmdb.Person, error) {
	row, err := s.people.GetByID(ctx, id)
	if err != nil || row == nil {
		return nil, nil, missing(err)
	}
	refetched, err := s.ensure(ctx, metaOf(row.Payload, row.FetchedAt, row.Stale), fetch{
		key: "person:" + strconv.Itoa(row.SourceID),
		get: func(ctx context.Context) (writes, error) {
			_, w, err := s.getPerson(ctx, row.SourceID)
			return w, err
		},
		delete: func(ctx context.Context, db store.DBTX) error {
			return s.people.DeleteBySourceID(ctx, db, row.SourceID)
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if refetched {
		if row, err = s.people.GetByID(ctx, id); err != nil || row == nil {
			return nil, nil, missing(err)
		}
	}
	var p tmdb.Person
	if err := json.Unmarshal(row.Payload, &p); err != nil {
		return nil, nil, fmt.Errorf("catalog: person %d: %w", id, err)
	}
	return row, &p, nil
}

func metaOf(payload json.RawMessage, fetchedAt *time.Time, stale bool) rowMeta {
	return rowMeta{payload: payload, fetchedAt: fetchedAt, stale: stale}
}

// missing turns a read that found nothing into ErrNotFound and passes a real error through.
func missing(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFound
}
