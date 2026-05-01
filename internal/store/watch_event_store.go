package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type WatchEvent struct {
	ID             string     `json:"id"`
	UserID         string     `json:"-"`
	MediaType      string     `json:"mediaType"`
	MediaID        int        `json:"mediaId"`
	MediaTitle     string     `json:"mediaTitle"`
	SeriesID       *int       `json:"seriesId,omitempty"`
	SeriesTitle    *string    `json:"seriesTitle,omitempty"`
	SeasonNumber   *int       `json:"seasonNumber,omitempty"`
	EpisodeNumber  *int       `json:"episodeNumber,omitempty"`
	EpisodeCount   *int       `json:"episodeCount,omitempty"`
	SeasonCount    *int       `json:"seasonCount,omitempty"`
	WatchedAt      *time.Time `json:"watchedAt,omitempty"`
	Timezone       string     `json:"timezone"`
	RewatchNumber  int        `json:"rewatchNumber"`
	RuntimeMinutes *int       `json:"runtimeMinutes,omitempty"`
	Genres         []string   `json:"genres"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type WatchEventRepository interface {
	List(ctx context.Context, userID string, filters WatchEventFilters) ([]WatchEvent, error)
	ListBySeriesID(ctx context.Context, userID string, seriesID int) ([]WatchEvent, error)
	GetByID(ctx context.Context, id, userID string) (*WatchEvent, error)
	Create(ctx context.Context, input WatchEventCreate) (*WatchEvent, error)
	Delete(ctx context.Context, id, userID string) error
}

type WatchEventStore struct {
	pool *pgxpool.Pool
}

func NewWatchEventStore(pool *pgxpool.Pool) *WatchEventStore {
	return &WatchEventStore{pool: pool}
}

type WatchEventCreate struct {
	ID            string
	UserID        string
	MediaType     string
	MediaID       int
	SeriesID      *int
	SeasonNumber  *int
	EpisodeNumber *int
	EpisodeCount  *int
	SeasonCount   *int
	WatchedAt     *time.Time
	Timezone      string
	Meta          ResolvedMedia
}

type WatchEventFilters struct {
	MediaType *string
	MediaID   *int
	SeriesID  *int
	DatedOnly bool
	Limit     int
}

func (s *WatchEventStore) List(ctx context.Context, userID string, filters WatchEventFilters) ([]WatchEvent, error) {
	defer metrics.TrackDbDuration(ctx, "watch_events.list")()
	limit := filters.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	columns := `id, media_type, media_id, media_title,
		series_id, series_title, season_number, episode_number, episode_count, season_count,
		watched_at, timezone, rewatch_number, runtime_minutes, genres, created_at`

	query := `select ` + columns + ` from watch_event where user_id = $1`
	args := []any{userID}
	argIdx := 2

	if filters.MediaType != nil {
		query += fmt.Sprintf(` and media_type = $%d`, argIdx)
		args = append(args, *filters.MediaType)
		argIdx++
	}
	if filters.MediaID != nil {
		query += fmt.Sprintf(` and media_id = $%d`, argIdx)
		args = append(args, *filters.MediaID)
		argIdx++
	}
	if filters.SeriesID != nil {
		query += fmt.Sprintf(` and series_id = $%d`, argIdx)
		args = append(args, *filters.SeriesID)
		argIdx++
	}
	if filters.DatedOnly {
		query += ` and watched_at is not null`
	}

	query += fmt.Sprintf(` order by created_at desc limit $%d`, argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []WatchEvent
	for rows.Next() {
		var ev WatchEvent
		if err := rows.Scan(
			&ev.ID, &ev.MediaType, &ev.MediaID, &ev.MediaTitle,
			&ev.SeriesID, &ev.SeriesTitle, &ev.SeasonNumber, &ev.EpisodeNumber,
			&ev.EpisodeCount, &ev.SeasonCount,
			&ev.WatchedAt, &ev.Timezone, &ev.RewatchNumber,
			&ev.RuntimeMinutes, &ev.Genres, &ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *WatchEventStore) ListBySeriesID(ctx context.Context, userID string, seriesID int) ([]WatchEvent, error) {
	defer metrics.TrackDbDuration(ctx, "watch_events.list_by_series_id")()
	query := `select id, media_type, media_id, media_title,
		series_id, series_title, season_number, episode_number, episode_count, season_count,
		watched_at, timezone, rewatch_number, runtime_minutes, genres, created_at
		from watch_event
		where user_id = $1 and series_id = $2
		order by season_number, episode_number, created_at`
	rows, err := s.pool.Query(ctx, query, userID, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []WatchEvent
	for rows.Next() {
		var ev WatchEvent
		if err := rows.Scan(
			&ev.ID, &ev.MediaType, &ev.MediaID, &ev.MediaTitle,
			&ev.SeriesID, &ev.SeriesTitle, &ev.SeasonNumber, &ev.EpisodeNumber,
			&ev.EpisodeCount, &ev.SeasonCount,
			&ev.WatchedAt, &ev.Timezone, &ev.RewatchNumber,
			&ev.RuntimeMinutes, &ev.Genres, &ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *WatchEventStore) GetByID(ctx context.Context, id, userID string) (*WatchEvent, error) {
	defer metrics.TrackDbDuration(ctx, "watch_events.get_by_id")()
	query := `select id, media_type, media_id, media_title,
		series_id, series_title, season_number, episode_number,
		episode_count, season_count, watched_at, timezone, 
		rewatch_number, runtime_minutes, genres, created_at
		from watch_event where id = $1 and user_id = $2`
	var ev WatchEvent
	err := s.pool.QueryRow(ctx, query, id, userID).Scan(
		&ev.ID, &ev.MediaType, &ev.MediaID, &ev.MediaTitle,
		&ev.SeriesID, &ev.SeriesTitle, &ev.SeasonNumber, &ev.EpisodeNumber,
		&ev.EpisodeCount, &ev.SeasonCount, &ev.WatchedAt, &ev.Timezone,
		&ev.RewatchNumber, &ev.RuntimeMinutes, &ev.Genres, &ev.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ev, nil
}

func (s *WatchEventStore) Create(ctx context.Context, input WatchEventCreate) (*WatchEvent, error) {
	defer metrics.TrackDbDuration(ctx, "watch_events.create")()

	var ev *WatchEvent
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var rewatchNumber int
		if err := tx.QueryRow(ctx,
			`select coalesce(max(rewatch_number), 0) + 1 from watch_event
			 where user_id = $1 and media_type = $2 and media_id = $3`,
			input.UserID, input.MediaType, input.MediaID,
		).Scan(&rewatchNumber); err != nil {
			return err
		}

		genres := input.Meta.Genres
		if genres == nil {
			genres = []string{}
		}

		var idOverride *string
		if input.ID != "" {
			idOverride = &input.ID
		}

		ev = &WatchEvent{}
		return tx.QueryRow(ctx,
			`insert into watch_event (
				id, user_id, media_type, media_id, media_title,
				series_id, series_title, season_number, episode_number,
				episode_count, season_count, watched_at, timezone,
				rewatch_number, runtime_minutes, genres
			) values (coalesce($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			returning id, media_type, media_id, media_title,
				series_id, series_title, season_number, episode_number,
				episode_count, season_count, watched_at, timezone,
				rewatch_number, runtime_minutes, genres, created_at`,
			idOverride, input.UserID, input.MediaType, input.MediaID, input.Meta.Title,
			input.SeriesID, input.Meta.SeriesTitle, input.SeasonNumber, input.EpisodeNumber,
			input.EpisodeCount, input.SeasonCount, input.WatchedAt, input.Timezone,
			rewatchNumber, input.Meta.RuntimeMinutes, genres,
		).Scan(
			&ev.ID, &ev.MediaType, &ev.MediaID, &ev.MediaTitle,
			&ev.SeriesID, &ev.SeriesTitle, &ev.SeasonNumber, &ev.EpisodeNumber,
			&ev.EpisodeCount, &ev.SeasonCount, &ev.WatchedAt, &ev.Timezone,
			&ev.RewatchNumber, &ev.RuntimeMinutes, &ev.Genres, &ev.CreatedAt,
		)
	})
	if err != nil {
		return nil, err
	}

	return ev, nil
}

func (s *WatchEventStore) Delete(ctx context.Context, id, userID string) error {
	defer metrics.TrackDbDuration(ctx, "watch_events.delete")()
	tag, err := s.pool.Exec(ctx, `delete from watch_event where id = $1 and user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
