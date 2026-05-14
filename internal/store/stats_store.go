package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

type StatsResult struct {
	TotalMoviesWatched      int             `json:"totalMoviesWatched"`
	TotalSeriesCompleted    int             `json:"totalSeriesCompleted"`
	TotalEpisodesWatched    int             `json:"totalEpisodesWatched"`
	EstimatedMinutesWatched int             `json:"estimatedMinutesWatched"`
	GenreBreakdown          []GenreStat     `json:"genreBreakdown"`
	MonthlyActivity         []MonthActivity `json:"monthlyActivity"`
	CurrentStreak           int             `json:"currentStreak"`
	LongestStreak           int             `json:"longestStreak"`
}

type GenreStat struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

type MonthActivity struct {
	Month           string `json:"month"`
	MoviesWatched   int    `json:"moviesWatched"`
	EpisodesWatched int    `json:"episodesWatched"`
}

type StatsRepository interface {
	GetStats(ctx context.Context, userID string) (*StatsResult, error)
}

type StatsStore struct {
	pool *pgxpool.Pool
}

func NewStatsStore(pool *pgxpool.Pool) *StatsStore {
	return &StatsStore{pool: pool}
}

func (s *StatsStore) GetStats(ctx context.Context, userID string) (*StatsResult, error) {
	defer metrics.TrackDbDuration(ctx, "stats.get_stats")()
	result := &StatsResult{}

	if err := s.pool.QueryRow(ctx,
		`select count(*) from watchlist_item where user_id = $1 and status = 'watched' and media_type = 'movie'`,
		userID,
	).Scan(&result.TotalMoviesWatched); err != nil {
		log.Warn().Err(err).Msg("stats: failed to query movies watched")
	}

	if err := s.pool.QueryRow(ctx,
		`select count(*) from watchlist_item where user_id = $1 and status = 'watched' and media_type = 'series'`,
		userID,
	).Scan(&result.TotalSeriesCompleted); err != nil {
		log.Warn().Err(err).Msg("stats: failed to query series completed")
	}

	if err := s.pool.QueryRow(ctx,
		`select coalesce(sum(case
			when media_type = 'episode' then 1
			when media_type = 'season' then coalesce(episode_count, 1)
			else 0
		end), 0)
		from watch_event where user_id = $1 and media_type in ('episode', 'season')`,
		userID,
	).Scan(&result.TotalEpisodesWatched); err != nil {
		log.Warn().Err(err).Msg("stats: failed to query episodes watched")
	}

	if err := s.pool.QueryRow(ctx,
		`select coalesce(sum(runtime_minutes), 0) from watch_event where user_id = $1`,
		userID,
	).Scan(&result.EstimatedMinutesWatched); err != nil {
		log.Warn().Err(err).Msg("stats: failed to query minutes watched")
	}

	genreRows, err := s.pool.Query(ctx,
		`select unnest(genres) as genre, count(*) as cnt
		 from watch_event where user_id = $1
		 group by genre order by cnt desc limit 20`,
		userID,
	)
	if err == nil {
		defer genreRows.Close()
		for genreRows.Next() {
			var gs GenreStat
			if genreRows.Scan(&gs.Genre, &gs.Count) == nil {
				result.GenreBreakdown = append(result.GenreBreakdown, gs)
			}
		}
	}

	monthRows, err := s.pool.Query(ctx,
		`select to_char(date_trunc('month', watched_at at time zone 'UTC'), 'YYYY-MM') as month,
		        count(*) filter (where media_type = 'movie') as movies,
		        coalesce(sum(case
		            when media_type = 'episode' then 1
		            when media_type = 'season' then coalesce(episode_count, 1)
		            else 0
		        end), 0) as episodes
		 from watch_event where user_id = $1 and watched_at is not null
		 group by month order by month desc limit 24`,
		userID,
	)
	if err == nil {
		defer monthRows.Close()
		for monthRows.Next() {
			var ma MonthActivity
			if monthRows.Scan(&ma.Month, &ma.MoviesWatched, &ma.EpisodesWatched) == nil {
				result.MonthlyActivity = append(result.MonthlyActivity, ma)
			}
		}
	}

	result.CurrentStreak, result.LongestStreak = s.computeStreaks(ctx, userID)

	return result, nil
}

func (s *StatsStore) computeStreaks(ctx context.Context, userID string) (current, longest int) {
	rows, err := s.pool.Query(ctx,
		`select distinct (watched_at at time zone 'UTC')::date as watch_date
		 from watch_event where user_id = $1 and watched_at is not null
		 order by watch_date desc`,
		userID,
	)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var d time.Time
		if rows.Scan(&d) == nil {
			dates = append(dates, d)
		}
	}

	if len(dates) == 0 {
		return 0, 0
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	streak := 0
	if dates[0].Equal(today) || dates[0].Equal(today.AddDate(0, 0, -1)) {
		streak = 1
		for i := 1; i < len(dates); i++ {
			diff := dates[i-1].Sub(dates[i])
			if diff == 24*time.Hour {
				streak++
			} else {
				break
			}
		}
	}

	best := 1
	run := 1
	for i := 1; i < len(dates); i++ {
		if dates[i-1].Sub(dates[i]) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
	}
	return streak, best
}
