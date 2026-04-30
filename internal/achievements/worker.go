package achievements

import (
	"context"
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

	for i := 0; i < workers; i++ {
		go w.run()
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

func (w *Worker) run() {
	for userID := range w.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		w.checkAll(ctx, userID)
		cancel()
	}
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

	if metrics.M != nil {
		metrics.M.RecordAchievementEarned(ctx, def.ID)
	}
}

func (w *Worker) Stop() {
	close(w.queue)
}
