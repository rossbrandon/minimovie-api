package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

type BigCacheAdapter struct {
	cache *bigcache.BigCache
}

func NewBigCacheAdapter(ctx context.Context) (*BigCacheAdapter, error) {
	config := bigcache.Config{
		Shards:             256,
		LifeWindow:         24 * time.Hour,
		CleanWindow:        1 * time.Hour,
		MaxEntriesInWindow: 1000000,
		MaxEntrySize:       50,
		Verbose:            false,
		HardMaxCacheSize:   100,
		StatsEnabled:       false,
	}

	cache, err := bigcache.New(ctx, config)
	if err != nil {
		return nil, err
	}

	return &BigCacheAdapter{cache: cache}, nil
}

func key(personID int) string {
	return "person:" + strconv.Itoa(personID)
}

func encode(dob, dod string) []byte {
	return []byte(dob + "|" + dod)
}

func decode(data []byte) (dob, dod string) {
	parts := strings.SplitN(string(data), "|", 2)
	if len(parts) >= 1 {
		dob = parts[0]
	}
	if len(parts) >= 2 {
		dod = parts[1]
	}
	return
}

func (a *BigCacheAdapter) Get(personID int) (dob, dod string, found bool) {
	data, err := a.cache.Get(key(personID))
	if err != nil {
		if metrics.M != nil {
			metrics.M.RecordCacheMiss(context.Background())
		}
		return "", "", false
	}
	if metrics.M != nil {
		metrics.M.RecordCacheHit(context.Background())
	}
	dob, dod = decode(data)
	return dob, dod, true
}

func (a *BigCacheAdapter) Set(personID int, dob, dod string) {
	if err := a.cache.Set(key(personID), encode(dob, dod)); err != nil {
		log.Warn().Err(err).Int("person_id", personID).Msg("failed to set cache entry")
		return
	}
	if metrics.M != nil {
		metrics.M.RecordCacheWrite(context.Background())
	}
}

func (a *BigCacheAdapter) SetBatch(entries map[int]PersonDates) {
	for id, dates := range entries {
		a.Set(id, dates.DateOfBirth, dates.DateOfDeath)
	}
}

func (a *BigCacheAdapter) Delete(personID int) {
	err := a.cache.Delete(key(personID))
	if err != nil && !errors.Is(err, bigcache.ErrEntryNotFound) {
		log.Warn().Err(err).Int("person_id", personID).Msg("failed to delete local cache entry for person")
	}
}

func (a *BigCacheAdapter) Close() error {
	return a.cache.Close()
}
