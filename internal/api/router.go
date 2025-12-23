package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/jwtauth/v5"
	"github.com/rossbrandon/minimovie-api/config"
	"github.com/rossbrandon/minimovie-api/internal/api/handlers"
	"github.com/rossbrandon/minimovie-api/internal/metrics"
)

var tokenAuth *jwtauth.JWTAuth

func NewRouter(h *handlers.Handlers, config *config.Config) *chi.Mux {
	tokenAuth = jwtauth.New("HS256", []byte(config.MiniMovieUiSecret), nil)

	r := chi.NewRouter()
	r.Use(metrics.Middleware)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(time.Duration(config.Timeout) * time.Second))
	r.Use(jwtauth.Verifier(tokenAuth))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://minimovie.info", "https://localhost:4321", "http://localhost:4321"},
		AllowedMethods:   []string{"GET", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Authenticated routes
	r.Group(func(r chi.Router) {
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

		// People
		r.Get("/people/{id}", h.GetPerson)
	})

	// Unauthenticated routes
	r.Group(func(r chi.Router) {
		r.Get("/ping", Ping)
	})

	return r
}

func Ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}
