package main

import (
	"context"
	"io"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const defaultAPIRateLimit = 40.0
const defaultMinPopularity = 0.7
const hydrateLongDesc = `
Hydrate loads the full entity data from the source API for each unhydrated entity
within the minimum popularity threshold and saves them to the database,
respecting the hydration limit and source system API rate limits (default 40 requests per second).
`

type hydrateOptions struct {
	dir                string
	entity             entityFlag
	limit              int
	apiRateLimit       float64
	minPopularity      float64
	resume             bool
	fresh              bool
	isLimitSet         bool
	isRateLimitSet     bool
	isMinPopularitySet bool
	out                io.Writer
}

type hydrateSettings struct {
	limit         int
	apiRateLimit  float64
	minPopularity float64
	resuming      bool
}

func (o hydrateOptions) validate() error {
	switch {
	case !o.entity.isSet():
		return usageErrorf("--entity is required (movies, series, or people)")
	case o.limit <= 0 && !o.resume:
		return usageErrorf("--limit is required")
	case o.resume && o.fresh:
		return usageErrorf("--resume and --fresh are mutually exclusive")
	}
	return nil
}

func newHydrateCmd(g *globalOptions) *cobra.Command {
	var o hydrateOptions
	cmd := &cobra.Command{
		Use:   "hydrate",
		Short: "Hydrate the database with full rows for the most popular never-hydrated entity ids",
		Long:  hydrateLongDesc,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.dir = g.dir
			o.isLimitSet = cmd.Flags().Changed("limit")
			o.isRateLimitSet = cmd.Flags().Changed("api-rate-limit")
			o.isMinPopularitySet = cmd.Flags().Changed("min-popularity")
			o.out = cmd.OutOrStdout()
			if err := o.validate(); err != nil {
				return err
			}
			return runHydrate(cmd.Context(), o)
		},
	}
	cmd.Flags().Var(&o.entity, "entity", "movies, series, or people")
	cmd.Flags().IntVar(&o.limit, "limit", 0,
		"rows to hydrate this run, most popular not-yet-hydrated first")
	cmd.Flags().Float64Var(&o.apiRateLimit, "api-rate-limit", defaultAPIRateLimit,
		"source system API requests per second limit")
	cmd.Flags().Float64Var(&o.minPopularity, "min-popularity", defaultMinPopularity,
		"hydrate only rows at or above this popularity")
	cmd.Flags().BoolVar(&o.resume, "resume", false,
		"continue an unfinished run, restoring --limit, --api-rate-limit, and --min-popularity from its checkpoint")
	cmd.Flags().BoolVar(&o.fresh, "fresh", false,
		"reset the run counters and start a new run (hydrated rows are kept)")
	return cmd
}

// runHydrate hydrates the next --limit not-yet-hydrated rows of one entity (most popular first).
// A resumed run finishes its record's remaining share. Rows that failed or were interrupted are
// still unhydrated, so the next run picks them up again.
func runHydrate(ctx context.Context, o hydrateOptions) error {
	entity := o.entity.entity
	st, settings, err := resolveHydrateSettings(o)
	if err != nil {
		return err
	}

	s, err := openSession(ctx, sessionOptions{dir: o.dir, entity: entity, rateLimit: settings.apiRateLimit})
	if err != nil {
		return err
	}
	defer s.close()

	budget := settings.limit
	if settings.resuming {
		budget -= st.Hydrate.attempted()
	}
	if budget <= 0 {
		log.Info().Str("entity", entity.String()).Int("limit", settings.limit).Msg("run already complete")
		st.Hydrate.finish()
		return st.save(o.dir, entity)
	}

	progress, runErr := hydrateRows(ctx, s, st, o, settings, budget)
	return finishHydrate(ctx, s, progress, o.out, runErr)
}

// resolveHydrateSettings applies the rerun guard and revolves the target hydration parameters.
func resolveHydrateSettings(o hydrateOptions) (*state, hydrateSettings, error) {
	st, err := readState(o.dir, o.entity.entity)
	if err != nil {
		return nil, hydrateSettings{}, err
	}
	if err := st.checkUnfinishedRun("hydrate", o.resume, o.fresh); err != nil {
		return nil, hydrateSettings{}, err
	}
	settings := hydrateSettings{
		limit:         o.limit,
		apiRateLimit:  o.apiRateLimit,
		minPopularity: o.minPopularity,
		resuming:      o.resume && st.hasUnfinishedHydrate(),
	}
	if settings.resuming && !o.isLimitSet {
		settings.limit = st.Hydrate.Limit
	}
	if settings.resuming && !o.isRateLimitSet {
		settings.apiRateLimit = st.Hydrate.APIRateLimit
	}
	if settings.resuming && !o.isMinPopularitySet {
		settings.minPopularity = st.Hydrate.MinPopularity
	}
	if settings.limit <= 0 {
		return nil, hydrateSettings{}, usageErrorf("--limit must be positive")
	}
	return st, settings, nil
}

// hydrateRows records the run and drives catalog.Hydrate.
func hydrateRows(
	ctx context.Context,
	s *session,
	st *state,
	o hydrateOptions,
	settings hydrateSettings,
	budget int,
) (*hydrateProgress, error) {
	entity := o.entity.entity
	st.beginHydrateRun(settings)
	progress := &hydrateProgress{
		st:              st,
		dir:             o.dir,
		entity:          entity,
		term:            s.term,
		limit:           settings.limit,
		resuming:        settings.resuming,
		attemptedBefore: settings.limit - budget,
		elapsedBefore:   st.Hydrate.ElapsedSeconds,
		start:           time.Now(),
	}
	log.Info().Str("entity", entity.String()).Int("limit", settings.limit).Int("remaining", budget).
		Float64("api_rate_limit", settings.apiRateLimit).Float64("min_popularity", settings.minPopularity).
		Int("run", st.Hydrate.Runs).Bool("resumed", settings.resuming).Msg("hydrate started")

	_, err := s.catalog.Hydrate(ctx, catalog.HydrateOptions{
		Entity:        entity,
		Budget:        budget,
		Classes:       []store.WorkClass{store.WorkUnhydrated},
		MinPopularity: settings.minPopularity,
		OnRow:         progress.recordRow,
	})
	return progress, err
}

// finishHydrate closes the record on success, checkpoints progress, logs the run's totals, and prints the table counts.
func finishHydrate(ctx context.Context, s *session, p *hydrateProgress, out io.Writer, runErr error) error {
	if runErr == nil {
		p.st.Hydrate.finish()
	}
	p.checkpoint()
	log.Info().Str("entity", p.entity.String()).Int("attempted", p.run.Attempted).Int("ok", p.run.OK).
		Int("failed", p.run.Failed).Int("gone", p.run.Gone).Int("people_fetched", p.run.PeopleFetched).
		Str("elapsed", time.Since(p.start).Round(time.Second).String()).
		Bool("finished", !p.st.Hydrate.isUnfinished()).Msg("hydrate ended")
	printTableCounts(context.WithoutCancel(ctx), s.catalog, out) // still wanted after Ctrl-C
	return interruptedIfCancelled(runErr)
}
