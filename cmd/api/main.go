package main

import (
	"net/http"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/api"
	"github.com/rossbrandon/minimovie-api/internal/api/handlers"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	log.Info().Msg("Starting MiniMovie API")

	config, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	tmdbClient := tmdb.NewClient(tmdb.Config{
		BaseURL:     config.TmdbBaseUrl,
		Timeout:     config.TmdbTimeout,
		AccessToken: config.TmdbAccessToken,
	})

	handlers := handlers.NewHandlers(tmdbClient)

	r := api.NewRouter(handlers, config)
	log.Info().Msg("Server is listening on port " + config.Port)
	err = http.ListenAndServe(":"+config.Port, r)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}
