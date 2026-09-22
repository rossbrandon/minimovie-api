package catalog

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

// SyncChanges asks the provider which rows of an entity changed between start and end (YYYY-MM-DD,
// inclusive) and flags the ones we hold as stale for the next refresh. It returns how many ids the
// feed listed and how many rows were flagged.
func (s *Service) SyncChanges(
	ctx context.Context,
	entity Entity,
	start, end string,
) (changed int, marked int64, err error) {
	mediaType, err := mediaType(entity)
	if err != nil {
		return 0, 0, err
	}
	table, err := s.table(entity)
	if err != nil {
		return 0, 0, err
	}
	ids, err := s.tmdb.GetChanges(ctx, mediaType, start, end)
	if err != nil {
		return 0, 0, fmt.Errorf("catalog: %s changes: %w", entity, err)
	}
	marked, err = table.MarkStale(ctx, ids)
	if err != nil || entity != EntitySeries {
		return len(ids), marked, err
	}
	// A changed series may have changed its seasons and episodes too.
	// Their documents are refreshed on the next request.
	held, err := s.series.IDsBySource(ctx, ids)
	if err != nil {
		return len(ids), marked, err
	}
	seriesIDs := slices.Collect(maps.Values(held))
	if _, err := s.seasons.MarkStaleBySeries(ctx, seriesIDs); err != nil {
		return len(ids), marked, err
	}
	_, err = s.episodes.MarkStaleBySeries(ctx, seriesIDs)
	return len(ids), marked, err
}

// mediaType is the provider's name for an entity, which is how its changes feed is addressed.
func mediaType(entity Entity) (tmdb.MediaType, error) {
	switch entity {
	case EntityMovie:
		return tmdb.MediaTypeMovie, nil
	case EntitySeries:
		return tmdb.MediaTypeTV, nil
	case EntityPerson:
		return tmdb.MediaTypePerson, nil
	}
	return "", fmt.Errorf("catalog: no changes feed for entity %s", entity)
}
