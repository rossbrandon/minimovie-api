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

const seasonColumns = `id, series_id, season_number, source_id, name, payload, stale, fetched_at, created_at, updated_at`

type Season struct {
	ID           int             `db:"id"`
	SeriesID     int             `db:"series_id"` // series.id
	SeasonNumber int             `db:"season_number"`
	SourceID     int             `db:"source_id"`
	Name         string          `db:"name"`
	Payload      json.RawMessage `db:"payload"` // nil until the season document is fetched
	Stale        bool            `db:"stale"`
	FetchedAt    *time.Time      `db:"fetched_at"`
	CreatedAt    time.Time       `db:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at"`
}

type SeasonSkeleton struct {
	SeasonNumber int
	SourceID     int
	Name         string
}

type SeasonStore struct {
	pool *pgxpool.Pool
}

func NewSeasonStore(pool *pgxpool.Pool) *SeasonStore {
	return &SeasonStore{pool: pool}
}

func (s *SeasonStore) TableName() string {
	return "seasons"
}

func (s *SeasonStore) Get(ctx context.Context, seriesID, seasonNumber int) (*Season, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	rows, err := s.pool.Query(ctx, `select `+seasonColumns+` from seasons where series_id = $1 and season_number = $2`, seriesID, seasonNumber)
	if err != nil {
		return nil, fmt.Errorf("season store: get: %w", err)
	}
	season, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[Season])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("season store: get: %w", err)
	}
	return season, nil
}

func (s *SeasonStore) ListBySeries(ctx context.Context, seriesID int) ([]Season, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	rows, err := s.pool.Query(ctx, `select `+seasonColumns+` from seasons where series_id = $1 order by season_number`, seriesID)
	if err != nil {
		return nil, fmt.Errorf("season store: list: %w", err)
	}
	seasons, err := pgx.CollectRows(rows, pgx.RowToStructByName[Season])
	if err != nil {
		return nil, fmt.Errorf("season store: list: %w", err)
	}
	return seasons, nil
}

func (s *SeasonStore) IDsBySeries(ctx context.Context, seriesID int) (map[int]int, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	rows, err := s.pool.Query(ctx, `select season_number, id from seasons where series_id = $1`, seriesID)
	if err != nil {
		return nil, fmt.Errorf("season store: ids by series: %w", err)
	}
	ids, err := collectIDMap(rows)
	if err != nil {
		return nil, fmt.Errorf("season store: ids by series: %w", err)
	}
	return ids, nil
}

func (s *SeasonStore) UpsertSkeletons(ctx context.Context, db DBTX, seriesID int, rows []SeasonSkeleton) error {
	if len(rows) == 0 {
		return nil
	}
	defer metrics.TrackDbDuration(ctx, "write")()

	numbers := make([]int32, len(rows))
	sourceIDs := make([]int32, len(rows))
	names := make([]string, len(rows))
	for i, r := range rows {
		numbers[i] = int32(r.SeasonNumber)
		sourceIDs[i] = int32(r.SourceID)
		names[i] = r.Name
	}
	_, err := db.Exec(ctx, `
		insert into seasons (series_id, season_number, source_id, name, updated_at)
		select $1, u.number, u.source_id, u.name, now()
		from unnest($2::int[], $3::int[], $4::text[]) as u(number, source_id, name)
		on conflict (source_id) do update set
			season_number = excluded.season_number,
			name          = case when seasons.payload is null then excluded.name else seasons.name end,
			updated_at    = now()`, seriesID, numbers, sourceIDs, names)
	if err != nil {
		return fmt.Errorf("season store: upsert skeletons for series %d: %w", seriesID, err)
	}
	return nil
}

func (s *SeasonStore) Upsert(ctx context.Context, db DBTX, season Season) (int, error) {
	defer metrics.TrackDbDuration(ctx, "write")()

	id, err := scanID(db.QueryRow(ctx, `
		insert into seasons (series_id, season_number, source_id, name, payload, stale, fetched_at, updated_at)
		values ($1, $2, $3, $4, $5, false, now(), now())
		on conflict (source_id) do update set
			season_number = excluded.season_number, name = excluded.name, payload = excluded.payload,
			stale = false, fetched_at = now(), updated_at = now()
		returning id`,
		season.SeriesID, season.SeasonNumber, season.SourceID, season.Name, season.Payload))
	if err != nil {
		return 0, fmt.Errorf("season store: upsert %d/%d: %w", season.SeriesID, season.SeasonNumber, err)
	}
	return id, nil
}

func (s *SeasonStore) MarkStaleBySeries(ctx context.Context, seriesIDs []int) (int64, error) {
	if len(seriesIDs) == 0 {
		return 0, nil
	}
	defer metrics.TrackDbDuration(ctx, "write")()

	tag, err := s.pool.Exec(ctx, `update seasons set stale = true, updated_at = now()
		where series_id = any($1) and payload is not null and not stale`, seriesIDs)
	if err != nil {
		return 0, fmt.Errorf("season store: mark stale: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *SeasonStore) DeleteExpired(ctx context.Context) (int64, error) {
	return purgeExpired(ctx, s.pool, "seasons")
}

func (s *SeasonStore) Delete(ctx context.Context, db DBTX, seriesID, seasonNumber int) error {
	defer metrics.TrackDbDuration(ctx, "write")()

	if _, err := db.Exec(ctx, `delete from seasons where series_id = $1 and season_number = $2`, seriesID, seasonNumber); err != nil {
		return fmt.Errorf("season store: delete %d/%d: %w", seriesID, seasonNumber, err)
	}
	return nil
}
