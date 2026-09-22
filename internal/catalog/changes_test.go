package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangeWindow(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	svc := newTestService(t, newFakeTMDB(t), 0)
	jobs := store.NewSyncJobStore(testPool)
	day := func(offset int) string { return time.Now().UTC().AddDate(0, 0, offset).Format(time.DateOnly) }

	start, end, err := svc.ChangeWindow(ctx, jobs, EntityMovie)
	require.NoError(t, err)
	assert.Equal(t, day(-1), start, "no job and no hydrated row: yesterday")
	assert.Equal(t, day(0), end)

	_, err = testPool.Exec(ctx, `insert into movies (source_id, title, payload, fetched_at)
		values (1, 'Old', '{}', now() - interval '10 days')`)
	require.NoError(t, err)
	start, _, err = svc.ChangeWindow(ctx, jobs, EntityMovie)
	require.NoError(t, err)
	assert.Equal(t, day(-11), start, "no job: a day before the oldest hydrated row")

	run, err := jobs.StartJob(ctx, EntityMovie.SyncJobType(), day(-5), day(-3))
	require.NoError(t, err)
	require.NoError(t, jobs.CompleteJob(ctx, run.ID, 0, nil, 0))
	start, _, err = svc.ChangeWindow(ctx, jobs, EntityMovie)
	require.NoError(t, err)
	assert.Equal(t, day(-3), start, "the last completed job's end, so consecutive runs overlap by a day")

	start, _, err = svc.ChangeWindow(ctx, jobs, EntitySeries)
	require.NoError(t, err)
	assert.Equal(t, day(-1), start, "watermarks are per entity")

	run, err = jobs.StartJob(ctx, EntityMovie.SyncJobType(), day(0), day(3))
	require.NoError(t, err)
	require.NoError(t, jobs.CompleteJob(ctx, run.ID, 0, nil, 0))
	start, _, err = svc.ChangeWindow(ctx, jobs, EntityMovie)
	require.NoError(t, err)
	assert.Equal(t, day(0), start, "a watermark left in the future is clamped to today")
}
