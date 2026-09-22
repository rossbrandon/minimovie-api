package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var entities = []catalog.Entity{catalog.EntityMovie, catalog.EntitySeries, catalog.EntityPerson}

type job struct {
	svc       *catalog.Service
	jobs      *store.SyncJobStore
	purgeable []store.Purgeable
	failed    bool
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	overrideStart, overrideEnd := parseDateFlags()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	ctx := context.Background()

	pool, err := store.NewPool(ctx, cfg.DatabaseURL, store.PoolConfig{
		MaxConns: cfg.DbMaxConns,
		MinConns: cfg.DbMinConns,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer pool.Close()

	j := newJob(pool, cfg)
	for _, entity := range entities {
		j.syncChanges(ctx, entity, overrideStart, overrideEnd)
	}
	j.refresh(ctx)
	j.hydrateNew(ctx, cfg.SyncHydrateBudget)
	j.purge(ctx)
	j.logStats(ctx)

	if j.failed {
		log.Error().Msg("Sync job completed with errors")
		pool.Close()
		os.Exit(1)
	}
	log.Info().Msg("Sync job completed successfully")
}

func newJob(pool *pgxpool.Pool, cfg *config.Config) *job {
	movies := store.NewMovieStore(pool)
	series := store.NewSeriesStore(pool)
	seasons := store.NewSeasonStore(pool)
	episodes := store.NewEpisodeStore(pool)
	people := store.NewPersonStore(pool)
	collections := store.NewCollectionStore(pool)
	svc := catalog.New(catalog.Deps{
		Pool:        pool,
		Movies:      movies,
		Series:      series,
		Seasons:     seasons,
		Episodes:    episodes,
		People:      people,
		Collections: collections,
		TMDB: tmdb.NewClient(tmdb.Config{
			BaseURL:     cfg.TmdbBaseUrl,
			Timeout:     cfg.TmdbTimeout,
			AccessToken: cfg.TmdbAccessToken,
			RateLimit:   cfg.TmdbRateLimit,
		}),
	})
	return &job{
		svc:  svc,
		jobs: store.NewSyncJobStore(pool),
		purgeable: []store.Purgeable{
			movies, series, seasons, episodes, collections, people,
			store.NewSessionStore(pool),
			store.NewAuthCodeStore(pool, nil),
			store.NewNotificationSeenStore(pool),
		},
	}
}

// syncChanges runs one entity's changes window as a sync job, so a finished run becomes the next
// run's starting point and a failed one is retried from the same place.
func (j *job) syncChanges(ctx context.Context, entity catalog.Entity, start, end string) {
	var err error
	if start == "" {
		if start, end, err = j.svc.ChangeWindow(ctx, j.jobs, entity); err != nil {
			j.fail(err, entity, "could not resolve the changes window")
			return
		}
	}
	run, err := j.jobs.StartJob(ctx, entity.SyncJobType(), start, end)
	if err != nil {
		j.fail(err, entity, "could not start the sync job")
		return
	}
	changed, marked, err := j.svc.SyncChanges(ctx, entity, start, end)
	if err != nil {
		if failErr := j.jobs.FailJob(ctx, run.ID, err.Error()); failErr != nil {
			log.Error().Err(failErr).Msg("could not record the failed sync job")
		}
		j.fail(err, entity, "changes sync failed")
		return
	}
	if err := j.jobs.CompleteJob(ctx, run.ID, changed, nil, marked); err != nil {
		j.fail(err, entity, "could not record the completed sync job")
		return
	}
	log.Info().Str("entity", entity.String()).Str("start", start).Str("end", end).
		Int("changed", changed).Int64("flagged", marked).Msg("changes flagged")
}

// refresh refetches every row the changes feed flagged and every row nearing the six-month cap.
func (j *job) refresh(ctx context.Context) {
	for _, entity := range entities {
		j.hydrate(ctx, catalog.HydrateOptions{
			Entity:  entity,
			Classes: []store.WorkClass{store.WorkStale, store.WorkExpiring},
		})
	}
}

// hydrateNew spends the budget on the most popular skeletons, table by table.
func (j *job) hydrateNew(ctx context.Context, budget int) {
	remaining := budget
	for _, entity := range entities {
		if remaining <= 0 {
			return
		}
		stats := j.hydrate(ctx, catalog.HydrateOptions{
			Entity:        entity,
			Budget:        remaining,
			Classes:       []store.WorkClass{store.WorkUnhydrated},
			MinPopularity: store.MinHydratePopularity,
		})
		remaining -= stats.Attempted
	}
}

// hydrate runs one hydration and logs its summary; a run that trips the failure streak is an error
// for the job but does not stop the steps after it.
func (j *job) hydrate(ctx context.Context, opts catalog.HydrateOptions) catalog.HydrateStats {
	opts.OnRow = func(r catalog.RowResult) {
		if r.Outcome == catalog.OutcomeFailed {
			log.Warn().Err(r.Err).Str("entity", opts.Entity.String()).Int("source_id", r.SourceID).Msg("row failed")
		}
	}
	stats, err := j.svc.Hydrate(ctx, opts)
	if err != nil {
		j.fail(err, opts.Entity, "hydration stopped")
	}
	log.Info().Str("entity", opts.Entity.String()).Int("attempted", stats.Attempted).Int("ok", stats.OK).
		Int("gone", stats.Gone).Int("failed", stats.Failed).Int("people_fetched", stats.PeopleFetched).
		Msg("hydration finished")
	return stats
}

func (j *job) purge(ctx context.Context) {
	for _, table := range j.purgeable {
		count, err := table.DeleteExpired(ctx)
		if err != nil {
			log.Error().Err(err).Str("table", table.TableName()).Msg("purge failed")
			j.failed = true
			continue
		}
		log.Info().Str("table", table.TableName()).Int64("rows_purged", count).Msg("purged expired rows")
	}
}

func (j *job) logStats(ctx context.Context) {
	all, err := j.svc.Stats(ctx)
	if err != nil {
		log.Error().Err(err).Msg("stats failed")
		j.failed = true
		return
	}
	for _, st := range all {
		log.Info().Str("table", st.Table).Int("rows", st.Rows).Int("hydrated", st.Hydrated).
			Int("stale", st.Stale).Int("expiring", st.Expiring).Int("oldest_days", st.OldestDays).Msg("table stats")
	}
}

func (j *job) fail(err error, entity catalog.Entity, msg string) {
	log.Error().Err(err).Str("entity", entity.String()).Msg(msg)
	j.failed = true
}

func parseDateFlags() (startDate, endDate string) {
	flag.StringVar(&startDate, "start", os.Getenv("SYNC_START_DATE"), "Start date override (YYYY-MM-DD)")
	flag.StringVar(&endDate, "end", os.Getenv("SYNC_END_DATE"), "End date override (YYYY-MM-DD)")
	flag.Parse()

	if (startDate == "") != (endDate == "") {
		log.Fatal().Msg("-start and -end go together")
	}
	if startDate != "" {
		if _, err := time.Parse(time.DateOnly, startDate); err != nil {
			log.Fatal().Str("start", startDate).Msg("Invalid start date format, expected YYYY-MM-DD")
		}
	}
	if endDate != "" {
		if _, err := time.Parse(time.DateOnly, endDate); err != nil {
			log.Fatal().Str("end", endDate).Msg("Invalid end date format, expected YYYY-MM-DD")
		}
	}
	if startDate != "" && endDate != "" && startDate > endDate {
		log.Fatal().
			Str("start", startDate).
			Str("end", endDate).
			Msg("Start date must not be after end date")
	}

	return startDate, endDate
}

func init() {
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
}
