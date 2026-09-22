package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/achievements"
	"github.com/rossbrandon/minimovie-api/internal/api"
	"github.com/rossbrandon/minimovie-api/internal/api/handlers"
	"github.com/rossbrandon/minimovie-api/internal/augur"
	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/background"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var _ handlers.Catalog = (*catalog.Service)(nil)

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

	if err := cfg.ValidateAPI(); err != nil {
		log.Fatal().Err(err).Msg("Invalid config")
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

	if err := metrics.M.RegisterDbPoolGauges(pool); err != nil {
		log.Warn().Err(err).Msg("Failed to register db pool gauges")
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

	// Initialize TMDB client and the catalog it feeds
	tmdbClient := tmdb.NewClient(tmdb.Config{
		BaseURL:     cfg.TmdbBaseUrl,
		Timeout:     cfg.TmdbTimeout,
		AccessToken: cfg.TmdbAccessToken,
		RateLimit:   cfg.TmdbRateLimit,
	})
	var bg background.Group
	svc := catalog.New(catalog.Deps{
		Pool:               pool,
		Movies:             store.NewMovieStore(pool),
		Series:             store.NewSeriesStore(pool),
		Seasons:            store.NewSeasonStore(pool),
		Episodes:           store.NewEpisodeStore(pool),
		People:             personStore,
		Collections:        store.NewCollectionStore(pool),
		TMDB:               tmdbClient,
		BG:                 &bg,
		MaxFetchPerRequest: cfg.MaxTmdbFetchPerRequest,
	})
	svc.Start()

	// Initialize interesting info enrichment
	var augurResolver *augur.Resolver
	if cfg.AnthropicApiKey != "" {
		augurResolver = augur.New(personStore, &bg, augur.Config{
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

	// Initialize API server
	httputil.DefaultCacheMaxAge = cfg.CacheMaxAge
	h := handlers.NewHandlers(handlers.HandlerDeps{
		Cfg:               cfg,
		TmdbClient:        tmdbClient,
		Catalog:           svc,
		BG:                &bg,
		AugurResolver:     augurResolver,
		Providers:         oidcProviders,
		UserStore:         userStore,
		SessionStore:      sessionStore,
		AuthCodeStore:     authCodeStore,
		NotificationStore: notificationStore,
		WatchlistStore:    watchlistStore,
		WatchEventStore:   watchEventStore,
		AchievementStore:  achievementStore,
		StatsStore:        statsStore,
		AchievementWorker: achievementWorker,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(h, cfg, sessionStore),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serve(srv, &bg, func() {
		achievementWorker.Stop()
		svc.Stop()
	})
}

func serve(srv *http.Server, bg *background.Group, stopWorkers func()) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	failed := make(chan error, 1)
	go func() {
		log.Info().Msg("Server is listening on " + srv.Addr)
		failed <- srv.ListenAndServe()
	}()
	select {
	case err := <-failed:
		log.Fatal().Err(err).Msg("Failed to start server")
	case <-ctx.Done():
	}

	log.Info().Msg("Shutting down")
	drain, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Warn().Err(err).Msg("server shutdown did not finish cleanly")
	}
	if err := bg.Wait(drain); err != nil {
		log.Warn().Err(err).Msg("background work was still running at shutdown")
	}
	stopWorkers()
}
