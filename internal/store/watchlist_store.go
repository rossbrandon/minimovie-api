package store

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

const watchlistSelect = `
	select w.id, w.media_type, w.media_id,
	       coalesce(m.title, s.name, w.media_title) as media_title,
	       coalesce(m.poster_path, s.poster_path) as poster_path,
	       w.status, w.started_at, w.finished_at, w.last_watched_at,
	       w.watch_count, w.episodes_watched, w.seasons_watched,
	       coalesce(m.genres, s.genres, '{}') as genres,
	       coalesce(m.runtime_minutes, s.episode_run_time) as runtime_minutes,
	       coalesce(m.vote_average, s.vote_average) as vote_average,
	       extract(year from coalesce(m.release_date, s.first_air_date))::int as release_year,
	       w.added_at, w.updated_at,
	       s.total_episodes, s.total_seasons, s.in_production, s.next_air_date::text as next_air_date
	from watchlist_item w
	left join movies m on w.media_type = 'movie' and m.id = w.media_id
	left join series s on w.media_type = 'series' and s.id = w.media_id`

type WatchlistItem struct {
	ID              string     `json:"id" db:"id"`
	UserID          string     `json:"-" db:"-"`
	MediaType       string     `json:"mediaType" db:"media_type"`
	MediaID         int        `json:"mediaId" db:"media_id"`
	MediaTitle      string     `json:"mediaTitle" db:"media_title"`
	PosterPath      *string    `json:"posterPath" db:"poster_path"`
	Status          string     `json:"status" db:"status"`
	StartedAt       *time.Time `json:"startedAt,omitempty" db:"started_at"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty" db:"finished_at"`
	LastWatchedAt   *time.Time `json:"lastWatchedAt,omitempty" db:"last_watched_at"`
	WatchCount      int        `json:"watchCount" db:"watch_count"`
	EpisodesWatched int        `json:"episodesWatched" db:"episodes_watched"`
	SeasonsWatched  int        `json:"seasonsWatched" db:"seasons_watched"`
	Genres          []string   `json:"genres" db:"genres"`
	RuntimeMinutes  *int       `json:"runtimeMinutes,omitempty" db:"runtime_minutes"`
	VoteAverage     *float32   `json:"voteAverage,omitempty" db:"vote_average"`
	ReleaseYear     *int       `json:"releaseYear,omitempty" db:"release_year"`
	AddedAt         time.Time  `json:"addedAt" db:"added_at"`
	UpdatedAt       time.Time  `json:"updatedAt" db:"updated_at"`
	TotalEpisodes   *int       `json:"totalEpisodes,omitempty" db:"total_episodes"`
	TotalSeasons    *int       `json:"totalSeasons,omitempty" db:"total_seasons"`
	InProduction    *bool      `json:"inProduction,omitempty" db:"in_production"`
	NextAirDate     *string    `json:"nextAirDate,omitempty" db:"next_air_date"`
}

type ResolvedMedia struct {
	Title          string
	Genres         []string
	RuntimeMinutes *int
	SeriesTitle    *string
	EpisodeCount   *int
	SeasonCount    *int
}

type WatchlistRepository interface {
	List(ctx context.Context, userID string, status, mediaType *string) ([]WatchlistItem, error)
	Check(ctx context.Context, userID, mediaType string, mediaID int) (*WatchlistItem, error)
	Create(
		ctx context.Context,
		id, userID, mediaType string,
		mediaID int,
		status, mediaTitle string,
	) (*WatchlistItem, error)
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

func (s *WatchlistStore) List(ctx context.Context, userId string, status, mediaType *string) ([]WatchlistItem, error) {
	defer metrics.TrackDbDuration(ctx, "watchlist.list")()
	// Sort puts the most recent interaction first; id breaks timestamp ties.
	query := watchlistSelect + ` where w.user_id = $1`
	args := []any{userId}

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
	return pgx.CollectRows(rows, pgx.RowToStructByName[WatchlistItem])
}

func (s *WatchlistStore) Check(ctx context.Context, userId, mediaType string, mediaId int) (*WatchlistItem, error) {
	defer metrics.TrackDbDuration(ctx, "watchlist.check")()
	query := `select id, status from watchlist_item where user_id = $1 and media_type = $2 and media_id = $3`
	var item WatchlistItem
	err := s.pool.QueryRow(ctx, query, userId, mediaType, mediaId).Scan(&item.ID, &item.Status)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *WatchlistStore) Create(
	ctx context.Context,
	id, userID, mediaType string,
	mediaID int,
	status, mediaTitle string,
) (*WatchlistItem, error) {
	defer metrics.TrackDbDuration(ctx, "watchlist.create")()
	_, err := s.pool.Exec(ctx, `
		insert into watchlist_item (id, user_id, media_type, media_id, media_title, status)
		values ($1, $2, $3, $4, $5, $6)`, id, userID, mediaType, mediaID, mediaTitle, status)
	if err != nil {
		return nil, err
	}
	return s.getByID(ctx, id, userID)
}

func (s *WatchlistStore) UpdateStatus(ctx context.Context, id, userId, status string) (*WatchlistItem, error) {
	defer metrics.TrackDbDuration(ctx, "watchlist.update_status")()
	query := `
		update watchlist_item set
			status = $1,
			started_at = case when $1 = 'watched' then coalesce(started_at, now()) else started_at end,
			finished_at = case when $1 = 'watched' then coalesce(finished_at, now()) else null end,
			updated_at = now()
		where id = $2 and user_id = $3
	`
	tag, err := s.pool.Exec(ctx, query, status, id, userId)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	return s.getByID(ctx, id, userId)
}

// UpdateSummary recomputes denormalized progress fields from watch_event.
// For series, episodes_watched = SUM(season.episode_count) + COUNT(episode events);
// seasons_watched counts only seasons the user has fully completed
// (season-mark OR episode-mark count ≥ season's cached episode_count).
// Status reverts to 'want_to_watch' when no events remain.
func (s *WatchlistStore) UpdateSummary(ctx context.Context, userId, mediaType string, mediaID int) error {
	defer metrics.TrackDbDuration(ctx, "watchlist.update_summary")()

	if mediaType == "series" {
		return s.updateSeriesSummary(ctx, userId, mediaID)
	}
	return s.updateMovieSummary(ctx, userId, mediaID)
}

// updateSeriesSummary recomputes denormalized progress fields from watch_event for series.
// season_totals expands the seasons listed in the series payload into one row per season (specials excluded);
// episode_counts counts the user's episode events per season;
// complete_seasons is the count of seasons that are fully watched (season-mark or enough episode marks).
func (s *WatchlistStore) updateSeriesSummary(ctx context.Context, userId string, seriesId int) error {
	query := `
		with totals as (
			select coalesce(sum(case when media_type = 'season' then episode_count else 0 end), 0)
				+ sum(case when media_type = 'episode' then 1 else 0 end) as episodes_watched,
				min(coalesce(watched_at, created_at)) as first_watched_at,
				max(coalesce(watched_at, created_at)) as last_watched_at,
				count(*) as total_events
			from watch_event
			where user_id = $1 and series_id = $2 and media_type in ('season', 'episode')
		),
		metadata as (
			select total_episodes, in_production
			from series
			where id = $2
		),
		season_totals as (
			select (season->>'season_number')::int as season_number, (season->>'episode_count')::int as total
			from series, jsonb_array_elements(coalesce(payload->'seasons', '[]'::jsonb)) as season
			where id = $2 and (season->>'season_number')::int > 0
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
		),
		derived as (
			select
				coalesce(totals.episodes_watched, 0) as episodes_watched,
				coalesce(complete_seasons.cnt, 0) as seasons_watched,
				totals.first_watched_at,
				totals.last_watched_at,
				case
					when coalesce(totals.total_events, 0) = 0 then 'want_to_watch'
					when metadata.total_episodes is null then 'in_progress'
					when totals.episodes_watched < metadata.total_episodes then 'in_progress'
					when coalesce(metadata.in_production, false) then 'in_progress'
					else 'watched'
				end as status
			from totals
			left join complete_seasons on true
			left join metadata on true
		)
		update watchlist_item set
			episodes_watched = derived.episodes_watched,
			seasons_watched = derived.seasons_watched,
			watch_count = derived.episodes_watched,
			last_watched_at = derived.last_watched_at,
			started_at = derived.first_watched_at,
			finished_at = case when derived.status = 'watched' then derived.last_watched_at else null end,
			status = derived.status,
			updated_at = now()
		from derived
		where watchlist_item.user_id = $1
		  and watchlist_item.media_type = 'series'
		  and watchlist_item.media_id = $2
	`
	_, err := s.pool.Exec(ctx, query, userId, seriesId)
	return err
}

// updateMovieSummary recomputes denormalized progress fields from watch_event for movies.
func (s *WatchlistStore) updateMovieSummary(ctx context.Context, userId string, movieId int) error {
	query := `
		with e as (
			select
				count(*) as cnt,
				max(coalesce(watched_at, created_at)) as last_watched_at
			from watch_event
			where user_id = $1 and media_type = 'movie' and media_id = $2
		)
		update watchlist_item set
			watch_count = coalesce(e.cnt, 0),
			last_watched_at = e.last_watched_at,
			started_at = case when coalesce(e.cnt, 0) > 0 then e.last_watched_at else null end,
			finished_at = case when coalesce(e.cnt, 0) > 0 then e.last_watched_at else null end,
			status = case when coalesce(e.cnt, 0) = 0 then 'want_to_watch' else 'watched' end,
			updated_at = now()
		from e
		where watchlist_item.user_id = $1
		  and watchlist_item.media_type = 'movie'
		  and watchlist_item.media_id = $2
	`
	_, err := s.pool.Exec(ctx, query, userId, movieId)
	return err
}

func (s *WatchlistStore) Delete(ctx context.Context, id, userId string) error {
	defer metrics.TrackDbDuration(ctx, "watchlist.delete")()
	tag, err := s.pool.Exec(ctx, `delete from watchlist_item where id = $1 and user_id = $2`, id, userId)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *WatchlistStore) getByID(ctx context.Context, id, userID string) (*WatchlistItem, error) {
	rows, err := s.pool.Query(ctx, watchlistSelect+` where w.id = $1 and w.user_id = $2`, id, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[WatchlistItem])
}
