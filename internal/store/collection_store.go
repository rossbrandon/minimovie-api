package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

const collectionColumns = `id, source_id, slug, name, payload, fetched_at, created_at, updated_at`

type Collection struct {
	ID        int             `db:"id"`
	SourceID  int             `db:"source_id"`
	Slug      string          `db:"slug"`
	Name      string          `db:"name"`
	Payload   json.RawMessage `db:"payload"`
	FetchedAt time.Time       `db:"fetched_at"`
	CreatedAt time.Time       `db:"created_at"`
	UpdatedAt time.Time       `db:"updated_at"`
}

type CollectionStore struct {
	pool *pgxpool.Pool
}

func NewCollectionStore(pool *pgxpool.Pool) *CollectionStore {
	return &CollectionStore{pool: pool}
}

func (s *CollectionStore) TableName() string {
	return "collections"
}

func (s *CollectionStore) Get(ctx context.Context, id int) (*Collection, error) {
	return s.getOne(ctx, `where id = $1`, id)
}

func (s *CollectionStore) GetBySourceID(ctx context.Context, sourceID int) (*Collection, error) {
	return s.getOne(ctx, `where source_id = $1`, sourceID)
}

func (s *CollectionStore) Upsert(ctx context.Context, db DBTX, c Collection) (int, error) {
	defer metrics.TrackDbDuration(ctx, "collections.write")()

	id, err := scanID(db.QueryRow(ctx, `
		insert into collections (source_id, name, payload, fetched_at, updated_at)
		values ($1, $2, $3, now(), now())
		on conflict (source_id) do update set name = excluded.name, payload = excluded.payload, fetched_at = now(), updated_at = now()
		returning id`,
		c.SourceID, c.Name, c.Payload))
	if err != nil {
		return 0, fmt.Errorf("collection store: upsert source %d: %w", c.SourceID, err)
	}
	return id, nil
}

func (s *CollectionStore) DeleteExpired(ctx context.Context) (int64, error) {
	return purgeExpired(ctx, s.pool, "collections")
}

func (s *CollectionStore) getOne(ctx context.Context, where string, arg any) (*Collection, error) {
	defer metrics.TrackDbDuration(ctx, "collections.read")()

	rows, err := s.pool.Query(ctx, `select `+collectionColumns+` from collections `+where, arg)
	if err != nil {
		return nil, fmt.Errorf("collection store: get: %w", err)
	}
	c, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[Collection])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("collection store: get: %w", err)
	}
	return c, nil
}
