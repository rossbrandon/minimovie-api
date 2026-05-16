package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/achievements"
	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/api"
	"github.com/rossbrandon/minimovie-api/internal/api/handlers"
	"github.com/rossbrandon/minimovie-api/internal/augur"
	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rossbrandon/minimovie-api/internal/series"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	// Configure logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if level, err := zerolog.ParseLevel(cfg.LogLevel); err == nil {
		zerolog.SetGlobalLevel(level)
	}

	validateConfig(cfg)

	log.Info().Msg("Starting MiniMovie API")

	ctx := context.Background()

	// Initialize metrics
	metricsShutdown, err := metrics.Init(ctx, metrics.Config{
		Enabled: cfg.OTelEnabled,
	})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to initialize metrics, continuing without")
	} else {
		defer func() { _ = metricsShutdown(ctx) }()
	}

	// Initialize database connection
	pool, err := store.NewPool(ctx, cfg.DatabaseURL, store.PoolConfig{
		MaxConns: cfg.DbMaxConns,
		MinConns: cfg.DbMinConns,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer pool.Close()

	if metrics.M != nil {
		if err := metrics.M.RegisterDbPoolGauges(pool); err != nil {
			log.Warn().Err(err).Msg("Failed to register db pool gauges")
		}
	}

	// Initialize stores
	personStore := store.NewPersonStore(pool)
	userStore := store.NewUserStore(pool)
	sessionStore := store.NewSessionStore(pool)
	authCodeStore := store.NewAuthCodeStore(pool, cfg.TokenEncryptionKey)
	notificationStore := store.NewNotificationSeenStore(pool)
	watchlistStore := store.NewWatchlistStore(pool)
	watchEventStore := store.NewWatchEventStore(pool)
	achievementStore := store.NewAchievementStore(pool)
	statsStore := store.NewStatsStore(pool)
	seriesMetadataStore := store.NewSeriesMetadataStore(pool)

	seasonCastCache, err := store.NewSeasonCastBigCacheAdapter(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create season cast BigCache")
	}
	seasonCastStore := store.NewSeasonCastPostgresStore(pool)
	seasonCastTiered := store.NewSeasonCastTieredCache(seasonCastCache, seasonCastStore)

	// Initialize TMDB client and resolver
	tmdbClient := tmdb.NewClient(tmdb.Config{
		BaseURL:     cfg.TmdbBaseUrl,
		Timeout:     cfg.TmdbTimeout,
		AccessToken: cfg.TmdbAccessToken,
	})
	tmdbResolver := tmdb.NewMetadataResolver(tmdbClient)

	seriesService, err := series.New(ctx, seriesMetadataStore, tmdbClient)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create series cache service")
	}

	// Initialize data enrichments
	ageResolver, err := age.New(ctx, personStore, tmdbClient, age.Config{
		MaxFetchPerRequest: cfg.MaxTmdbFetchPerRequest,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create age resolver")
	}

	// Initialize interesting info enrichment
	interestingInfoStore := store.NewInterestingInfoStore(pool)

	var augurResolver *augur.Resolver
	if cfg.AnthropicApiKey != "" {
		augurResolver = augur.New(interestingInfoStore, augur.Config{
			ApiKey:        cfg.AnthropicApiKey,
			Model:         cfg.AugurModel,
			MaxTokens:     cfg.AugurMaxTokens,
			MaxRetries:    cfg.AugurMaxRetries,
			MinConfidence: cfg.AugurMinConfidence,
		})
		log.Info().Msg("Augur enrichment enabled")
	} else {
		log.Warn().Msg("ANTHROPIC_API_KEY not set, augur enrichment disabled")
	}

	// Initialize OIDC providers
	oidcProviders, err := auth.NewOIDCProviders(ctx, auth.ProviderConfig{
		GoogleIssuerURL:    cfg.GoogleIssuerURL,
		GoogleClientID:     cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		AppleIssuerURL:     cfg.AppleIssuerURL,
		AppleClientID:      cfg.AppleClientID,
		AppleTeamID:        cfg.AppleTeamID,
		AppleKeyID:         cfg.AppleKeyID,
		ApplePrivateKey:    cfg.ApplePrivateKey,
		BaseURL:            cfg.AuthBaseURL,
		CookieHashKey:      cfg.CookieHashKey,
		CookieEncKey:       cfg.CookieEncKey,
		IsProduction:       cfg.IsProduction,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize OIDC providers")
	}

	// Initialize achievement worker
	achievementWorker := achievements.NewWorker(achievementStore, watchlistStore, watchEventStore, 8)
	defer achievementWorker.Stop()

	// Initialize API server
	httputil.DefaultCacheMaxAge = cfg.CacheMaxAge
	h := handlers.NewHandlers(handlers.HandlerDeps{
		Cfg:                 cfg,
		TmdbClient:          tmdbClient,
		TmdbResolver:        tmdbResolver,
		AgeResolver:         ageResolver,
		SeasonCastCache:     seasonCastTiered,
		SeriesService:       seriesService,
		AugurResolver:       augurResolver,
		Providers:           oidcProviders,
		UserStore:           userStore,
		SessionStore:        sessionStore,
		AuthCodeStore:       authCodeStore,
		NotificationStore:   notificationStore,
		WatchlistStore:      watchlistStore,
		WatchEventStore:     watchEventStore,
		AchievementStore:    achievementStore,
		StatsStore:          statsStore,
		SeriesMetadataStore: seriesMetadataStore,
		AchievementWorker:   achievementWorker,
	})

	r := api.NewRouter(h, cfg, sessionStore)
	log.Info().Msg("Server is listening on port " + cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

func validateConfig(cfg *config.Config) {
	if cfg.MiniMovieUiSecret == "" {
		log.Fatal().Msg("MINI_MOVIE_UI_SECRET must be set")
	}
	if len(cfg.SessionSecret) < 32 {
		log.Fatal().Msg("SESSION_SECRET must be set and at least 32 bytes")
	}
	if len(cfg.TokenEncryptionKey) != 32 {
		log.Fatal().Msg("TOKEN_ENCRYPTION_KEY must be set and exactly 32 bytes")
	}
	if cfg.GoogleClientID == "" && cfg.AppleClientID == "" {
		log.Fatal().Msg("At least one OAuth provider (GOOGLE_CLIENT_ID or APPLE_CLIENT_ID) must be configured")
	}
	if cfg.AuthBaseURL == "" {
		log.Fatal().Str("auth_base_url", cfg.AuthBaseURL).Msg("AUTH_BASE_URL must be set")
	}
	if cfg.AuthUIBaseURL == "" {
		log.Fatal().Str("auth_ui_base_url", cfg.AuthUIBaseURL).Msg("AUTH_UI_BASE_URL must be set")
	}

	// Production validations
	if cfg.IsProduction {
		if !strings.HasPrefix(cfg.AuthBaseURL, "https://") {
			log.Fatal().Str("auth_base_url", cfg.AuthBaseURL).Msg("AUTH_BASE_URL must use HTTPS in production")
		}
		if !strings.HasPrefix(cfg.AuthUIBaseURL, "https://") {
			log.Fatal().Str("auth_ui_base_url", cfg.AuthUIBaseURL).Msg("AUTH_UI_BASE_URL must use HTTPS in production")
		}
	}
}
