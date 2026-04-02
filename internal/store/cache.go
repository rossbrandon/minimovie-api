package store

import (
	"context"
	"time"
)

type PersonCache interface {
	Get(personID int) (dob, dod string, found bool)
	Set(personID int, dob, dod string)
	SetBatch(entries map[int]PersonDates)
	Delete(personID int)
}

type SeasonCastCache interface {
	Get(ctx context.Context, seriesID, seasonNumber int) (castMap map[int]int, found bool)
	Set(ctx context.Context, seriesID, seasonNumber int, castMap map[int]int, expiresAt time.Time)
}

// Purgeable is implemented by any Postgres cache store with expiring records.
// The cleanup command iterates all registered Purgeable stores and calls PurgeExpired.
type Purgeable interface {
	PurgeExpired(ctx context.Context) (rowsPurged int64, err error)
	TableName() string
}

// SeasonCastTieredCache composes BigCache and Postgres into a single cache.
type SeasonCastTieredCache struct {
	cache *SeasonCastBigCacheAdapter
	store *SeasonCastPostgresStore
}

func NewSeasonCastTieredCache(cache *SeasonCastBigCacheAdapter, store *SeasonCastPostgresStore) *SeasonCastTieredCache {
	return &SeasonCastTieredCache{cache: cache, store: store}
}

func (c *SeasonCastTieredCache) Get(ctx context.Context, seriesID, seasonNumber int) (map[int]int, bool) {
	if castMap, ok := c.cache.Get(ctx, seriesID, seasonNumber); ok {
		return castMap, true
	}

	castMap, ok := c.store.Get(ctx, seriesID, seasonNumber)
	if !ok {
		return nil, false
	}

	c.cache.Set(ctx, seriesID, seasonNumber, castMap, time.Time{})
	return castMap, true
}

func (c *SeasonCastTieredCache) Set(ctx context.Context, seriesID, seasonNumber int, castMap map[int]int, expiresAt time.Time) {
	c.cache.Set(ctx, seriesID, seasonNumber, castMap, expiresAt)
	c.store.Set(ctx, seriesID, seasonNumber, castMap, expiresAt)
}
