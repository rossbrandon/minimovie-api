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
	ID                  int             `json:"id"`
	ImdbID              string          `json:"imdbID"`
	Title               string          `json:"title"`
	Tagline             string          `json:"tagline"`
	Overview            string          `json:"overview"`
	Genres              []string        `json:"genres"`
	PosterPath          string          `json:"posterPath"`
	Status              string          `json:"status"`
	ReleaseDate         string          `json:"releaseDate"`
	Runtime             int             `json:"runtime"`
	Budget              int             `json:"budget"`
	Revenue             int             `json:"revenue"`
	VoteAverage         float64         `json:"voteAverage"`
	OriginalTitle       string          `json:"originalTitle"`
	OriginalLanguage    string          `json:"originalLanguage"`
	OriginCountry       string          `json:"originCountry"`
	SpokenLanguages     []string        `json:"spokenLanguages"`
	ProductionCompanies []string        `json:"productionCompanies"`
	ProductionCountries []string        `json:"productionCountries"`
	WhereToWatch        *WhereToWatch   `json:"whereToWatch,omitempty"`
	Credits             *Credits        `json:"credits,omitempty"`
	CollectionInfo      *CollectionInfo `json:"collectionInfo,omitempty"`
}

type CollectionInfo struct {
	ID         int            `json:"id"`
	Name       string         `json:"name"`
	Overview   string         `json:"overview"`
	PosterPath string         `json:"posterPath"`
	Parts      []MovieDetails `json:"parts"`
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

	if movie.BelongsToCollection != nil {
		collection, err := h.tmdbClient.GetCollection(r.Context(), movie.BelongsToCollection.ID)
		if err != nil {
			log.Warn().Err(err).Int("collection_id", movie.BelongsToCollection.ID).Msg("failed to fetch collection")
		} else {
			details.CollectionInfo = toCollectionInfo(collection)
		}
	}

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
		PosterPath:          movie.PosterPath,
		Status:              movie.Status,
		ReleaseDate:         movie.ReleaseDate,
		Runtime:             movie.Runtime,
		Budget:              movie.Budget,
		Revenue:             movie.Revenue,
		VoteAverage:         movie.VoteAverage,
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

func toCollectionInfo(collection *tmdb.Collection) *CollectionInfo {
	parts := make([]MovieDetails, len(collection.Parts))
	for i, p := range collection.Parts {
		parts[i] = MovieDetails{
			ID:          p.ID,
			Title:       p.Title,
			Overview:    p.Overview,
			PosterPath:  p.PosterPath,
			ReleaseDate: p.ReleaseDate,
			VoteAverage: p.VoteAverage,
		}
	}

	return &CollectionInfo{
		ID:         collection.ID,
		Name:       collection.Name,
		Overview:   collection.Overview,
		PosterPath: collection.PosterPath,
		Parts:      parts,
	}
}
