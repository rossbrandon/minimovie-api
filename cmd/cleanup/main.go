package main

import (
	"context"
	"os"
	"time"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const jobType = "cache_cleanup"

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

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

	syncJobStore := store.NewSyncJobStore(pool)

	// Register all purgeable stores — add new cache stores here as they're created
	stores := []store.Purgeable{
		store.NewSeasonCastPostgresStore(pool),
	}

	today := time.Now().UTC().Format(time.DateOnly)

	job, err := syncJobStore.StartJob(ctx, jobType, today, today)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start cleanup job")
	}

	totalPurged := int64(0)
	var jobErr error

	for _, s := range stores {
		count, err := s.PurgeExpired(ctx)
		if err != nil {
			log.Error().Err(err).Str("table", s.TableName()).Msg("Failed to purge expired records")
			jobErr = err
			continue
		}

		log.Info().
			Str("table", s.TableName()).
			Int64("rows_purged", count).
			Msg("Purged expired cache records")

		totalPurged += count
	}

	if jobErr != nil {
		if failErr := syncJobStore.FailJob(ctx, job.ID, jobErr.Error()); failErr != nil {
			log.Error().Err(failErr).Msg("Failed to record job failure")
		}
		log.Fatal().Msg("Cleanup job completed with errors")
	}

	if err := syncJobStore.CompleteJob(ctx, job.ID, len(stores), nil, totalPurged); err != nil {
		log.Error().Err(err).Msg("Failed to record job completion")
	}

	log.Info().
		Int64("total_purged", totalPurged).
		Int("tables_processed", len(stores)).
		Msg("Cleanup job completed successfully")
}

func init() {
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
}
