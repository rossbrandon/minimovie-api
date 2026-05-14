package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

type SeriesMetadataBigCacheAdapter struct {
	cache *bigcache.BigCache
}

func NewSeriesMetadataBigCacheAdapter(ctx context.Context) (*SeriesMetadataBigCacheAdapter, error) {
	config := bigcache.Config{
		Shards:             256,
		LifeWindow:         24 * time.Hour,
		CleanWindow:        1 * time.Hour,
		MaxEntriesInWindow: 100000,
		MaxEntrySize:       300,
		Verbose:            false,
		HardMaxCacheSize:   50,
		StatsEnabled:       false,
	}

	cache, err := bigcache.New(ctx, config)
	if err != nil {
		return nil, err
	}

	return &SeriesMetadataBigCacheAdapter{cache: cache}, nil
}

func (a *SeriesMetadataBigCacheAdapter) Get(ctx context.Context, seriesID int) (*SeriesMetadata, bool) {
	data, err := a.cache.Get(seriesMetaKey(seriesID))
	if err != nil {
		if metrics.M != nil {
			metrics.M.RecordCacheMiss(ctx, "series_metadata")
		}
		return nil, false
	}
	if metrics.M != nil {
		metrics.M.RecordCacheHit(ctx, "series_metadata")
	}

	var meta SeriesMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		log.Warn().Err(err).Int("series_id", seriesID).Msg("failed to decode series metadata cache entry")
		return nil, false
	}
	return &meta, true
}

func (a *SeriesMetadataBigCacheAdapter) Set(ctx context.Context, meta *SeriesMetadata) {
	data, err := json.Marshal(meta)
	if err != nil {
		log.Warn().Err(err).Int("series_id", meta.SeriesID).Msg("failed to encode series metadata cache entry")
		return
	}
	if err := a.cache.Set(seriesMetaKey(meta.SeriesID), data); err != nil {
		log.Warn().Err(err).Int("series_id", meta.SeriesID).Msg("failed to set series metadata cache entry")
		return
	}
	if metrics.M != nil {
		metrics.M.RecordCacheWrite(ctx, "series_metadata")
	}
}

func (a *SeriesMetadataBigCacheAdapter) Delete(seriesID int) {
	if err := a.cache.Delete(seriesMetaKey(seriesID)); err != nil && !errors.Is(err, bigcache.ErrEntryNotFound) {
		log.Warn().Err(err).Int("series_id", seriesID).Msg("failed to delete series metadata cache entry")
	}
}

func (a *SeriesMetadataBigCacheAdapter) Close() error {
	return a.cache.Close()
}

func seriesMetaKey(seriesID int) string {
	return "sm:" + strconv.Itoa(seriesID)
}
