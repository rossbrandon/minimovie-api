package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type InterestingInfoStore struct {
	pool *pgxpool.Pool
}

func NewInterestingInfoStore(pool *pgxpool.Pool) *InterestingInfoStore {
	return &InterestingInfoStore{pool: pool}
}

func (s *InterestingInfoStore) Get(ctx context.Context, entityType string, entityID int) (json.RawMessage, time.Time, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	query := `
		select data, fetched_at
		from interesting_info
		where entity_type = $1 and entity_id = $2
	`

	var data json.RawMessage
	var fetchedAt time.Time
	err := s.pool.QueryRow(ctx, query, entityType, entityID).Scan(&data, &fetchedAt)
	if err != nil {
		return nil, time.Time{}, err
	}

	return data, fetchedAt, nil
}

func (s *InterestingInfoStore) Set(ctx context.Context, entityType string, entityID int, name string, data json.RawMessage) error {
	defer metrics.TrackDbDuration(ctx, "write")()

	query := `
		insert into interesting_info (entity_type, entity_id, name, data, fetched_at, updated_at)
		values ($1, $2, $3, $4, now(), now())
		on conflict (entity_type, entity_id) do update set
			name = excluded.name,
			data = excluded.data,
			fetched_at = now(),
			updated_at = now()
	`

	_, err := s.pool.Exec(ctx, query, entityType, entityID, name, data)
	return err
}
