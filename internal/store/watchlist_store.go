package store

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

type WatchlistItem struct {
	ID                  string     `json:"id"`
	UserID              string     `json:"-"`
	MediaType           string     `json:"mediaType"`
	MediaID             int        `json:"mediaId"`
	MediaTitle          string     `json:"mediaTitle"`
	PosterPath          *string    `json:"posterPath"`
	Status              string     `json:"status"`
	StartedAt           *time.Time `json:"startedAt,omitempty"`
	FinishedAt          *time.Time `json:"finishedAt,omitempty"`
	LastWatchedAt       *time.Time `json:"lastWatchedAt,omitempty"`
	WatchCount          int        `json:"watchCount"`
	Genres              []string   `json:"genres"`
	RuntimeMinutes      *int       `json:"runtimeMinutes,omitempty"`
	VoteAverage         *float32   `json:"voteAverage,omitempty"`
	ReleaseYear         *int       `json:"releaseYear,omitempty"`
	MetadataRefreshedAt time.Time  `json:"-"`
	AddedAt             time.Time  `json:"addedAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

type ResolvedMedia struct {
	Title          string
	MediaID        *int
	PosterPath     *string
	Genres         []string
	RuntimeMinutes *int
	VoteAverage    *float32
	ReleaseYear    *int
	SeriesTitle    *string
	EpisodeCount   *int
	SeasonCount    *int
}

type WatchlistRepository interface {
	List(ctx context.Context, userID string, status, mediaType *string) ([]WatchlistItem, error)
	Check(ctx context.Context, userID, mediaType string, mediaID int) (*WatchlistItem, error)
	Create(ctx context.Context, userID, mediaType string, mediaID int, status string, meta ResolvedMedia) (*WatchlistItem, error)
	UpdateStatus(ctx context.Context, id, userID, status string) (*WatchlistItem, error)
	UpdateSummary(ctx context.Context, userID, mediaType string, mediaID int) error
	Delete(ctx context.Context, id, userID string) error
}

type WatchlistStore struct {
	pool *pgxpool.Pool
}

func NewWatchlistStore(pool *pgxpool.Pool) *WatchlistStore {
	return &WatchlistStore{pool: pool}
}

func (s *WatchlistStore) List(ctx context.Context, userID string, status, mediaType *string) ([]WatchlistItem, error) {
	defer metrics.TrackDbDuration(ctx, "watchlist.list")()
	query := `select id, media_type, media_id, media_title, poster_path, status,
	                 started_at, finished_at, last_watched_at, watch_count,
	                 genres, runtime_minutes, vote_average, release_year,
	                 added_at, updated_at
	          from watchlist_item where user_id = $1`
	args := []any{userID}

	if status != nil {
		args = append(args, *status)
		query += ` and status = $` + strconv.Itoa(len(args))
	}
	if mediaType != nil {
		args = append(args, *mediaType)
		query += ` and media_type = $` + strconv.Itoa(len(args))
	}
	query += ` order by updated_at desc`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []WatchlistItem
	for rows.Next() {
		var item WatchlistItem
		if err := rows.Scan(
			&item.ID, &item.MediaType, &item.MediaID, &item.MediaTitle,
			&item.PosterPath, &item.Status, &item.StartedAt, &item.FinishedAt,
			&item.LastWatchedAt, &item.WatchCount, &item.Genres,
			&item.RuntimeMinutes, &item.VoteAverage, &item.ReleaseYear,
			&item.AddedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *WatchlistStore) Check(ctx context.Context, userID, mediaType string, mediaID int) (*WatchlistItem, error) {
	defer metrics.TrackDbDuration(ctx, "watchlist.check")()
	query := `select id, status from watchlist_item where user_id = $1 and media_type = $2 and media_id = $3`
	var item WatchlistItem
	err := s.pool.QueryRow(ctx, query, userID, mediaType, mediaID).Scan(&item.ID, &item.Status)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *WatchlistStore) Create(ctx context.Context, userID, mediaType string, mediaID int, status string, meta ResolvedMedia) (*WatchlistItem, error) {
	defer metrics.TrackDbDuration(ctx, "watchlist.create")()
	query := `
		insert into watchlist_item (user_id, media_type, media_id, media_title, poster_path, status,
		                            genres, runtime_minutes, vote_average, release_year)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		returning id, media_type, media_id, media_title, poster_path, status,
		          watch_count, genres, runtime_minutes, vote_average, release_year, added_at, updated_at
	`
	var item WatchlistItem
	err := s.pool.QueryRow(ctx, query,
		userID, mediaType, mediaID, meta.Title, meta.PosterPath, status,
		meta.Genres, meta.RuntimeMinutes, meta.VoteAverage, meta.ReleaseYear,
	).Scan(
		&item.ID, &item.MediaType, &item.MediaID, &item.MediaTitle,
		&item.PosterPath, &item.Status, &item.WatchCount,
		&item.Genres, &item.RuntimeMinutes, &item.VoteAverage, &item.ReleaseYear,
		&item.AddedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *WatchlistStore) UpdateStatus(ctx context.Context, id, userID, status string) (*WatchlistItem, error) {
	defer metrics.TrackDbDuration(ctx, "watchlist.update_status")()
	query := `
		update watchlist_item set status = $1, updated_at = now()
		where id = $2 and user_id = $3
		returning id, media_type, media_id, media_title, poster_path, status,
		          watch_count, genres, runtime_minutes, vote_average, release_year, added_at, updated_at
	`
	var item WatchlistItem
	err := s.pool.QueryRow(ctx, query, status, id, userID).Scan(
		&item.ID, &item.MediaType, &item.MediaID, &item.MediaTitle,
		&item.PosterPath, &item.Status, &item.WatchCount,
		&item.Genres, &item.RuntimeMinutes, &item.VoteAverage, &item.ReleaseYear,
		&item.AddedAt, &item.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *WatchlistStore) UpdateSummary(ctx context.Context, userID, mediaType string, mediaID int) error {
	defer metrics.TrackDbDuration(ctx, "watchlist.update_summary")()
	query := `
		update watchlist_item set
			watch_count = coalesce((
				select count(*) from watch_event
				where user_id = $1
				  and ((media_type = $2 and media_id = $3)
				       or (series_id = $3 and media_type in ('episode', 'season')))
			), 0),
			last_watched_at = (
				select max(coalesce(watched_at, created_at)) from watch_event
				where user_id = $1
				  and ((media_type = $2 and media_id = $3)
				       or (series_id = $3 and media_type in ('episode', 'season')))
			),
			status = case
				when (select count(*) from watch_event
				      where user_id = $1
				        and ((media_type = $2 and media_id = $3)
				             or (series_id = $3 and media_type in ('episode', 'season')))) > 0
				then 'watched'
				else status
			end,
			updated_at = now()
		where user_id = $1 and media_type = $2 and media_id = $3
	`
	_, err := s.pool.Exec(ctx, query, userID, mediaType, mediaID)
	return err
}

func (s *WatchlistStore) Delete(ctx context.Context, id, userID string) error {
	defer metrics.TrackDbDuration(ctx, "watchlist.delete")()
	tag, err := s.pool.Exec(ctx, `delete from watchlist_item where id = $1 and user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
