package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncJobStore_LastSuccessfulJobIsTheWatermark(t *testing.T) {
	ctx := context.Background()
	s := NewSyncJobStore(testPool)
	jobType := "test_sync_" + t.Name()

	last, err := s.GetLastSuccessfulJob(ctx, jobType)
	require.NoError(t, err)
	assert.Nil(t, last, "no job yet")

	failed, err := s.StartJob(ctx, jobType, "2026-09-01", "2026-09-02")
	require.NoError(t, err)
	require.NoError(t, s.FailJob(ctx, failed.ID, "boom"))
	done, err := s.StartJob(ctx, jobType, "2026-09-02", "2026-09-03")
	require.NoError(t, err)
	require.NoError(t, s.CompleteJob(ctx, done.ID, 10, nil, 7))

	last, err = s.GetLastSuccessfulJob(ctx, jobType)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, "2026-09-02", last.StartDate)
	assert.Equal(t, "2026-09-03", last.EndDate, "a failed job never moves the watermark")
}
