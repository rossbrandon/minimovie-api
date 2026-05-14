package store

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type SeriesMetadata struct {
	SeriesID            int
	Name                string
	TotalEpisodes       int
	TotalSeasons        int
	SeasonEpisodeCounts map[int]int
	InProduction        bool
	Status              string
	LastAirDate         string
	NextAirDate         string
	Fetched             bool
	FetchedAt           time.Time
}

type SeriesMetadataStore struct {
	pool *pgxpool.Pool
}

func NewSeriesMetadataStore(pool *pgxpool.Pool) *SeriesMetadataStore {
	return &SeriesMetadataStore{pool: pool}
}

func (s *SeriesMetadataStore) Get(ctx context.Context, seriesID int) (*SeriesMetadata, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	var meta SeriesMetadata
	var status pgtype.Text
	var lastAir, nextAir pgtype.Date
	var seasonCounts []byte
	err := s.pool.QueryRow(ctx, `
		select series_id, name, total_episodes, total_seasons, season_episode_counts,
		       in_production, status, last_air_date, next_air_date, fetched, fetched_at
		from series_metadata
		where series_id = $1
	`, seriesID).Scan(
		&meta.SeriesID, &meta.Name, &meta.TotalEpisodes, &meta.TotalSeasons,
		&seasonCounts, &meta.InProduction, &status, &lastAir, &nextAir,
		&meta.Fetched, &meta.FetchedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	meta.SeasonEpisodeCounts = decodeSeasonCounts(seasonCounts)
	if status.Valid {
		meta.Status = status.String
	}
	if lastAir.Valid {
		meta.LastAirDate = lastAir.Time.Format(time.DateOnly)
	}
	if nextAir.Valid {
		meta.NextAirDate = nextAir.Time.Format(time.DateOnly)
	}
	return &meta, nil
}

func (s *SeriesMetadataStore) Upsert(ctx context.Context, meta *SeriesMetadata) error {
	defer metrics.TrackDbDuration(ctx, "write")()

	seasonCounts, err := encodeSeasonCounts(meta.SeasonEpisodeCounts)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		insert into series_metadata
			(series_id, name, total_episodes, total_seasons, season_episode_counts,
			 in_production, status, last_air_date, next_air_date, fetched, fetched_at)
		values ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, true, now())
		on conflict (series_id) do update set
			name = excluded.name,
			total_episodes = excluded.total_episodes,
			total_seasons = excluded.total_seasons,
			season_episode_counts = excluded.season_episode_counts,
			in_production = excluded.in_production,
			status = excluded.status,
			last_air_date = excluded.last_air_date,
			next_air_date = excluded.next_air_date,
			fetched = true,
			fetched_at = now()
	`,
		meta.SeriesID, meta.Name, meta.TotalEpisodes, meta.TotalSeasons, seasonCounts,
		meta.InProduction, nullableString(meta.Status),
		nullableDate(meta.LastAirDate), nullableDate(meta.NextAirDate),
	)
	return err
}

func encodeSeasonCounts(counts map[int]int) ([]byte, error) {
	if len(counts) == 0 {
		return []byte("{}"), nil
	}
	// JSON object keys must be strings; serialize season numbers as strings.
	stringKeyed := make(map[string]int, len(counts))
	for season, episodes := range counts {
		stringKeyed[strconv.Itoa(season)] = episodes
	}
	return json.Marshal(stringKeyed)
}

func decodeSeasonCounts(raw []byte) map[int]int {
	if len(raw) == 0 {
		return nil
	}
	var stringKeyed map[string]int
	if err := json.Unmarshal(raw, &stringKeyed); err != nil {
		return nil
	}
	if len(stringKeyed) == 0 {
		return nil
	}
	result := make(map[int]int, len(stringKeyed))
	for k, v := range stringKeyed {
		season, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		result[season] = v
	}
	return result
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableDate(s string) any {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return nil
	}
	return t
}
