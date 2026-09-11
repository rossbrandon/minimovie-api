package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

const (
	RefreshAfter = 150 * 24 * time.Hour
	ExpireAfter  = 180 * 24 * time.Hour
)

const (
	WorkStale    WorkClass = iota + 1 // changed at the provider since it was fetched
	WorkExpiring                      // hydrated and older than RefreshAfter
	WorkUnhydrated
)

const MinHydratePopularity = 5.0

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// WorkClass orders the daily job's refresh and hydration work.
type WorkClass int

// Skeleton is what the id export files, search results, and credits know about a title.
type Skeleton struct {
	SourceID    int
	Title       string
	Popularity  float64
	Overview    *string
	PosterPath  *string
	ReleaseDate *string // YYYY-MM-DD
	VoteAverage *float64
}

type ClaimParams struct {
	Limit         int
	RefreshAfter  time.Duration
	MinPopularity float64
	Exclude       []int
}

// CatalogStats is one table's hydration stats at a glance.
type CatalogStats struct {
	Table             string `db:"table_name"`
	Rows              int    `db:"rows"`
	Hydrated          int    `db:"hydrated"`
	Stale             int    `db:"stale"`
	Expiring          int    `db:"expiring"`
	UnhydratedPopular int    `db:"unhydrated_popular"`
	OldestDays        int    `db:"oldest_days"`
}

type catalogTable struct {
	pool *pgxpool.Pool
	name string
}

func (t catalogTable) TableName() string {
	return t.name
}

func (t catalogTable) MarkStale(ctx context.Context, sourceIDs []int) (int64, error) {
	if len(sourceIDs) == 0 {
		return 0, nil
	}
	defer metrics.TrackDbDuration(ctx, "write")()

	// Only a hydrated row has anything to refresh
	tag, err := t.pool.Exec(ctx, `update `+t.name+` set stale = true, updated_at = now()
		where source_id = any($1) and payload is not null and not stale`, sourceIDs)
	if err != nil {
		return 0, fmt.Errorf("%s store: mark stale: %w", t.name, err)
	}
	return tag.RowsAffected(), nil
}

func (t catalogTable) DeleteBySourceID(ctx context.Context, db DBTX, sourceID int) error {
	defer metrics.TrackDbDuration(ctx, "write")()

	if _, err := db.Exec(ctx, `delete from `+t.name+` where source_id = $1`, sourceID); err != nil {
		return fmt.Errorf("%s store: delete source %d: %w", t.name, sourceID, err)
	}
	return nil
}

func (t catalogTable) DeleteExpired(ctx context.Context) (int64, error) {
	return purgeExpired(ctx, t.pool, t.name)
}

// Claim locks up to p.Limit rows of one work class for the calling transaction and returns their source ids.
func (t catalogTable) Claim(ctx context.Context, tx pgx.Tx, class WorkClass, p ClaimParams) ([]int, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	exclude := p.Exclude
	if exclude == nil {
		exclude = []int{} // a nil array is null in SQL and would exclude every row
	}
	var rows pgx.Rows
	var err error
	switch class {
	case WorkStale:
		rows, err = tx.Query(ctx, `select source_id from `+t.name+` where stale and payload is not null and source_id <> all($2)
			order by fetched_at limit $1 for update skip locked`, p.Limit, exclude)
	case WorkExpiring:
		rows, err = tx.Query(ctx, `select source_id from `+t.name+` where payload is not null and fetched_at < now() - make_interval(secs => $2)
			and source_id <> all($3) order by fetched_at limit $1 for update skip locked`, p.Limit, p.RefreshAfter.Seconds(), exclude)
	case WorkUnhydrated:
		rows, err = tx.Query(ctx, `select source_id from `+t.name+` where payload is null and popularity >= $2 and source_id <> all($3)
			order by popularity desc limit $1 for update skip locked`, p.Limit, p.MinPopularity, exclude)
	default:
		return nil, fmt.Errorf("%s store: unknown work class %d", t.name, class)
	}
	if err != nil {
		return nil, fmt.Errorf("%s store: claim: %w", t.name, err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, fmt.Errorf("%s store: claim: %w", t.name, err)
	}
	return ids, nil
}

func (t catalogTable) Stats(ctx context.Context) (CatalogStats, error) {
	defer metrics.TrackDbDuration(ctx, "read")()

	rows, err := t.pool.Query(ctx, `
		select $1::text as table_name,
		       count(*)::int as rows,
		       count(payload)::int as hydrated,
		       (count(*) filter (where stale and payload is not null))::int as stale,
		       (count(*) filter (where payload is not null and fetched_at < now() - make_interval(secs => $2)))::int as expiring,
		       (count(*) filter (where payload is null and popularity >= $3))::int as unhydrated_popular,
		       coalesce(extract(days from now() - min(fetched_at))::int, 0) as oldest_days
		from `+t.name, t.name, RefreshAfter.Seconds(), MinHydratePopularity)
	if err != nil {
		return CatalogStats{}, fmt.Errorf("%s store: stats: %w", t.name, err)
	}
	stats, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[CatalogStats])
	if err != nil {
		return CatalogStats{}, fmt.Errorf("%s store: stats: %w", t.name, err)
	}
	return stats, nil
}

func purgeExpired(ctx context.Context, pool *pgxpool.Pool, table string) (int64, error) {
	defer metrics.TrackDbDuration(ctx, "write")()

	// Rows never hydrated have no fetched_at, so the cap applies only to rows holding a TMDB document.
	tag, err := pool.Exec(ctx, `delete from `+table+` where fetched_at < now() - make_interval(secs => $1)`, ExpireAfter.Seconds())
	if err != nil {
		return 0, fmt.Errorf("%s store: delete expired: %w", table, err)
	}
	if metrics.M != nil {
		metrics.M.RecordDbPurge(ctx, table, tag.RowsAffected())
	}
	return tag.RowsAffected(), nil
}

func nullTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}

func scanID(row pgx.Row) (int, error) {
	var id int
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}
