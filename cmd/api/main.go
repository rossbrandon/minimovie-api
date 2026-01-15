package main

import (
	"context"
	"net/http"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/api"
	"github.com/rossbrandon/minimovie-api/internal/api/handlers"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	if level, err := zerolog.ParseLevel(cfg.LogLevel); err == nil {
		zerolog.SetGlobalLevel(level)
	}

	log.Info().Msg("Starting MiniMovie API")

	ctx := context.Background()

	// Initialize metrics
	metricsShutdown, err := metrics.Init(ctx, metrics.Config{
		Enabled: cfg.OTelEnabled,
	})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to initialize metrics, continuing without")
	} else {
		defer metricsShutdown(ctx)
	}

	// Initialize database connection
	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer pool.Close()

	// Initialize BigCache stores
	personStore := store.NewPersonStore(pool)

	// Initialize TMDB client
	tmdbClient := tmdb.NewClient(tmdb.Config{
		BaseURL:     cfg.TmdbBaseUrl,
		Timeout:     cfg.TmdbTimeout,
		AccessToken: cfg.TmdbAccessToken,
	})

	// Initialize data enrichments
	ageResolver, err := age.New(ctx, personStore, tmdbClient, age.Config{
		MaxFetchPerRequest: cfg.MaxTmdbFetchPerRequest,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create age resolver")
	}

	// Initialize API server
	httputil.DefaultCacheMaxAge = cfg.CacheMaxAge
	h := handlers.NewHandlers(tmdbClient, ageResolver)

	r := api.NewRouter(h, cfg)
	log.Info().Msg("Server is listening on port " + cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}
