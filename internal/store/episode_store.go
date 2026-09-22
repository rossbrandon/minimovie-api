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

const episodeColumns = `id, series_id, season_number, episode_number, source_id, name, payload, stale, fetched_at, created_at, updated_at`

type Episode struct {
	ID            int             `db:"id"`
	SeriesID      int             `db:"series_id"` // series.id
	SeasonNumber  int             `db:"season_number"`
	EpisodeNumber int             `db:"episode_number"`
	SourceID      int             `db:"source_id"`
	Name          string          `db:"name"`
	Payload       json.RawMessage `db:"payload"` // nil until the episode document is fetched
	Stale         bool            `db:"stale"`
	FetchedAt     *time.Time      `db:"fetched_at"`
	CreatedAt     time.Time       `db:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at"`
}

type EpisodeSkeleton struct {
	EpisodeNumber int
	SourceID      int
	Name          string
}

type EpisodeStore struct {
	pool *pgxpool.Pool
}

func NewEpisodeStore(pool *pgxpool.Pool) *EpisodeStore {
	return &EpisodeStore{pool: pool}
}

func (s *EpisodeStore) TableName() string {
	return "episodes"
}

func (s *EpisodeStore) Get(ctx context.Context, seriesID, seasonNumber, episodeNumber int) (*Episode, error) {
	defer metrics.TrackDbDuration(ctx, "episodes.read")()

	rows, err := s.pool.Query(ctx, `select `+episodeColumns+` from episodes where series_id = $1 and season_number = $2 and episode_number = $3`,
		seriesID, seasonNumber, episodeNumber)
	if err != nil {
		return nil, fmt.Errorf("episode store: get: %w", err)
	}
	ep, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[Episode])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("episode store: get: %w", err)
	}
	return ep, nil
}

func (s *EpisodeStore) IDsBySeason(ctx context.Context, seriesID, seasonNumber int) (map[int]int, error) {
	defer metrics.TrackDbDuration(ctx, "episodes.read")()

	rows, err := s.pool.Query(ctx, `select episode_number, id from episodes where series_id = $1 and season_number = $2`,
		seriesID, seasonNumber)
	if err != nil {
		return nil, fmt.Errorf("episode store: ids by season: %w", err)
	}
	ids, err := collectIDMap(rows)
	if err != nil {
		return nil, fmt.Errorf("episode store: ids by season: %w", err)
	}
	return ids, nil
}

func (s *EpisodeStore) UpsertSkeletons(ctx context.Context, db DBTX, seriesID, seasonNumber int, rows []EpisodeSkeleton) error {
	if len(rows) == 0 {
		return nil
	}
	defer metrics.TrackDbDuration(ctx, "episodes.write")()

	numbers := make([]int32, len(rows))
	sourceIDs := make([]int32, len(rows))
	names := make([]string, len(rows))
	for i, r := range rows {
		numbers[i] = int32(r.EpisodeNumber)
		sourceIDs[i] = int32(r.SourceID)
		names[i] = r.Name
	}

	_, err := db.Exec(ctx, `delete from episodes where series_id = $1 and season_number = $2 and source_id <> all($3::int[])`,
		seriesID, seasonNumber, sourceIDs)
	if err != nil {
		return fmt.Errorf("episode store: drop unlisted episodes for %d/%d: %w", seriesID, seasonNumber, err)
	}
	_, err = db.Exec(ctx, `
		insert into episodes (series_id, season_number, episode_number, source_id, name, updated_at)
		select $1, $2, u.number, u.source_id, u.name, now()
		from unnest($3::int[], $4::int[], $5::text[]) as u(number, source_id, name)
		on conflict (source_id) do update set
			episode_number = excluded.episode_number,
			name           = case when episodes.payload is null then excluded.name else episodes.name end,
			updated_at     = now()`, seriesID, seasonNumber, numbers, sourceIDs, names)
	if err != nil {
		return fmt.Errorf("episode store: upsert skeletons for %d/%d: %w", seriesID, seasonNumber, err)
	}
	return nil
}

func (s *EpisodeStore) Upsert(ctx context.Context, db DBTX, ep Episode) (int, error) {
	defer metrics.TrackDbDuration(ctx, "episodes.write")()

	id, err := scanID(db.QueryRow(ctx, `
		insert into episodes (series_id, season_number, episode_number, source_id, name, payload, stale, fetched_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, false, now(), now())
		on conflict (source_id) do update set
			episode_number = excluded.episode_number, name = excluded.name, payload = excluded.payload,
			stale = false, fetched_at = now(), updated_at = now()
		returning id`,
		ep.SeriesID, ep.SeasonNumber, ep.EpisodeNumber, ep.SourceID, ep.Name, ep.Payload))
	if err != nil {
		return 0, fmt.Errorf("episode store: upsert %d/%d/%d: %w", ep.SeriesID, ep.SeasonNumber, ep.EpisodeNumber, err)
	}
	return id, nil
}

func (s *EpisodeStore) MarkStaleBySeries(ctx context.Context, seriesIDs []int) (int64, error) {
	if len(seriesIDs) == 0 {
		return 0, nil
	}
	defer metrics.TrackDbDuration(ctx, "episodes.write")()

	tag, err := s.pool.Exec(ctx, `update episodes set stale = true, updated_at = now()
		where series_id = any($1) and payload is not null and not stale`, seriesIDs)
	if err != nil {
		return 0, fmt.Errorf("episode store: mark stale: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *EpisodeStore) DeleteExpired(ctx context.Context) (int64, error) {
	return purgeExpired(ctx, s.pool, "episodes")
}

func (s *EpisodeStore) Delete(ctx context.Context, db DBTX, seriesID, seasonNumber, episodeNumber int) error {
	defer metrics.TrackDbDuration(ctx, "episodes.write")()

	if _, err := db.Exec(ctx, `delete from episodes where series_id = $1 and season_number = $2 and episode_number = $3`,
		seriesID, seasonNumber, episodeNumber); err != nil {
		return fmt.Errorf("episode store: delete %d/%d/%d: %w", seriesID, seasonNumber, episodeNumber, err)
	}
	return nil
}
