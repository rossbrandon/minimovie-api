package achievements

import (
	"context"
	"sync"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog/log"
)

type Worker struct {
	queue            chan string
	achievementStore *store.AchievementStore
	watchlistStore   store.WatchlistRepository
	watchEventStore  store.WatchEventRepository

	wg   sync.WaitGroup
	stop context.CancelFunc
}

func NewWorker(
	achievementStore *store.AchievementStore,
	watchlistStore store.WatchlistRepository,
	watchEventStore store.WatchEventRepository,
	workers int,
) *Worker {
	w := &Worker{
		queue:            make(chan string, 1024),
		achievementStore: achievementStore,
		watchlistStore:   watchlistStore,
		watchEventStore:  watchEventStore,
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.stop = cancel
	for range workers {
		w.wg.Go(func() { w.run(ctx) })
	}

	return w
}

func (w *Worker) Enqueue(userID string) {
	select {
	case w.queue <- userID:
	default:
		log.Warn().Msg("achievement queue full, dropping job")
	}
}

func (w *Worker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case userID := <-w.queue:
			w.checkUser(userID)
		}
	}
}

func (w *Worker) checkUser(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	w.checkAll(ctx, userID)
}

func (w *Worker) checkAll(ctx context.Context, userID string) {
	for _, def := range AllDefinitions {
		w.checkSingle(ctx, userID, def)
	}
}

func (w *Worker) checkSingle(ctx context.Context, userID string, def Definition) {
	exists, err := w.achievementStore.Exists(ctx, userID, def.ID)
	if err != nil || exists {
		return
	}

	earned, mediaType, mediaID, mediaTitle := def.Check(ctx, userID, w.watchlistStore, w.watchEventStore)
	if !earned {
		return
	}

	if err := w.achievementStore.Create(ctx, userID, def.ID, mediaType, mediaID, mediaTitle); err != nil {
		log.Error().Err(err).Str("achievement", def.ID).Msg("failed to award achievement")
		return
	}

	metrics.M.RecordAchievementEarned(ctx, def.ID)
}

// Stop ends the workers and waits for the checks in flight.
func (w *Worker) Stop() {
	w.stop()
	w.wg.Wait()
}
