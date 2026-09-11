package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const updateLongDesc = `
Update brings hydrated rows up to date with the source: ask the changes feed which ids moved, flag
the rows we hold, and refresh every flagged or expiring row. The window starts where the last update
of that entity ended (recorded in sync_job_status) or, before any update, at the oldest hydrated row,
and ends today; --start and --end override it. Without --entity all three run in turn.
`

type updateOptions struct {
	dir          string
	entity       entityFlag // unset means all three
	start        string
	end          string
	apiRateLimit float64
	out          io.Writer
}

func (o updateOptions) validate() error {
	switch {
	case (o.start == "") != (o.end == ""):
		return usageErrorf("--start and --end go together")
	case o.start != "" && !validDateRange(o.start, o.end):
		return usageErrorf("--start and --end must be YYYY-MM-DD with start on or before end")
	}
	return nil
}

func newUpdateCmd(g *globalOptions) *cobra.Command {
	var o updateOptions
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Flag rows the source changed and refresh them",
		Long:  updateLongDesc,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.dir = g.dir
			o.out = cmd.OutOrStdout()
			if err := o.validate(); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), o)
		},
	}
	cmd.Flags().Var(&o.entity, "entity", "movies, series, or people; default all three")
	cmd.Flags().StringVar(&o.start, "start", "", "first day of the changes window (YYYY-MM-DD)")
	cmd.Flags().StringVar(&o.end, "end", "", "last day of the changes window (YYYY-MM-DD)")
	cmd.Flags().Float64Var(&o.apiRateLimit, "api-rate-limit", defaultAPIRateLimit,
		"source system API requests per second limit")
	return cmd
}

func runUpdate(ctx context.Context, o updateOptions) error {
	entities := seedEntities
	if o.entity.isSet() {
		entities = []catalog.Entity{o.entity.entity}
	}
	for _, entity := range entities {
		if err := updateEntity(ctx, o, entity); err != nil {
			return err
		}
	}
	return nil
}

// updateEntity runs one entity's changes window as a sync job, so a finished run becomes the next
// run's starting point and an interrupted one is retried
func updateEntity(ctx context.Context, o updateOptions, entity catalog.Entity) error {
	s, err := openSession(ctx, sessionOptions{dir: o.dir, entity: entity, rateLimit: o.apiRateLimit})
	if err != nil {
		return err
	}
	defer s.close()

	jobs := store.NewSyncJobStore(s.pool)
	start, end, err := changeWindow(ctx, s.catalog, jobs, entity, o)
	if err != nil {
		return err
	}
	job, err := jobs.StartJob(ctx, entity.SyncJobType(), start, end)
	if err != nil {
		return err
	}
	log.Info().Str("entity", entity.String()).Str("start", start).Str("end", end).Msg("update started")

	changed, marked, err := s.catalog.SyncChanges(ctx, entity, start, end)
	if err != nil {
		return failUpdate(ctx, jobs, job.ID, err)
	}
	log.Info().Str("entity", entity.String()).Int("changed", changed).Int64("flagged", marked).Msg("changes flagged")

	progress := &refreshProgress{entity: entity, term: s.term, start: time.Now()}
	stats, err := s.catalog.Hydrate(ctx, catalog.HydrateOptions{
		Entity:  entity,
		Classes: []store.WorkClass{store.WorkStale, store.WorkExpiring},
		OnRow:   progress.recordRow,
	})
	if err != nil {
		return failUpdate(ctx, jobs, job.ID, err)
	}
	if err := jobs.CompleteJob(ctx, job.ID, changed, nil, int64(stats.OK)); err != nil {
		return err
	}
	log.Info().Str("entity", entity.String()).Int("refreshed", stats.OK).Int("failed", stats.Failed).
		Int("gone", stats.Gone).Int("people_fetched", stats.PeopleFetched).
		Str("elapsed", time.Since(progress.start).Round(time.Second).String()).Msg("update ended")
	printTableCounts(context.WithoutCancel(ctx), s.catalog, o.out)
	return nil
}

// changeWindow is the date range to ask the feed for. Consecutive runs overlap by a day.
func changeWindow(
	ctx context.Context,
	svc *catalog.Service,
	jobs *store.SyncJobStore,
	entity catalog.Entity,
	o updateOptions,
) (start, end string, err error) {
	if o.start != "" {
		return o.start, o.end, nil
	}
	today := time.Now().UTC()
	end = today.Format(time.DateOnly)
	last, err := jobs.GetLastSuccessfulJob(ctx, entity.SyncJobType())
	if err != nil {
		return "", "", err
	}
	if last != nil {
		// An explicit --end in the future leaves a watermark past today.
		return min(last.EndDate, end), end, nil
	}
	stats, err := entityStats(ctx, svc, entity)
	if err != nil {
		return "", "", err
	}
	// OldestDays is a whole-day floor: reach back one more day.
	return today.AddDate(0, 0, -(stats.OldestDays + 1)).Format(time.DateOnly), end, nil
}

func entityStats(ctx context.Context, svc *catalog.Service, entity catalog.Entity) (store.CatalogStats, error) {
	all, err := svc.Stats(ctx)
	if err != nil {
		return store.CatalogStats{}, err
	}
	for _, st := range all {
		if st.Table == entity.String() {
			return st, nil
		}
	}
	return store.CatalogStats{}, fmt.Errorf("no stats for %s", entity)
}

// failUpdate records the failure on the job.
func failUpdate(ctx context.Context, jobs *store.SyncJobStore, jobID int, err error) error {
	if failErr := jobs.FailJob(context.WithoutCancel(ctx), jobID, err.Error()); failErr != nil {
		log.Error().Err(failErr).Msg("could not record the failed update")
	}
	return interruptedIfCancelled(err)
}

func validDateRange(start, end string) bool {
	from, err := time.Parse(time.DateOnly, start)
	if err != nil {
		return false
	}
	to, err := time.Parse(time.DateOnly, end)
	return err == nil && !from.After(to)
}
