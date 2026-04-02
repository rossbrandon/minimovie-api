package store

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

type SeasonCastBigCacheAdapter struct {
	cache *bigcache.BigCache
}

func NewSeasonCastBigCacheAdapter(ctx context.Context) (*SeasonCastBigCacheAdapter, error) {
	config := bigcache.Config{
		Shards:             256,
		LifeWindow:         24 * time.Hour,
		CleanWindow:        1 * time.Hour,
		MaxEntriesInWindow: 500000,
		MaxEntrySize:       5000,
		Verbose:            false,
		HardMaxCacheSize:   50,
		StatsEnabled:       false,
	}

	cache, err := bigcache.New(ctx, config)
	if err != nil {
		return nil, err
	}

	return &SeasonCastBigCacheAdapter{cache: cache}, nil
}

func seasonCastKey(seriesID, seasonNumber int) string {
	return "sc:" + strconv.Itoa(seriesID) + ":" + strconv.Itoa(seasonNumber)
}

func (a *SeasonCastBigCacheAdapter) Get(ctx context.Context, seriesID, seasonNumber int) (map[int]int, bool) {
	data, err := a.cache.Get(seasonCastKey(seriesID, seasonNumber))
	if err != nil {
		if metrics.M != nil {
			metrics.M.RecordCacheMiss(ctx, "season_cast")
		}
		return nil, false
	}
	if metrics.M != nil {
		metrics.M.RecordCacheHit(ctx, "season_cast")
	}

	var castMap map[int]int
	if err := json.Unmarshal(data, &castMap); err != nil {
		log.Warn().Err(err).Int("series_id", seriesID).Int("season", seasonNumber).Msg("failed to decode season cast cache entry")
		return nil, false
	}

	return castMap, true
}

func (a *SeasonCastBigCacheAdapter) Set(ctx context.Context, seriesID, seasonNumber int, castMap map[int]int, _ time.Time) {
	data, err := json.Marshal(castMap)
	if err != nil {
		log.Warn().Err(err).Int("series_id", seriesID).Int("season", seasonNumber).Msg("failed to encode season cast cache entry")
		return
	}

	if err := a.cache.Set(seasonCastKey(seriesID, seasonNumber), data); err != nil {
		log.Warn().Err(err).Int("series_id", seriesID).Int("season", seasonNumber).Msg("failed to set season cast cache entry")
		return
	}
	if metrics.M != nil {
		metrics.M.RecordCacheWrite(ctx, "season_cast")
	}
}

func (a *SeasonCastBigCacheAdapter) Close() error {
	return a.cache.Close()
}
