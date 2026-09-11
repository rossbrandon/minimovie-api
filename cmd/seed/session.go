package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type session struct {
	term    *terminal
	catalog *catalog.Service
	pool    *pgxpool.Pool
	logFile *os.File
}

type sessionOptions struct {
	dir       string
	entity    catalog.Entity
	rateLimit float64 // source API requests per second
}

func openSession(ctx context.Context, o sessionOptions) (*session, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	term := newTerminal(os.Stderr)
	logFile, err := openLogFile(o.dir, o.entity, term)
	if err != nil {
		return nil, err
	}
	svc, pool, err := connectCatalog(ctx, cfg, o.rateLimit)
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	return &session{term: term, catalog: svc, pool: pool, logFile: logFile}, nil
}

func (s *session) close() {
	s.term.finish()
	s.pool.Close()
	_ = s.logFile.Close()
}

// connectCatalog opens the database pool and wires the catalog service to a rate-limited TMDB client.
func connectCatalog(ctx context.Context, cfg *config.Config, rateLimit float64) (*catalog.Service, *pgxpool.Pool, error) {
	pool, err := store.NewPool(ctx, cfg.DatabaseURL, store.PoolConfig{MaxConns: cfg.DbMaxConns, MinConns: cfg.DbMinConns})
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	client := tmdb.NewClient(tmdb.Config{
		BaseURL:     cfg.TmdbBaseUrl,
		Timeout:     cfg.TmdbTimeout,
		AccessToken: cfg.TmdbAccessToken,
		RateLimit:   rateLimit,
	})
	svc := catalog.New(catalog.Deps{
		Pool:        pool,
		Movies:      store.NewMovieStore(pool),
		Series:      store.NewSeriesStore(pool),
		Seasons:     store.NewSeasonStore(pool),
		Episodes:    store.NewEpisodeStore(pool),
		People:      store.NewPersonStore(pool),
		Collections: store.NewCollectionStore(pool),
		TMDB:        client,
	})
	return svc, pool, nil
}

func openLogFile(dir string, entity catalog.Entity, term *terminal) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("seed-%s.log", entity))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	console := zerolog.ConsoleWriter{Out: term, TimeFormat: "15:04:05"}
	consoleInfo := &zerolog.FilteredLevelWriter{Writer: zerolog.LevelWriterAdapter{Writer: console}, Level: zerolog.InfoLevel}
	log.Logger = zerolog.New(zerolog.MultiLevelWriter(f, consoleInfo)).Level(zerolog.DebugLevel).With().Timestamp().Logger()
	return f, nil
}
