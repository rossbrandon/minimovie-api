package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SyncJob struct {
	ID              int
	Type            string
	StartDate       string
	EndDate         string
	Status          string
	Message         string
	UpdatedIDs      []int
	TmdbChangeCount int
	UpdatedCount    int
	DurationMs      int
	StartedAt       time.Time
	FinishedAt      *time.Time
}

type SyncJobStore struct {
	pool *pgxpool.Pool
}

func NewSyncJobStore(pool *pgxpool.Pool) *SyncJobStore {
	return &SyncJobStore{pool: pool}
}

func (s *SyncJobStore) StartJob(ctx context.Context, jobType, startDate, endDate string) (*SyncJob, error) {
	query := `
		insert into sync_job_status (type, start_date, end_date, status, started_at)
		values ($1, $2, $3, 'running', now())
		returning id, started_at
	`

	var job SyncJob
	job.Type = jobType
	job.StartDate = startDate
	job.EndDate = endDate
	job.Status = "running"

	err := s.pool.QueryRow(ctx, query, jobType, startDate, endDate).Scan(&job.ID, &job.StartedAt)
	if err != nil {
		return nil, err
	}

	return &job, nil
}

func (s *SyncJobStore) CompleteJob(ctx context.Context, jobID int, tmdbChangeCount int, changedIDs []int, updatedCount int64) error {
	query := `
		update sync_job_status
		set status = 'completed',
			tmdb_change_count = $2,
			updated_ids = $3,
			updated_count = $4,
			duration_ms = extract(epoch from (now() - started_at)) * 1000,
			finished_at = now(),
			updated_at = now()
		where id = $1
	`

	_, err := s.pool.Exec(ctx, query, jobID, tmdbChangeCount, changedIDs, updatedCount)
	return err
}

func (s *SyncJobStore) FailJob(ctx context.Context, jobID int, errMessage string) error {
	query := `
		update sync_job_status
		set status = 'failed',
			message = $2,
			duration_ms = extract(epoch from (now() - started_at)) * 1000,
			finished_at = now(),
			updated_at = now()
		where id = $1
	`

	_, err := s.pool.Exec(ctx, query, jobID, errMessage)
	return err
}

func (s *SyncJobStore) GetLastSuccessfulJob(ctx context.Context, jobType string) (*SyncJob, error) {
	query := `
		select id, type, start_date, end_date, status, started_at, finished_at
		from sync_job_status
		where type = $1 and status = 'completed'
		order by finished_at desc
		limit 1
	`

	var job SyncJob
	err := s.pool.QueryRow(ctx, query, jobType).Scan(
		&job.ID,
		&job.Type,
		&job.StartDate,
		&job.EndDate,
		&job.Status,
		&job.StartedAt,
		&job.FinishedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &job, nil
}
