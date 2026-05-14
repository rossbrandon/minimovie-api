package series

import (
	"context"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

const (
	dbQueryTimeout      = 3 * time.Second
	asyncPersistTimeout = 5 * time.Second
)

type Service struct {
	cache      *store.SeriesMetadataBigCacheAdapter
	db         *store.SeriesMetadataStore
	tmdbClient *tmdb.Client
}

func New(ctx context.Context, db *store.SeriesMetadataStore, tmdbClient *tmdb.Client) (*Service, error) {
	cache, err := store.NewSeriesMetadataBigCacheAdapter(ctx)
	if err != nil {
		return nil, err
	}
	return &Service{cache: cache, db: db, tmdbClient: tmdbClient}, nil
}

// GetSeries reads BigCache first, then Postgres on miss, then TMDB. TMDB
// fetches write back to BigCache synchronously and persist to Postgres in a
// goroutine with a detached context so the row survives request cancellation.
func (s *Service) GetSeries(ctx context.Context, seriesID int) (*store.SeriesMetadata, error) {
	if meta, ok := s.cache.Get(ctx, seriesID); ok {
		return meta, nil
	}

	dbCtx, cancel := context.WithTimeout(ctx, dbQueryTimeout)
	defer cancel()

	if meta, err := s.db.Get(dbCtx, seriesID); err != nil {
		log.Warn().Err(err).Int("series_id", seriesID).Msg("series metadata DB read failed")
	} else if meta != nil {
		s.cache.Set(ctx, meta)
		return meta, nil
	}

	return s.fetchFromTmdb(ctx, seriesID)
}

// Update refreshes the series_metadata row in the background
func (s *Service) UpdateSeries(seriesID int) {
	if s == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), asyncPersistTimeout)
		defer cancel()
		if _, err := s.GetSeries(ctx, seriesID); err != nil {
			log.Debug().Err(err).Int("series_id", seriesID).Msg("series metadata update failed")
		}
	}()
}

func (s *Service) fetchFromTmdb(ctx context.Context, seriesID int) (*store.SeriesMetadata, error) {
	series, err := s.tmdbClient.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	meta := toMetadata(series)
	s.cache.Set(ctx, meta)
	s.persistAsync(ctx, meta)
	return meta, nil
}

func (s *Service) persistAsync(ctx context.Context, meta *store.SeriesMetadata) {
	go func() {
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), asyncPersistTimeout)
		defer cancel()
		if err := s.db.Upsert(bgCtx, meta); err != nil {
			log.Warn().Err(err).Int("series_id", meta.SeriesID).Msg("series metadata upsert failed")
		}
	}()
}

func toMetadata(series *tmdb.Series) *store.SeriesMetadata {
	meta := &store.SeriesMetadata{
		SeriesID:            series.ID,
		Name:                series.Name,
		TotalEpisodes:       series.NumberOfEpisodes,
		TotalSeasons:        series.NumberOfSeasons,
		SeasonEpisodeCounts: collectSeasonEpisodeCounts(series.Seasons),
		InProduction:        series.InProduction,
		Status:              series.Status,
		LastAirDate:         series.LastAirDate,
		Fetched:             true,
		FetchedAt:           time.Now(),
	}
	if series.NextEpisodeToAir != nil {
		meta.NextAirDate = series.NextEpisodeToAir.AirDate
	}
	return meta
}

// collectSeasonEpisodeCounts skips specials (season 0)
func collectSeasonEpisodeCounts(seasons []tmdb.Season) map[int]int {
	if len(seasons) == 0 {
		return nil
	}
	counts := make(map[int]int, len(seasons))
	for _, season := range seasons {
		if season.SeasonNumber <= 0 {
			continue
		}
		counts[season.SeasonNumber] = season.EpisodeCount
	}
	return counts
}
