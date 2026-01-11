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

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	startDate, endDate := parseDateFlags()

	log.Info().Str("start_date", startDate).Str("end_date", endDate).Msg("Starting person sync job")

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

	tmdbClient := tmdb.NewClient(tmdb.Config{
		BaseURL:     cfg.TmdbBaseUrl,
		Timeout:     cfg.TmdbTimeout,
		AccessToken: cfg.TmdbAccessToken,
	})

	if err := syncPersonChanges(ctx, tmdbClient, personStore, startDate, endDate); err != nil {
		log.Fatal().Err(err).Msg("Sync job failed")
	}

	log.Info().Msg("Sync job completed successfully")
}

func parseDateFlags() (startDate, endDate string) {
	defaultStart := time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	defaultEnd := time.Now().UTC().Format(time.DateOnly)

	flag.StringVar(&startDate, "start", getEnvOrDefault("SYNC_START_DATE", defaultStart), "Start date (YYYY-MM-DD)")
	flag.StringVar(&endDate, "end", getEnvOrDefault("SYNC_END_DATE", defaultEnd), "End date (YYYY-MM-DD)")
	flag.Parse()

	return startDate, endDate
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func syncPersonChanges(ctx context.Context, tmdbClient *tmdb.Client, personStore *store.PersonStore, startDate, endDate string) error {
	changedIDs, err := tmdbClient.GetPersonChanges(ctx, startDate, endDate)
	if err != nil {
		return err
	}

	if len(changedIDs) == 0 {
		log.Info().Msg("No person changes found")
		return nil
	}

	affected, err := personStore.MarkPeopleStale(ctx, changedIDs)
	if err != nil {
		return err
	}

	log.Info().
		Int("change_count", len(changedIDs)).
		Int64("marked_stale_count", affected).
		Str("start_date", startDate).
		Str("end_date", endDate).
		Msg("Person changes synced")

	return nil
}

func init() {
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
}
