package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
)

type Config struct {
	Port                   string
	Timeout                int
	TmdbBaseUrl            string
	TmdbTimeout            int
	TmdbAccessToken        string
	MiniMovieUiSecret      string
	DatabaseURL            string
	MaxTmdbFetchPerRequest int
	OTelEnabled            bool
}

const defaultPort = "8080"
const defaultTimeout int = 10
const defaultTmdbBaseUrl = "https://api.themoviedb.org/3"
const defaultTmdbTimeout int = 10
const defaultMaxTmdbFetchPerRequest int = 10

func Load() (*Config, error) {
	tmdbAccessToken := os.Getenv("TMDB_ACCESS_TOKEN")
	if tmdbAccessToken == "" {
		return nil, errors.New("TMDB_ACCESS_TOKEN is not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		log.Warn().Msg("PORT is not set, using default port")
		port = defaultPort
	}

	timeoutStr := os.Getenv("TIMEOUT")
	timeout := defaultTimeout
	if timeoutStr == "" {
		log.Warn().Msg(fmt.Sprintf("TIMEOUT is not set, using default timeout of %d seconds", defaultTimeout))
	} else {
		timeoutInt, err := strconv.Atoi(timeoutStr)
		if err != nil {
			return nil, errors.New("TIMEOUT is not a valid integer")
		}
		timeout = timeoutInt
	}

	tmdbBaseUrl := os.Getenv("TMDB_BASE_URL")
	if tmdbBaseUrl == "" {
		log.Warn().Msg(fmt.Sprintf("TMDB_BASE_URL is not set, using default base URL of %s", defaultTmdbBaseUrl))
		tmdbBaseUrl = defaultTmdbBaseUrl
	}

	tmdbTimeoutStr := os.Getenv("TMDB_TIMEOUT")
	tmdbTimeout := defaultTmdbTimeout
	if tmdbTimeoutStr == "" {
		log.Warn().Msg(fmt.Sprintf("TMDB_TIMEOUT is not set, using default timeout of %d seconds", defaultTmdbTimeout))
	} else {
		tmdbTimeoutInt, err := strconv.Atoi(tmdbTimeoutStr)
		if err != nil {
			return nil, errors.New("TMDB_TIMEOUT is not a valid integer")
		}
		tmdbTimeout = tmdbTimeoutInt
	}

	miniMovieUiSecret := os.Getenv("MINI_MOVIE_UI_SECRET")

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is not set")
	}

	maxTmdbFetchPerRequestStr := os.Getenv("MAX_TMDB_FETCH_PER_REQUEST")
	maxTmdbFetchPerRequest := defaultMaxTmdbFetchPerRequest
	if maxTmdbFetchPerRequestStr != "" {
		maxTmdbFetchPerRequestInt, err := strconv.Atoi(maxTmdbFetchPerRequestStr)
		if err != nil {
			return nil, errors.New("MAX_TMDB_FETCH_PER_REQUEST is not a valid integer")
		}
		maxTmdbFetchPerRequest = maxTmdbFetchPerRequestInt
	}

	otelEnabled := os.Getenv("OTEL_ENABLED") == "true"
	otelEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	otelHeaders := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")

	if otelEnabled && (otelEndpoint == "" || otelHeaders == "") {
		log.Warn().Msg("OTEL_ENABLED is true but OTEL_EXPORTER_OTLP_ENDPOINT or OTEL_EXPORTER_OTLP_HEADERS is not set, disabling metrics")
		otelEnabled = false
	}

	return &Config{
		Port:                   port,
		Timeout:                timeout,
		TmdbBaseUrl:            tmdbBaseUrl,
		TmdbTimeout:            tmdbTimeout,
		TmdbAccessToken:        tmdbAccessToken,
		MiniMovieUiSecret:      miniMovieUiSecret,
		DatabaseURL:            databaseURL,
		MaxTmdbFetchPerRequest: maxTmdbFetchPerRequest,
		OTelEnabled:            otelEnabled,
	}, nil
}
