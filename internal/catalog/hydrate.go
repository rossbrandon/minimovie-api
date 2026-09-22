package catalog

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"golang.org/x/sync/errgroup"
)

const (
	claimBatchSize      = 25
	batchWorkers        = 5  // batches in flight
	prefetchConcurrency = 10 // documents in flight per batch
	batchTimeout        = 2 * time.Minute
	maxConsecutiveFails = 20
)

const (
	OutcomeOK     Outcome = "ok"
	OutcomeGone   Outcome = "gone" // TMDB returned 404; the row was deleted
	OutcomeFailed Outcome = "failed"
)

// ErrTooManyFailures ends a run after maxConsecutiveFails rows fail in a row.
var ErrTooManyFailures = errors.New("catalog: too many consecutive failures")

type entityTable interface {
	Claim(ctx context.Context, tx pgx.Tx, class store.WorkClass, p store.ClaimParams) ([]int, error)
	DeleteBySourceID(ctx context.Context, db store.DBTX, sourceID int) error
	MarkStale(ctx context.Context, sourceIDs []int) (int64, error)
}

type Outcome string

type RowResult struct {
	SourceID      int
	Outcome       Outcome
	Err           error
	PeopleFetched int
}

type HydrateOptions struct {
	Entity        Entity
	Budget        int //number of rows to attempt. 0 or less means until nothing is left to claim.
	Classes       []store.WorkClass
	MinPopularity float64
	OnRow         func(RowResult) // called after every row so a caller can keep counters/checkpoints.
}

type HydrateStats struct {
	Attempted     int
	OK            int
	Gone          int
	Failed        int
	PeopleFetched int
}

type failedRows struct {
	mu  sync.Mutex
	ids []int
}

// prepared is a claimed row after the network call has been made.
type prepared struct {
	sourceID int
	w        writes
	err      error
	result   RowResult
}

// Hydrate claims rows in batches and runs the fetch path on each.
func (s *Service) Hydrate(ctx context.Context, opts HydrateOptions) (HydrateStats, error) {
	var stats HydrateStats
	classes := opts.Classes
	if classes == nil {
		classes = []store.WorkClass{store.WorkStale, store.WorkExpiring, store.WorkUnhydrated}
	}

	var streak int
	for _, class := range classes {
		if err := s.hydrateClass(ctx, opts, class, &stats, &streak); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// hydrateClass drains one work class with batchWorkers concurrent batches.
func (s *Service) hydrateClass(
	ctx context.Context,
	opts HydrateOptions,
	class store.WorkClass,
	stats *HydrateStats,
	streak *int,
) error {
	wctx, stop := context.WithCancel(ctx)
	defer stop()

	// Each worker reserves its batch from the budget before claiming.
	var remaining atomic.Int64
	remaining.Store(int64(opts.Budget - stats.Attempted))
	nextLimit := func() int {
		if opts.Budget <= 0 {
			return claimBatchSize
		}
		left := remaining.Add(-claimBatchSize) + claimBatchSize
		return min(claimBatchSize, int(left))
	}

	results := make(chan []RowResult)
	var failed failedRows
	var g errgroup.Group
	for range batchWorkers {
		g.Go(func() error {
			for wctx.Err() == nil {
				limit := nextLimit()
				if limit <= 0 {
					return nil
				}
				batchCtx, cancel := context.WithTimeout(context.WithoutCancel(wctx), batchTimeout)
				rows, err := s.hydrateBatch(batchCtx, opts, class, limit, &failed)
				cancel()
				if err != nil {
					stop()
					return err
				}
				if len(rows) == 0 {
					return nil
				}
				select {
				case results <- rows:
				case <-wctx.Done():
				}
			}
			return nil
		})
	}
	var runErr error
	go func() {
		runErr = g.Wait()
		close(results)
	}()

	var tripped error
	for rows := range results {
		if tripped != nil {
			continue // drain until the workers have exited
		}
		for _, r := range rows {
			stats.Record(r)
			if r.Outcome == OutcomeFailed {
				*streak++
			} else {
				*streak = 0
			}
			if opts.OnRow != nil {
				opts.OnRow(r)
			}
			if *streak >= maxConsecutiveFails {
				tripped = fmt.Errorf("%w: last error: %w", ErrTooManyFailures, r.Err)
				stop()
				break
			}
		}
	}
	if tripped != nil {
		return tripped
	}
	if runErr != nil {
		return runErr
	}
	return ctx.Err()
}

// Record folds one row's outcome into the totals.
func (st *HydrateStats) Record(r RowResult) {
	st.Attempted++
	st.PeopleFetched += r.PeopleFetched
	switch r.Outcome {
	case OutcomeOK:
		st.OK++
	case OutcomeGone:
		st.Gone++
	case OutcomeFailed:
		st.Failed++
	}
}

func (s *Service) hydrateBatch(
	ctx context.Context,
	opts HydrateOptions,
	class store.WorkClass,
	limit int,
	failed *failedRows,
) ([]RowResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	table, err := s.table(opts.Entity)
	if err != nil {
		return nil, err
	}
	ids, err := table.Claim(ctx, tx, class, store.ClaimParams{
		Limit:         limit,
		RefreshAfter:  store.RefreshAfter,
		MinPopularity: opts.MinPopularity,
		Exclude:       failed.list(),
	})
	if err != nil {
		return nil, err
	}

	docs := s.prefetch(ctx, opts.Entity, ids)
	for i := range docs {
		hydrateRow(ctx, tx, table, &docs[i])
		if docs[i].result.Outcome == OutcomeFailed {
			failed.add(docs[i].sourceID)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("catalog: commit: %w", err)
	}

	// Post-commit work is a network round-trip or two per row, so the rows run side by side; each
	// goroutine touches only its own element.
	var g errgroup.Group
	g.SetLimit(personFetchConcurrency)
	for i := range docs {
		if docs[i].result.Outcome != OutcomeOK {
			continue
		}
		g.Go(func() error {
			docs[i].result.PeopleFetched, docs[i].result.Err = s.afterCommit(ctx, docs[i].w)
			return nil
		})
	}
	_ = g.Wait()

	results := make([]RowResult, 0, len(docs))
	for _, doc := range docs {
		results = append(results, doc.result)
	}
	return results, nil
}

// afterCommit runs a row's post-commit work.
func (s *Service) afterCommit(ctx context.Context, w writes) (int, error) {
	if err := w.seed(ctx, s.pool); err != nil {
		return 0, err
	}
	if len(w.refs) == 0 {
		return 0, nil
	}
	_, fetched, err := s.fetchPriorityPeople(ctx, w.refs, s.peoplePerHydration)
	return fetched, err
}

// prefetch runs the network half of every claimed row concurrently. Writes stay sequential inside
// the batch transaction, so this is where a batch spends its wall-clock.
func (s *Service) prefetch(ctx context.Context, entity Entity, ids []int) []prepared {
	docs := make([]prepared, len(ids))
	var g errgroup.Group
	g.SetLimit(prefetchConcurrency)
	for i, id := range ids {
		g.Go(func() error {
			w, err := s.prepare(ctx, entity, id)
			docs[i] = prepared{sourceID: id, w: w, err: err}
			return nil
		})
	}
	_ = g.Wait()
	return docs
}

func (s *Service) prepare(ctx context.Context, entity Entity, sourceID int) (writes, error) {
	switch entity {
	case EntityMovie:
		_, w, err := s.getMovie(ctx, sourceID)
		return w, err
	case EntitySeries:
		_, w, err := s.getSeries(ctx, sourceID)
		return w, err
	case EntityPerson:
		_, w, err := s.getPerson(ctx, sourceID)
		return w, err
	}
	return writes{}, fmt.Errorf("catalog: cannot hydrate entity %s", entity)
}

// hydrateRow runs a prepared row's write inside a savepoint, so a failed row rolls back alone and
// the batch commits; its seed waits for the commit. A 404 from the fetch deletes the row.
func hydrateRow(ctx context.Context, tx pgx.Tx, table entityTable, doc *prepared) {
	doc.result = RowResult{SourceID: doc.sourceID, Outcome: OutcomeOK}
	fail := func(err error) {
		doc.result.Outcome, doc.result.Err = OutcomeFailed, err
	}
	gone := errors.Is(doc.err, tmdb.ErrNotFound)
	if doc.err != nil && !gone {
		fail(doc.err)
		return
	}

	sp, err := tx.Begin(ctx)
	if err != nil {
		fail(err)
		return
	}
	if gone {
		doc.result.Outcome = OutcomeGone
		err = table.DeleteBySourceID(ctx, sp, doc.sourceID)
	} else {
		err = doc.w.write(ctx, sp)
	}
	if err != nil {
		_ = sp.Rollback(ctx)
		fail(err)
		return
	}
	if err := sp.Commit(ctx); err != nil {
		fail(err)
	}
}

func (f *failedRows) add(id int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, id)
}

func (f *failedRows) list() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ids)
}

func (s *Service) table(entity Entity) (entityTable, error) {
	switch entity {
	case EntityMovie:
		return s.movies, nil
	case EntitySeries:
		return s.series, nil
	case EntityPerson:
		return s.people, nil
	}
	return nil, fmt.Errorf("catalog: no claim queue for entity %s", entity)
}
