package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/jwtauth/v5"
	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/api/handlers"
	mw "github.com/rossbrandon/minimovie-api/internal/api/middleware"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
	"github.com/rossbrandon/minimovie-api/internal/store"
)

var tokenAuth *jwtauth.JWTAuth

func NewRouter(h *handlers.Handlers, cfg *config.Config, sessionStore store.SessionRepository) *chi.Mux {
	tokenAuth = jwtauth.New("HS256", []byte(cfg.MiniMovieUiSecret), nil)

	r := chi.NewRouter()
	r.Use(metrics.Middleware)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://minimovie.info", "https://localhost:4321", "http://localhost:4321"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	cookieName := "__Host-mm_session"
	if !cfg.IsProduction {
		cookieName = "mm_session_dev"
	}

	// Public catalog (guest key authenticated)
	r.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(time.Duration(cfg.Timeout) * time.Second))
		r.Use(jwtauth.Verifier(tokenAuth))
		r.Use(jwtauth.Authenticator(tokenAuth))

		// Search
		r.Get("/search", h.Search)

		// Movies
		r.Get("/movies/{id}", h.GetMovie)

		// TV Series
		r.Get("/series/{id}", h.GetSeries)
		r.Get("/series/{seriesId}/seasons/{seasonNumber}", h.GetSeason)
		r.Get("/series/{seriesId}/seasons/{seasonNumber}/episodes/{episodeNumber}", h.GetEpisode)
		r.Get("/series/{seriesId}/person/{personId}/credits", h.GetPersonSeriesCredits)

		// People
		r.Get("/people/{id}", h.GetPerson)
	})

	// Authenticated routes with longer timeout for LLM-backed enrichment
	r.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(time.Duration(cfg.AugurTimeout) * time.Second))
		r.Use(jwtauth.Verifier(tokenAuth))
		r.Use(jwtauth.Authenticator(tokenAuth))

		// Interesting Info
		r.Get("/interesting/person/{id}", h.GetPersonInterestingInfo)
	})

	// OAuth webhooks and callbacks
	r.Route("/auth", func(r chi.Router) {
		r.Get("/{provider}", h.BeginAuth)
		r.Get("/{provider}/callback", h.AuthCallback)
		r.Post("/{provider}/callback", h.AuthCallback)
		r.Post("/apple/notifications", h.AppleNotifications)

		// Token exchanges uses the UI JWT as well
		r.Group(func(r chi.Router) {
			r.Use(jwtauth.Verifier(tokenAuth))
			r.Use(jwtauth.Authenticator(tokenAuth))
			r.Post("/token", h.ExchangeToken)
		})
	})

	// User routes (session cookie only)
	r.Group(func(r chi.Router) {
		r.Use(mw.RequireSession(sessionStore, cfg.SessionSecret, cookieName))
		r.Use(mw.NoStoreCache)

		r.Get("/auth/session", h.GetSession)
		r.Post("/auth/logout", h.Logout)

		r.Route("/users/me", func(r chi.Router) {
			r.Post("/delete", h.DeleteAccount)
			r.Post("/export", h.ExportUserData)

			r.Get("/watchlist", h.ListWatchlist)
			r.Post("/watchlist", h.AddToWatchlist)
			r.Get("/watchlist/check", h.CheckWatchlist)
			r.Patch("/watchlist/{id}", h.UpdateWatchlistStatus)
			r.Delete("/watchlist/{id}", h.RemoveFromWatchlist)

			r.Get("/progress/{seriesId}", h.GetWatchProgress)
			r.Post("/progress", h.MarkEpisodeWatched)

			r.Get("/watch-events", h.ListWatchEvents)
			r.Post("/watch-events", h.CreateWatchEvent)
			r.Delete("/watch-events/{id}", h.UnmarkEpisode)

			r.Get("/stats", h.GetStats)
			r.Get("/achievements", h.GetAchievements)
			r.Get("/achievements/unseen", h.GetUnseenAchievements)
			r.Patch("/achievements/seen", h.MarkAchievementsSeen)
		})
	})

	// Unauthenticated routes
	r.Group(func(r chi.Router) {
		r.Get("/ping", Ping)
	})

	return r
}

func Ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("pong"))
}
