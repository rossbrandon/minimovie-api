package background

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rs/zerolog/log"
)

type Group struct {
	wg sync.WaitGroup
}

func (g *Group) Go(
	ctx context.Context,
	name string,
	timeout time.Duration,
	fn func(context.Context) error,
) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	g.wg.Go(func() {
		defer cancel()
		start := time.Now()
		err := fn(ctx)
		duration := time.Since(start)
		outcome := "success"
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			outcome = "deadline_exceeded"
		case err != nil:
			outcome = "error"
		}
		if err != nil {
			log.Error().Err(err).Str("task", name).Dur("duration_ms", duration).Msg("background task failed")
		}
		metrics.M.RecordBgPersist(ctx, name, outcome, duration)
	})
}

// Wait blocks until every task has finished or ctx is done.
func (g *Group) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
