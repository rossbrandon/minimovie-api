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

const seriesColumns = `id, source_id, slug, name, original_name, overview, first_air_date, next_air_date, in_production,
	total_seasons, total_episodes, episode_run_time, popularity, vote_average, poster_path, genres, payload,
	stale, fetched_at, created_at, updated_at`

type Series struct {
	ID             int             `db:"id"`
	SourceID       int             `db:"source_id"`
	Slug           string          `db:"slug"`
	Name           string          `db:"name"`
	OriginalName   *string         `db:"original_name"`
	Overview       *string         `db:"overview"`
	FirstAirDate   *time.Time      `db:"first_air_date"`
	NextAirDate    *time.Time      `db:"next_air_date"`
	InProduction   *bool           `db:"in_production"`  // nil until hydrated
	TotalSeasons   *int            `db:"total_seasons"`  // nil until hydrated
	TotalEpisodes  *int            `db:"total_episodes"` // nil until hydrated; 0 would mark a series watched after one episode
	EpisodeRunTime *int            `db:"episode_run_time"`
	Popularity     float64         `db:"popularity"`
	VoteAverage    *float64        `db:"vote_average"`
	PosterPath     *string         `db:"poster_path"`
	Genres         []string        `db:"genres"`
	Payload        json.RawMessage `db:"payload"`
	Stale          bool            `db:"stale"`
	FetchedAt      *time.Time      `db:"fetched_at"`
	CreatedAt      time.Time       `db:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"`
}

type SeriesStore struct {
	catalogTable
}

func NewSeriesStore(pool *pgxpool.Pool) *SeriesStore {
	return &SeriesStore{catalogTable{pool: pool, name: "series"}}
}

func (s *SeriesStore) GetByID(ctx context.Context, id int) (*Series, error) {
	return s.getOne(ctx, `where id = $1`, id)
}

func (s *SeriesStore) GetBySourceID(ctx context.Context, sourceID int) (*Series, error) {
	return s.getOne(ctx, `where source_id = $1`, sourceID)
}

func (s *SeriesStore) UpsertHydrated(ctx context.Context, db DBTX, sr Series) (int, error) {
	defer metrics.TrackDbDuration(ctx, "write")()
	if sr.Genres == nil {
		sr.Genres = []string{}
	}

	id, err := scanID(db.QueryRow(ctx, `
		insert into series (source_id, name, original_name, overview, first_air_date, next_air_date, in_production,
		                    total_seasons, total_episodes, episode_run_time, popularity, vote_average, poster_path,
		                    genres, payload, stale, fetched_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, false, now(), now())
		on conflict (source_id) do update set
			name             = excluded.name,
			original_name    = excluded.original_name,
			overview         = excluded.overview,
			first_air_date   = excluded.first_air_date,
			next_air_date    = excluded.next_air_date,
			in_production    = excluded.in_production,
			total_seasons    = excluded.total_seasons,
			total_episodes   = excluded.total_episodes,
			episode_run_time = excluded.episode_run_time,
			popularity       = excluded.popularity,
			vote_average     = excluded.vote_average,
			poster_path      = excluded.poster_path,
			genres           = excluded.genres,
			payload          = excluded.payload,
			stale            = false,
			fetched_at       = now(),
			updated_at       = now()
		returning id`,
		sr.SourceID, sr.Name, sr.OriginalName, sr.Overview, nullTime(sr.FirstAirDate), nullTime(sr.NextAirDate), sr.InProduction,
		sr.TotalSeasons, sr.TotalEpisodes, sr.EpisodeRunTime, sr.Popularity, sr.VoteAverage, sr.PosterPath,
		sr.Genres, sr.Payload))
	if err != nil {
		return 0, fmt.Errorf("series store: upsert source %d: %w", sr.SourceID, err)
	}
	return id, nil
}

func (s *SeriesStore) UpsertSkeleton(ctx context.Context, db DBTX, rows []Skeleton) error {
	if len(rows) == 0 {
		return nil
	}
	defer metrics.TrackDbDuration(ctx, "write")()

	cols := skeletonColumns(rows)
	_, err := db.Exec(ctx, `
		insert into series (source_id, name, original_name, overview, poster_path, first_air_date, vote_average, popularity, updated_at)
		select u.source_id, u.name, u.name, u.overview, u.poster, u.first_air_date::date, u.vote, u.popularity, now()
		from unnest($1::int[], $2::text[], $3::text[], $4::text[], $5::text[], $6::real[], $7::real[])
			as u(source_id, name, overview, poster, first_air_date, vote, popularity)
		on conflict (source_id) do update set
			popularity     = case when series.payload is null and excluded.popularity > 0
			                 then excluded.popularity else series.popularity end,
			name           = case when series.payload is null then excluded.name else series.name end,
			overview       = case when series.payload is null then coalesce(excluded.overview, series.overview) else series.overview end,
			poster_path    = case when series.payload is null then coalesce(excluded.poster_path, series.poster_path) else series.poster_path end,
			first_air_date = case when series.payload is null then coalesce(excluded.first_air_date, series.first_air_date) else series.first_air_date end,
			vote_average   = case when series.payload is null then coalesce(excluded.vote_average, series.vote_average) else series.vote_average end,
			updated_at     = now()`,
		cols.sourceIDs, cols.titles, cols.overviews, cols.posters, cols.dates, cols.votes, cols.popularity)
	if err != nil {
		return fmt.Errorf("series store: upsert skeletons: %w", err)
	}
	return nil
}

func (s *SeriesStore) getOne(ctx context.Context, where string, arg any) (*Series, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	rows, err := s.pool.Query(ctx, `select `+seriesColumns+` from series `+where, arg)
	if err != nil {
		return nil, fmt.Errorf("series store: get: %w", err)
	}
	sr, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[Series])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("series store: get: %w", err)
	}
	return sr, nil
}
