package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/tmdb"
	"github.com/rs/zerolog/log"
)

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

	httputil.JSON(w, http.StatusOK, toMovieDetails(movie))
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
		WatchProviders:      buildWatchProviders(movie.WatchProviders, "US"),
		Credits:             buildCredits(movie.Credits),
	}
}

func buildImageURL(path string, size string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf("https://image.tmdb.org/t/p/%s%s", size, path)
}

func buildWatchProviders(wp tmdb.WatchProviders, country string) *WatchProviders {
	countryProviders, ok := wp.Results[country]
	if !ok {
		return nil
	}

	return &WatchProviders{
		Stream: toWatchProviders(countryProviders.Flatrate),
		Rent:   toWatchProviders(countryProviders.Rent),
		Buy:    toWatchProviders(countryProviders.Buy),
		Free:   toWatchProviders(countryProviders.Free),
		Ads:    toWatchProviders(countryProviders.Ads),
	}
}

func toWatchProviders(providers []tmdb.Provider) []WatchProvider {
	if len(providers) == 0 {
		return nil
	}
	result := make([]WatchProvider, len(providers))
	for i, p := range providers {
		result[i] = WatchProvider{
			Name:    p.ProviderName,
			LogoURL: buildImageURL(p.LogoPath, "w92"),
		}
	}
	return result
}

func buildCredits(credits tmdb.Credits) *Credits {
	cast := make([]Person, len(credits.Cast))
	for i, c := range credits.Cast {
		cast[i] = Person{
			ID:       c.ID,
			Name:     c.Name,
			PhotoURL: buildImageURL(c.ProfilePath, "w185"),
			Role:     c.Character,
			Order:    c.Order,
		}
	}

	// Extract key crew by job
	var directors, writers, producers, composers []Person
	var cinematographers, editors, productionDesign, costumeDesign, casting []Person

	for _, c := range credits.Crew {
		person := Person{
			ID:       c.ID,
			Name:     c.Name,
			PhotoURL: buildImageURL(c.ProfilePath, "w185"),
			Role:     c.Job,
		}

		switch c.Job {
		case "Director":
			directors = append(directors, person)
		case "Screenplay", "Writer", "Story":
			writers = append(writers, person)
		case "Producer", "Executive Producer":
			producers = append(producers, person)
		case "Original Music Composer":
			composers = append(composers, person)
		case "Director of Photography":
			cinematographers = append(cinematographers, person)
		case "Editor":
			editors = append(editors, person)
		case "Production Design", "Set Designer":
			productionDesign = append(productionDesign, person)
		case "Costume Design":
			costumeDesign = append(costumeDesign, person)
		case "Casting":
			casting = append(casting, person)
		}
	}

	return &Credits{
		Cast:             cast,
		Directors:        directors,
		Writers:          writers,
		Producers:        producers,
		Composers:        composers,
		Cinematographers: cinematographers,
		Editors:          editors,
		ProductionDesign: productionDesign,
		CostumeDesign:    costumeDesign,
		Casting:          casting,
	}
}
