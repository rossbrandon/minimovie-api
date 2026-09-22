package config

import (
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

type Config struct {
	Port                   string
	Timeout                int
	LogLevel               string
	TmdbBaseUrl            string
	TmdbTimeout            int
	TmdbAccessToken        string
	TmdbRateLimit          float64
	MiniMovieUiSecret      string
	DatabaseURL            string
	MaxTmdbFetchPerRequest int
	SyncHydrateBudget      int
	DbMaxConns             int
	DbMinConns             int
	OTelEnabled            bool
	CacheMaxAge            int
	AnthropicApiKey        string
	AugurModel             string
	AugurMaxTokens         int
	AugurMaxRetries        int
	AugurMinConfidence     float64
	AugurTimeout           int
	IsProduction           bool
	GoogleClientID         string
	GoogleClientSecret     string
	GoogleIssuerURL        string
	AppleClientID          string
	AppleTeamID            string
	AppleKeyID             string
	ApplePrivateKey        []byte
	AppleIssuerURL         string
	AuthBaseURL            string
	AuthUIBaseURL          string
	SessionSecret          []byte
	CookieHashKey          []byte
	CookieEncKey           []byte
	TokenEncryptionKey     []byte
}

const defaultPort = "8080"
const defaultTimeout int = 10
const defaultLogLevel = "info"
const defaultTmdbBaseUrl = "https://api.themoviedb.org/3"
const defaultTmdbTimeout int = 10
const defaultTmdbRateLimit float64 = 20
const defaultMaxTmdbFetchPerRequest int = 10
const defaultSyncHydrateBudget int = 2000
const defaultDbMaxConns int = 20
const defaultDbMinConns int = 5
const defaultCacheMaxAge int = 3600
const defaultAugurModel = "claude-sonnet-4-6"
const defaultAugurMaxTokens int = 4096
const defaultAugurMaxRetries int = 1
const defaultAugurMinConfidence float64 = 0.65
const defaultAugurTimeout int = 60
const defaultGoogleIssuerURL = "https://accounts.google.com"
const defaultAppleIssuerURL = "https://appleid.apple.com"

func Load() (*Config, error) {
	var errs []error
	cfg := &Config{
		Port:                   cmp.Or(os.Getenv("PORT"), defaultPort),
		Timeout:                env(&errs, "TIMEOUT", defaultTimeout, strconv.Atoi),
		LogLevel:               cmp.Or(os.Getenv("LOG_LEVEL"), defaultLogLevel),
		TmdbBaseUrl:            cmp.Or(os.Getenv("TMDB_BASE_URL"), defaultTmdbBaseUrl),
		TmdbTimeout:            env(&errs, "TMDB_TIMEOUT", defaultTmdbTimeout, strconv.Atoi),
		TmdbAccessToken:        required(&errs, "TMDB_ACCESS_TOKEN"),
		TmdbRateLimit:          env(&errs, "TMDB_RATE_LIMIT", defaultTmdbRateLimit, parsePositiveFloat),
		MiniMovieUiSecret:      os.Getenv("MINI_MOVIE_UI_SECRET"),
		DatabaseURL:            required(&errs, "DATABASE_URL"),
		MaxTmdbFetchPerRequest: env(&errs, "MAX_TMDB_FETCH_PER_REQUEST", defaultMaxTmdbFetchPerRequest, strconv.Atoi),
		SyncHydrateBudget:      env(&errs, "SYNC_HYDRATE_BUDGET", defaultSyncHydrateBudget, strconv.Atoi),
		DbMaxConns:             env(&errs, "DB_MAX_CONNS", defaultDbMaxConns, strconv.Atoi),
		DbMinConns:             env(&errs, "DB_MIN_CONNS", defaultDbMinConns, strconv.Atoi),
		OTelEnabled:            env(&errs, "OTEL_ENABLED", false, strconv.ParseBool),
		CacheMaxAge:            env(&errs, "CACHE_MAX_AGE", defaultCacheMaxAge, strconv.Atoi),
		AnthropicApiKey:        os.Getenv("ANTHROPIC_API_KEY"),
		AugurModel:             cmp.Or(os.Getenv("AUGUR_MODEL"), defaultAugurModel),
		AugurMaxTokens:         env(&errs, "AUGUR_MAX_TOKENS", defaultAugurMaxTokens, strconv.Atoi),
		AugurMaxRetries:        env(&errs, "AUGUR_MAX_RETRIES", defaultAugurMaxRetries, strconv.Atoi),
		AugurMinConfidence:     env(&errs, "AUGUR_MIN_CONFIDENCE", defaultAugurMinConfidence, parseFloat),
		AugurTimeout:           env(&errs, "AUGUR_TIMEOUT", defaultAugurTimeout, strconv.Atoi),
		IsProduction:           os.Getenv("ENV") == "production",
		GoogleClientID:         os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:     os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleIssuerURL:        cmp.Or(os.Getenv("GOOGLE_ISSUER_URL"), defaultGoogleIssuerURL),
		AppleClientID:          os.Getenv("APPLE_CLIENT_ID"),
		AppleTeamID:            os.Getenv("APPLE_TEAM_ID"),
		AppleKeyID:             os.Getenv("APPLE_KEY_ID"),
		ApplePrivateKey:        env(&errs, "APPLE_PRIVATE_KEY", nil, base64.StdEncoding.DecodeString),
		AppleIssuerURL:         cmp.Or(os.Getenv("APPLE_ISSUER_URL"), defaultAppleIssuerURL),
		AuthBaseURL:            os.Getenv("AUTH_BASE_URL"),
		AuthUIBaseURL:          os.Getenv("AUTH_UI_BASE_URL"),
		SessionSecret:          env(&errs, "SESSION_SECRET", nil, base64.StdEncoding.DecodeString),
		TokenEncryptionKey:     env(&errs, "TOKEN_ENCRYPTION_KEY", nil, base64.StdEncoding.DecodeString),
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	exporterSet := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" && os.Getenv("OTEL_EXPORTER_OTLP_HEADERS") != ""
	if cfg.OTelEnabled && !exporterSet {
		log.Warn().Msg("OTEL_ENABLED is true but the OTLP endpoint or headers are not set, disabling metrics")
		cfg.OTelEnabled = false
	}
	if len(cfg.SessionSecret) >= 32 {
		cfg.CookieHashKey = cfg.SessionSecret[:16]
		cfg.CookieEncKey = cfg.SessionSecret[16:32]
	}
	return cfg, nil
}

func (c *Config) ValidateAPI() error {
	var errs []error
	check := func(ok bool, msg string) {
		if !ok {
			errs = append(errs, errors.New(msg))
		}
	}
	check(c.MiniMovieUiSecret != "", "MINI_MOVIE_UI_SECRET must be set")
	check(len(c.SessionSecret) >= 32, "SESSION_SECRET must be set and at least 32 bytes")
	check(len(c.TokenEncryptionKey) == 32, "TOKEN_ENCRYPTION_KEY must be set and exactly 32 bytes")
	check(c.GoogleClientID != "" || c.AppleClientID != "",
		"At least one OAuth provider (GOOGLE_CLIENT_ID or APPLE_CLIENT_ID) must be configured")
	check(c.AuthBaseURL != "", "AUTH_BASE_URL must be set")
	check(c.AuthUIBaseURL != "", "AUTH_UI_BASE_URL must be set")
	if c.IsProduction {
		check(strings.HasPrefix(c.AuthBaseURL, "https://"), "AUTH_BASE_URL must use HTTPS in production")
		check(strings.HasPrefix(c.AuthUIBaseURL, "https://"), "AUTH_UI_BASE_URL must use HTTPS in production")
	}
	return errors.Join(errs...)
}

// env returns def when key is unset, otherwise parse's reading of it; a parse error is added to errs.
func env[T any](errs *[]error, key string, def T, parse func(string) (T, error)) T {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	v, err := parse(s)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return v
}

// required is env for a key that has no default.
func required(errs *[]error, key string) string {
	v := os.Getenv(key)
	if v == "" {
		*errs = append(*errs, errors.New(key+" is not set"))
	}
	return v
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func parsePositiveFloat(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("not a positive number")
	}
	return v, nil
}
