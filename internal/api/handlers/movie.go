package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

type MovieDetails struct {
	ID                  int           `json:"id"`
	ImdbID              string        `json:"imdbID"`
	Title               string        `json:"title"`
	Tagline             string        `json:"tagline"`
	Overview            string        `json:"overview"`
	Genres              []string      `json:"genres"`
	PosterURL           string        `json:"posterUrl"`
	Status              string        `json:"status"`
	ReleaseDate         string        `json:"releaseDate"`
	Runtime             int           `json:"runtime"`
	Budget              int           `json:"budget"`
	Revenue             int           `json:"revenue"`
	OriginalTitle       string        `json:"originalTitle"`
	OriginalLanguage    string        `json:"originalLanguage"`
	OriginCountry       string        `json:"originCountry"`
	SpokenLanguages     []string      `json:"spokenLanguages"`
	ProductionCompanies []string      `json:"productionCompanies"`
	ProductionCountries []string      `json:"productionCountries"`
	WhereToWatch        *WhereToWatch `json:"whereToWatch,omitempty"`
	Credits             *Credits      `json:"credits,omitempty"`
}

func (h *Handlers) GetMovie(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid movie ID")
		return
	}

	movie, err := h.tmdbClient.GetMovie(r.Context(), id)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Movie not found")
			return
		}
		log.Error().Err(err).Int("movie_id", id).Msg("failed to fetch movie")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch movie")
		return
	}

	details := toMovieDetails(movie)
	h.enrichCreditsWithAges(r.Context(), details.Credits, movie.ReleaseDate, movie.ReleaseDate)

	httputil.JSON(w, http.StatusOK, details)
}

func toMovieDetails(movie *tmdb.Movie) *MovieDetails {
	genres := make([]string, len(movie.Genres))
	for i, g := range movie.Genres {
		genres[i] = g.Name
	}

	var originCountry string
	if len(movie.OriginCountry) > 0 {
		originCountry = movie.OriginCountry[0]
	}

	spokenLanguages := make([]string, len(movie.SpokenLanguages))
	for i, l := range movie.SpokenLanguages {
		spokenLanguages[i] = l.EnglishName
	}

	productionCompanies := make([]string, len(movie.ProductionCompanies))
	for i, c := range movie.ProductionCompanies {
		productionCompanies[i] = c.Name
	}

	productionCountries := make([]string, len(movie.ProductionCountries))
	for i, c := range movie.ProductionCountries {
		productionCountries[i] = c.Code
	}

	return &MovieDetails{
		ID:                  movie.ID,
		ImdbID:              movie.ImdbID,
		Title:               movie.Title,
		Tagline:             movie.Tagline,
		Overview:            movie.Overview,
		Genres:              genres,
		PosterURL:           buildImageURL(movie.PosterPath, "w300"),
		Status:              movie.Status,
		ReleaseDate:         movie.ReleaseDate,
		Runtime:             movie.Runtime,
		Budget:              movie.Budget,
		Revenue:             movie.Revenue,
		OriginalTitle:       movie.OriginalTitle,
		OriginalLanguage:    movie.OriginalLanguage,
		OriginCountry:       originCountry,
		SpokenLanguages:     spokenLanguages,
		ProductionCompanies: productionCompanies,
		ProductionCountries: productionCountries,
		WhereToWatch:        buildWhereToWatch(movie.WatchProviders, "US"),
		Credits:             buildCredits(movie.Credits),
	}
}
