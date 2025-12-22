package api

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rossbrandon/minimovie-api/internal/api/handlers"
)

func NewRouter(h *handlers.Handlers, timeout int) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(time.Duration(timeout) * time.Second))

	// Search
	r.Get("/search", h.SearchMulti)

	// Movies
	r.Get("/movies/{id}", h.GetMovie)

	// TV Series
	r.Get("/series/{id}", h.GetSeries)
	r.Get("/series/{seriesId}/seasons/{seasonNumber}", h.GetSeason)
	r.Get("/series/{seriesId}/seasons/{seasonNumber}/episodes/{episodeNumber}", h.GetEpisode)

	// People
	r.Get("/people/{id}", h.GetPerson)

	return r
}
