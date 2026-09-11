package catalog

import (
	"context"
	"fmt"

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
