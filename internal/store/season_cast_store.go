package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

type SeasonCastPostgresStore struct {
	pool *pgxpool.Pool
}

func NewSeasonCastPostgresStore(pool *pgxpool.Pool) *SeasonCastPostgresStore {
	return &SeasonCastPostgresStore{pool: pool}
}

func (s *SeasonCastPostgresStore) Get(ctx context.Context, seriesID, seasonNumber int) (map[int]int, bool) {
	defer metrics.TrackDbDuration(ctx, "season_cast_read")()

	query := `
		select cast_data
		from season_cast_cache
		where series_id = $1
			and season_number = $2
			and expires_at > now()
	`

	var raw json.RawMessage
	err := s.pool.QueryRow(ctx, query, seriesID, seasonNumber).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, false
	}
	if err != nil {
		log.Warn().Err(err).Int("series_id", seriesID).Int("season", seasonNumber).Msg("failed to read season cast from database")
		return nil, false
	}

	var stringMap map[string]int
	if err := json.Unmarshal(raw, &stringMap); err != nil {
		log.Warn().Err(err).Msg("failed to decode season cast data from database")
		return nil, false
	}

	castMap := make(map[int]int, len(stringMap))
	for k, v := range stringMap {
		id, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		castMap[id] = v
	}

	return castMap, true
}

func (s *SeasonCastPostgresStore) Set(ctx context.Context, seriesID, seasonNumber int, castMap map[int]int, expiresAt time.Time) {
	defer metrics.TrackDbDuration(ctx, "season_cast_write")()

	stringMap := make(map[string]int, len(castMap))
	for k, v := range castMap {
		stringMap[strconv.Itoa(k)] = v
	}

	data, err := json.Marshal(stringMap)
	if err != nil {
		log.Warn().Err(err).Int("series_id", seriesID).Int("season", seasonNumber).Msg("failed to encode season cast data for database")
		return
	}

	query := `
		insert into season_cast_cache (series_id, season_number, cast_data, expires_at)
		values ($1, $2, $3, $4)
		on conflict (series_id, season_number) do update set
			cast_data = excluded.cast_data,
			expires_at = excluded.expires_at,
			created_at = now()
	`

	if _, err := s.pool.Exec(ctx, query, seriesID, seasonNumber, data, expiresAt); err != nil {
		log.Warn().Err(err).Int("series_id", seriesID).Int("season", seasonNumber).Msg("failed to write season cast to database")
	}
}

func (s *SeasonCastPostgresStore) PurgeExpired(ctx context.Context) (int64, error) {
	defer metrics.TrackDbDuration(ctx, "season_cast_purge")()

	result, err := s.pool.Exec(ctx, `delete from season_cast_cache where expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("failed to purge season_cast_cache: %w", err)
	}

	count := result.RowsAffected()
	if metrics.M != nil {
		metrics.M.RecordDbPurge(ctx, "season_cast_cache", count)
	}

	return count, nil
}

func (s *SeasonCastPostgresStore) TableName() string {
	return "season_cast_cache"
}
