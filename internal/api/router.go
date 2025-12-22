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

	r.Get("/movies/{id}", h.GetMovie)
	r.Get("/series/{id}", h.GetSeries)

	return r
}
