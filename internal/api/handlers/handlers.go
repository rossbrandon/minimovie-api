package handlers

import (
	"context"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/achievements"
	"github.com/rossbrandon/minimovie-api/internal/augur"
	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/background"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Catalog interface {
	Movie(ctx context.Context, id int) (*catalog.Movie, error)
	Collection(ctx context.Context, id int) (*catalog.Collection, error)
	Series(ctx context.Context, id int) (*catalog.Series, error)
	Season(ctx context.Context, seriesID, seasonNumber int) (*catalog.Season, error)
	Seasons(ctx context.Context, seriesID int, numbers []int) (map[int]*tmdb.SeasonDetails, error)
	Episode(ctx context.Context, seriesID, seasonNumber, episodeNumber int) (*catalog.Episode, error)
	Person(ctx context.Context, id int) (*catalog.Person, error)
	SeedSearch(ctx context.Context, results *tmdb.SearchResults) (*catalog.SearchRefs, error)
	ResolveMovie(ctx context.Context, id int) (store.ResolvedMedia, error)
	ResolveSeries(ctx context.Context, id int) (store.ResolvedMedia, error)
	ResolveSeason(ctx context.Context, seriesID, seasonNumber int) (store.ResolvedMedia, error)
	ResolveEpisode(ctx context.Context, seriesID, seasonNumber, episodeNumber int) (store.ResolvedMedia, error)
}

type Handlers struct {
	cfg *config.Config

	tmdbClient    tmdb.MediaClient
	catalog       Catalog
	bg            *background.Group
	augurResolver *augur.Resolver

	providers         auth.ProviderRegistry
	userStore         store.UserRepository
	sessionStore      store.SessionRepository
	authCodeStore     store.AuthCodeRepository
	notificationStore store.NotificationSeenRepository
	watchlistStore    store.WatchlistRepository
	watchEventStore   store.WatchEventRepository
	achievementStore  store.AchievementRepository
	statsStore        store.StatsRepository

	achievementWorker *achievements.Worker
}

type HandlerDeps struct {
	Cfg *config.Config

	TmdbClient    tmdb.MediaClient
	Catalog       Catalog
	BG            *background.Group
	AugurResolver *augur.Resolver

	Providers         auth.ProviderRegistry
	UserStore         store.UserRepository
	SessionStore      store.SessionRepository
	AuthCodeStore     store.AuthCodeRepository
	NotificationStore store.NotificationSeenRepository
	WatchlistStore    store.WatchlistRepository
	WatchEventStore   store.WatchEventRepository
	AchievementStore  store.AchievementRepository
	StatsStore        store.StatsRepository

	AchievementWorker *achievements.Worker
}

func NewHandlers(deps HandlerDeps) *Handlers {
	return &Handlers{
		cfg:               deps.Cfg,
		tmdbClient:        deps.TmdbClient,
		catalog:           deps.Catalog,
		bg:                deps.BG,
		augurResolver:     deps.AugurResolver,
		providers:         deps.Providers,
		userStore:         deps.UserStore,
		sessionStore:      deps.SessionStore,
		authCodeStore:     deps.AuthCodeStore,
		notificationStore: deps.NotificationStore,
		watchlistStore:    deps.WatchlistStore,
		watchEventStore:   deps.WatchEventStore,
		achievementStore:  deps.AchievementStore,
		statsStore:        deps.StatsStore,
		achievementWorker: deps.AchievementWorker,
	}
}

func (h *Handlers) resolveMedia(ctx context.Context, mediaType string, mediaID int) (store.ResolvedMedia, error) {
	switch mediaType {
	case "movie":
		return h.catalog.ResolveMovie(ctx, mediaID)
	case "series":
		return h.catalog.ResolveSeries(ctx, mediaID)
	default:
		return store.ResolvedMedia{}, catalog.ErrNotFound
	}
}
