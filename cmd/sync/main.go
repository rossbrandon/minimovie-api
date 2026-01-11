package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const jobType = "person_sync"

type SyncResult struct {
	TmdbChangeCount int
	ChangedIDs      []int
	UpdatedCount    int64
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	overrideStart, overrideEnd := parseDateFlags()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	ctx := context.Background()

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer pool.Close()

	personStore := store.NewPersonStore(pool)
	syncJobStore := store.NewSyncJobStore(pool)

	startDate, endDate := resolveDates(ctx, syncJobStore, overrideStart, overrideEnd)

	log.Info().
		Str("start_date", startDate).
		Str("end_date", endDate).
		Msg("Starting person sync job")

	job, err := syncJobStore.StartJob(ctx, jobType, startDate, endDate)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start sync job")
	}

	tmdbClient := tmdb.NewClient(tmdb.Config{
		BaseURL:     cfg.TmdbBaseUrl,
		Timeout:     cfg.TmdbTimeout,
		AccessToken: cfg.TmdbAccessToken,
	})

	result, err := syncPersonChanges(ctx, tmdbClient, personStore, startDate, endDate)
	if err != nil {
		if failErr := syncJobStore.FailJob(ctx, job.ID, err.Error()); failErr != nil {
			log.Error().Err(failErr).Msg("Failed to record job failure")
		}
		log.Fatal().Err(err).Msg("Sync job failed")
	}

	err = syncJobStore.CompleteJob(
		ctx,
		job.ID,
		result.TmdbChangeCount,
		result.ChangedIDs,
		result.UpdatedCount,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to record job completion")
	}

	log.Info().Msg("Sync job completed successfully")
}

func parseDateFlags() (startDate, endDate string) {
	flag.StringVar(&startDate, "start", os.Getenv("SYNC_START_DATE"), "Start date override (YYYY-MM-DD)")
	flag.StringVar(&endDate, "end", os.Getenv("SYNC_END_DATE"), "End date override (YYYY-MM-DD)")
	flag.Parse()

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

func resolveDates(ctx context.Context, syncJobStore *store.SyncJobStore, overrideStart, overrideEnd string) (startDate, endDate string) {
	if overrideStart != "" && overrideEnd != "" {
		return overrideStart, overrideEnd
	}

	endDate = time.Now().UTC().Format(time.DateOnly)

	lastJob, err := syncJobStore.GetLastSuccessfulJob(ctx, jobType)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get last successful job, using default start date")
		startDate = time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
		return startDate, endDate
	}

	if lastJob != nil {
		startDate = lastJob.EndDate
	} else {
		startDate = time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	}

	return startDate, endDate
}

func syncPersonChanges(
	ctx context.Context,
	tmdbClient *tmdb.Client,
	personStore *store.PersonStore,
	startDate, endDate string,
) (*SyncResult, error) {
	changedIDs, err := tmdbClient.GetPersonChanges(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}

	if len(changedIDs) == 0 {
		log.Info().Msg("No person changes found")
		return &SyncResult{}, nil
	}

	affected, err := personStore.MarkPeopleStale(ctx, changedIDs)
	if err != nil {
		return nil, err
	}

	log.Info().
		Int("change_count", len(changedIDs)).
		Int64("marked_stale_count", affected).
		Str("start_date", startDate).
		Str("end_date", endDate).
		Msg("Person changes synced")

	return &SyncResult{
		TmdbChangeCount: len(changedIDs),
		ChangedIDs:      changedIDs,
		UpdatedCount:    affected,
	}, nil
}

func init() {
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
}
