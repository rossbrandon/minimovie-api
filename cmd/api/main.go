package main

import (
	"context"
	"net/http"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/api"
	"github.com/rossbrandon/minimovie-api/internal/api/handlers"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	log.Info().Msg("Starting MiniMovie API")

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

	ageResolver, err := age.New(ctx, personStore, tmdbClient, age.Config{
		MaxFetchPerRequest: cfg.MaxTMDBFetchPerRequest,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create age resolver")
	}

	h := handlers.NewHandlers(tmdbClient, ageResolver)

	r := api.NewRouter(h, cfg)
	log.Info().Msg("Server is listening on port " + cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}
