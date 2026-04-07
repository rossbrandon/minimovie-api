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
	LogLevel               string
	TmdbBaseUrl            string
	TmdbTimeout            int
	TmdbAccessToken        string
	MiniMovieUiSecret      string
	DatabaseURL            string
	MaxTmdbFetchPerRequest int
	OTelEnabled            bool
	CacheMaxAge            int
	AnthropicApiKey        string
	AugurModel             string
	AugurMaxTokens         int
	AugurMaxRetries        int
	AugurMinConfidence     float64
	AugurTimeout           int
}

const defaultPort = "8080"
const defaultTimeout int = 10
const defaultLogLevel = "info"
const defaultTmdbBaseUrl = "https://api.themoviedb.org/3"
const defaultTmdbTimeout int = 10
const defaultMaxTmdbFetchPerRequest int = 10
const defaultCacheMaxAge int = 3600
const defaultAugurModel = "claude-sonnet-4-6"
const defaultAugurMaxTokens int = 4096
const defaultAugurMaxRetries int = 1
const defaultAugurMinConfidence float64 = 0.65
const defaultAugurTimeout int = 60

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

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = defaultLogLevel
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

	cacheMaxAgeStr := os.Getenv("CACHE_MAX_AGE")
	cacheMaxAge := defaultCacheMaxAge
	if cacheMaxAgeStr != "" {
		cacheMaxAgeInt, err := strconv.Atoi(cacheMaxAgeStr)
		if err != nil {
			return nil, errors.New("CACHE_MAX_AGE is not a valid integer")
		}
		cacheMaxAge = cacheMaxAgeInt
	}

	anthropicApiKey := os.Getenv("ANTHROPIC_API_KEY")

	augurModel := os.Getenv("AUGUR_MODEL")
	if augurModel == "" {
		augurModel = defaultAugurModel
	}

	augurMaxTokensStr := os.Getenv("AUGUR_MAX_TOKENS")
	augurMaxTokens := defaultAugurMaxTokens
	if augurMaxTokensStr != "" {
		augurMaxTokensInt, err := strconv.Atoi(augurMaxTokensStr)
		if err != nil {
			return nil, errors.New("AUGUR_MAX_TOKENS is not a valid integer")
		}
		augurMaxTokens = augurMaxTokensInt
	}

	augurMaxRetriesStr := os.Getenv("AUGUR_MAX_RETRIES")
	augurMaxRetries := defaultAugurMaxRetries
	if augurMaxRetriesStr != "" {
		augurMaxRetriesInt, err := strconv.Atoi(augurMaxRetriesStr)
		if err != nil {
			return nil, errors.New("AUGUR_MAX_RETRIES is not a valid integer")
		}
		augurMaxRetries = augurMaxRetriesInt
	}

	augurMinConfidenceStr := os.Getenv("AUGUR_MIN_CONFIDENCE")
	augurMinConfidence := defaultAugurMinConfidence
	if augurMinConfidenceStr != "" {
		augurMinConfidenceFloat, err := strconv.ParseFloat(augurMinConfidenceStr, 64)
		if err != nil {
			return nil, errors.New("AUGUR_MIN_CONFIDENCE is not a valid float")
		}
		augurMinConfidence = augurMinConfidenceFloat
	}

	augurTimeoutStr := os.Getenv("AUGUR_TIMEOUT")
	augurTimeout := defaultAugurTimeout
	if augurTimeoutStr != "" {
		augurTimeoutInt, err := strconv.Atoi(augurTimeoutStr)
		if err != nil {
			return nil, errors.New("AUGUR_TIMEOUT is not a valid integer")
		}
		augurTimeout = augurTimeoutInt
	}

	return &Config{
		Port:                   port,
		Timeout:                timeout,
		LogLevel:               logLevel,
		TmdbBaseUrl:            tmdbBaseUrl,
		TmdbTimeout:            tmdbTimeout,
		TmdbAccessToken:        tmdbAccessToken,
		MiniMovieUiSecret:      miniMovieUiSecret,
		DatabaseURL:            databaseURL,
		MaxTmdbFetchPerRequest: maxTmdbFetchPerRequest,
		OTelEnabled:            otelEnabled,
		CacheMaxAge:            cacheMaxAge,
		AnthropicApiKey:        anthropicApiKey,
		AugurModel:             augurModel,
		AugurMaxTokens:         augurMaxTokens,
		AugurMaxRetries:        augurMaxRetries,
		AugurMinConfidence:     augurMinConfidence,
		AugurTimeout:           augurTimeout,
	}, nil
}
