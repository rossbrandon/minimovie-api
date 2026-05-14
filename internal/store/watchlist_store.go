package store

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	EpisodesWatched     int        `json:"episodesWatched"`
	SeasonsWatched      int        `json:"seasonsWatched"`
	Genres              []string   `json:"genres"`
	RuntimeMinutes      *int       `json:"runtimeMinutes,omitempty"`
	VoteAverage         *float32   `json:"voteAverage,omitempty"`
	ReleaseYear         *int       `json:"releaseYear,omitempty"`
	MetadataRefreshedAt time.Time  `json:"-"`
	AddedAt             time.Time  `json:"addedAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	TotalEpisodes       *int       `json:"totalEpisodes,omitempty"`
	TotalSeasons        *int       `json:"totalSeasons,omitempty"`
	InProduction        *bool      `json:"inProduction,omitempty"`
	NextAirDate         *string    `json:"nextAirDate,omitempty"`
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
	// LEFT JOIN to series_metadata leaves movie rows with NULL totals.
	// Sort puts the most recent interaction first; id breaks timestamp ties.
	query := `select w.id, w.media_type, w.media_id, w.media_title, w.poster_path, w.status,
	                 w.started_at, w.finished_at, w.last_watched_at, w.watch_count,
	                 w.episodes_watched, w.seasons_watched,
	                 w.genres, w.runtime_minutes, w.vote_average, w.release_year,
	                 w.added_at, w.updated_at,
	                 sm.total_episodes, sm.total_seasons, sm.in_production, sm.next_air_date
	          from watchlist_item w
	          left join series_metadata sm on sm.series_id = w.media_id and w.media_type = 'series'
	          where w.user_id = $1`
	args := []any{userID}

	if status != nil {
		args = append(args, *status)
		query += ` and w.status = $` + strconv.Itoa(len(args))
	}
	if mediaType != nil {
		args = append(args, *mediaType)
		query += ` and w.media_type = $` + strconv.Itoa(len(args))
	}
	query += ` order by coalesce(w.last_watched_at, w.added_at) desc, w.id desc`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []WatchlistItem
	for rows.Next() {
		var item WatchlistItem
		var totalEp, totalSeason *int
		var inProd *bool
		var nextAir pgtype.Date
		if err := rows.Scan(
			&item.ID, &item.MediaType, &item.MediaID, &item.MediaTitle,
			&item.PosterPath, &item.Status, &item.StartedAt, &item.FinishedAt,
			&item.LastWatchedAt, &item.WatchCount,
			&item.EpisodesWatched, &item.SeasonsWatched,
			&item.Genres, &item.RuntimeMinutes, &item.VoteAverage, &item.ReleaseYear,
			&item.AddedAt, &item.UpdatedAt,
			&totalEp, &totalSeason, &inProd, &nextAir,
		); err != nil {
			return nil, err
		}
		item.TotalEpisodes = totalEp
		item.TotalSeasons = totalSeason
		item.InProduction = inProd
		if nextAir.Valid {
			formatted := nextAir.Time.Format(time.DateOnly)
			item.NextAirDate = &formatted
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

// UpdateSummary recomputes denormalized progress fields from watch_event.
// For series, episodes_watched = SUM(season.episode_count) + COUNT(episode events);
// seasons_watched counts only seasons the user has fully completed
// (season-mark OR episode-mark count ≥ season's cached episode_count).
// Status reverts to 'want_to_watch' when no events remain.
func (s *WatchlistStore) UpdateSummary(ctx context.Context, userID, mediaType string, mediaID int) error {
	defer metrics.TrackDbDuration(ctx, "watchlist.update_summary")()

	if mediaType == "series" {
		return s.updateSeriesSummary(ctx, userID, mediaID)
	}
	return s.updateMovieSummary(ctx, userID, mediaID)
}

// updateSeriesSummary recomputes denormalized progress fields from watch_event for series.
// season_totals expands the series_metadata season_episode_counts jsonb into one row per season;
// episode_counts counts the user's episode events per season;
// complete_seasons is the count of seasons that are fully watched (season-mark or enough episode marks).
func (s *WatchlistStore) updateSeriesSummary(ctx context.Context, userID string, seriesID int) error {
	query := `
		with totals as (
			select coalesce(sum(case when media_type = 'season' then episode_count else 0 end), 0)
				+ sum(case when media_type = 'episode' then 1 else 0 end) as episodes_watched,
				max(coalesce(watched_at, created_at)) as last_watched_at,
				count(*) as total_events
			from watch_event
			where user_id = $1 and series_id = $2 and media_type in ('season', 'episode')
		),
		season_totals as (
			select (kv.key)::int as season_number, (kv.value)::int as total
			from series_metadata sm,
			     jsonb_each_text(coalesce(sm.season_episode_counts, '{}'::jsonb)) as kv
			where sm.series_id = $2
		),
		episode_counts as (
			select season_number, count(*) as cnt
			from watch_event
			where user_id = $1 and series_id = $2 and media_type = 'episode'
			group by season_number
		),
		complete_seasons as (
			select count(*) as cnt
			from season_totals st
			where exists (
				select 1 from watch_event we
				where we.user_id = $1 and we.series_id = $2
				  and we.media_type = 'season' and we.season_number = st.season_number
			)
			or (st.total > 0
				and coalesce((select cnt from episode_counts ec where ec.season_number = st.season_number), 0) >= st.total)
		)
		update watchlist_item set
			episodes_watched = coalesce(totals.episodes_watched, 0)::int,
			seasons_watched = coalesce(complete_seasons.cnt, 0)::int,
			watch_count = coalesce(totals.episodes_watched, 0)::int,
			last_watched_at = totals.last_watched_at,
			status = case
				when coalesce(totals.total_events, 0) = 0 then 'want_to_watch'
				else 'watched'
			end,
			updated_at = now()
		from totals
		left join complete_seasons on true
		where watchlist_item.user_id = $1
		  and watchlist_item.media_type = 'series'
		  and watchlist_item.media_id = $2
	`
	_, err := s.pool.Exec(ctx, query, userID, seriesID)
	return err
}

// updateMovieSummary recomputes denormalized progress fields from watch_event for movies.
func (s *WatchlistStore) updateMovieSummary(ctx context.Context, userID string, movieID int) error {
	query := `
		with e as (
			select
				count(*) as cnt,
				max(coalesce(watched_at, created_at)) as last_watched_at
			from watch_event
			where user_id = $1 and media_type = 'movie' and media_id = $2
		)
		update watchlist_item set
			watch_count = coalesce(e.cnt, 0)::int,
			last_watched_at = e.last_watched_at,
			status = case when coalesce(e.cnt, 0) = 0 then 'want_to_watch' else 'watched' end,
			updated_at = now()
		from e
		where watchlist_item.user_id = $1
		  and watchlist_item.media_type = 'movie'
		  and watchlist_item.media_id = $2
	`
	_, err := s.pool.Exec(ctx, query, userID, movieID)
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
