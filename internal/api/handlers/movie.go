package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

type MovieDetails struct {
	ID                  int             `json:"id"`
	Slug                string          `json:"slug,omitempty"`
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
	id, ok := catalog.ParseSlugID(chi.URLParam(r, "id"))
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "Invalid movie ID")
		return
	}

	m, err := h.catalog.Movie(r.Context(), id)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Movie not found")
			return
		}
		log.Error().Err(err).Int("movie_id", id).Msg("failed to fetch movie")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch movie")
		return
	}

	details := toMovieDetails(m.Movie)
	details.ID, details.Slug = m.ID, m.Slug
	applyPeople(details.Credits, m.People, m.ReleaseDate, m.ReleaseDate)
	details.CollectionInfo = h.collectionInfo(r.Context(), m.CollectionID)

	httputil.JSON(w, http.StatusOK, details)
}

func (h *Handlers) collectionInfo(ctx context.Context, id *int) *CollectionInfo {
	if id == nil {
		return nil
	}
	c, err := h.catalog.Collection(ctx, *id)
	if err != nil {
		if !errors.Is(err, catalog.ErrNotFound) {
			log.Warn().Err(err).Int("collection_id", *id).Msg("failed to fetch collection")
		}
		return nil
	}
	return toCollectionInfo(c)
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

func toCollectionInfo(c *catalog.Collection) *CollectionInfo {
	parts := make([]MovieDetails, 0, len(c.Parts))
	for _, p := range c.Parts {
		id, ok := c.PartIDs[p.ID]
		if !ok {
			continue
		}
		parts = append(parts, MovieDetails{
			ID:          id,
			Title:       p.Title,
			Overview:    p.Overview,
			PosterPath:  p.PosterPath,
			ReleaseDate: p.ReleaseDate,
			VoteAverage: p.VoteAverage,
		})
	}

	return &CollectionInfo{
		ID:         c.ID,
		Name:       c.Name,
		Overview:   c.Overview,
		PosterPath: c.PosterPath,
		Parts:      parts,
	}
}
