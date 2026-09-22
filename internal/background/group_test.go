package background

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroup_GoIsDetachedFromCallerCancellation(t *testing.T) {
	var g Group
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ran := make(chan error, 1)
	g.Go(ctx, "test", time.Second, func(ctx context.Context) error {
		ran <- ctx.Err()
		return nil
	})

	require.NoError(t, g.Wait(context.Background()))
	assert.NoError(t, <-ran, "the task's context ignores the caller's cancellation")
}

func TestGroup_GoHonoursTimeout(t *testing.T) {
	var g Group
	got := make(chan error, 1)
	g.Go(context.Background(), "test", time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		got <- ctx.Err()
		return ctx.Err()
	})

	require.NoError(t, g.Wait(context.Background()))
	assert.ErrorIs(t, <-got, context.DeadlineExceeded)
}

func TestGroup_WaitReturnsAfterLastTask(t *testing.T) {
	var g Group
	release := make(chan struct{})
	for range 3 {
		g.Go(context.Background(), "test", time.Second, func(context.Context) error {
			<-release
			return errors.New("logged, not returned")
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, g.Wait(ctx), context.DeadlineExceeded, "Wait obeys its own context while tasks run")

	close(release)
	require.NoError(t, g.Wait(context.Background()))
}
