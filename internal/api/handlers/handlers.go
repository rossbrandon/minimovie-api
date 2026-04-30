package handlers

import (
	"context"

	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/achievements"
	"github.com/rossbrandon/minimovie-api/internal/age"
	"github.com/rossbrandon/minimovie-api/internal/augur"
	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
)

type Handlers struct {
	cfg *config.Config

	tmdbClient      tmdb.MediaClient
	tmdbResolver    *tmdb.MetadataResolver
	ageResolver     *age.Resolver
	seasonCastCache store.SeasonCastCache
	augurResolver   *augur.Resolver

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

	TmdbClient      tmdb.MediaClient
	TmdbResolver    *tmdb.MetadataResolver
	AgeResolver     *age.Resolver
	SeasonCastCache store.SeasonCastCache
	AugurResolver   *augur.Resolver

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
		tmdbResolver:      deps.TmdbResolver,
		ageResolver:       deps.AgeResolver,
		seasonCastCache:   deps.SeasonCastCache,
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
		return h.tmdbResolver.ResolveMovie(ctx, mediaID)
	case "series":
		return h.tmdbResolver.ResolveSeries(ctx, mediaID)
	default:
		return store.ResolvedMedia{}, tmdb.ErrNotFound
	}
}
