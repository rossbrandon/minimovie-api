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

type SeriesDetails struct {
	ID                  int           `json:"id"`
	Name                string        `json:"name"`
	Tagline             string        `json:"tagline"`
	Overview            string        `json:"overview"`
	Genres              []string      `json:"genres"`
	PosterURL           string        `json:"posterUrl"`
	Status              string        `json:"status"`
	InProduction        bool          `json:"inProduction"`
	FirstAirDate        string        `json:"firstAirDate"`
	LastAirDate         string        `json:"lastAirDate"`
	NumberOfSeasons     int           `json:"numberOfSeasons"`
	NumberOfEpisodes    int           `json:"numberOfEpisodes"`
	EpisodeRunTime      []int         `json:"episodeRunTime"`
	OriginalName        string        `json:"originalName"`
	OriginalLanguage    string        `json:"originalLanguage"`
	OriginCountry       string        `json:"originCountry"`
	SpokenLanguages     []string      `json:"spokenLanguages"`
	ProductionCompanies []string      `json:"productionCompanies"`
	ProductionCountries []string      `json:"productionCountries"`
	CreatedBy           []Person      `json:"createdBy"`
	Networks            []Network     `json:"networks"`
	Seasons             []Season      `json:"seasons"`
	WhereToWatch        *WhereToWatch `json:"whereToWatch,omitempty"`
	Credits             *Credits      `json:"credits,omitempty"`
}

type Network struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	LogoURL       string `json:"logoUrl"`
	OriginCountry string `json:"originCountry"`
}

type Season struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	SeasonNumber int     `json:"seasonNumber"`
	EpisodeCount int     `json:"episodeCount"`
	AirDate      string  `json:"airDate"`
	PosterURL    string  `json:"posterUrl"`
	VoteAverage  float64 `json:"voteAverage"`
}

func (h *Handlers) GetSeries(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid series ID")
		return
	}

	series, err := h.tmdbClient.GetSeries(r.Context(), id)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Series not found")
			return
		}
		log.Error().Err(err).Int("series_id", id).Msg("failed to fetch series")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch series")
		return
	}

	httputil.JSON(w, http.StatusOK, toSeriesDetails(series))
}

func toSeriesDetails(series *tmdb.Series) *SeriesDetails {
	genres := make([]string, len(series.Genres))
	for i, g := range series.Genres {
		genres[i] = g.Name
	}

	var originCountry string
	if len(series.OriginCountry) > 0 {
		originCountry = series.OriginCountry[0]
	}

	spokenLanguages := make([]string, len(series.SpokenLanguages))
	for i, l := range series.SpokenLanguages {
		spokenLanguages[i] = l.EnglishName
	}

	productionCompanies := make([]string, len(series.ProductionCompanies))
	for i, c := range series.ProductionCompanies {
		productionCompanies[i] = c.Name
	}

	productionCountries := make([]string, len(series.ProductionCountries))
	for i, c := range series.ProductionCountries {
		productionCountries[i] = c.Code
	}

	createdBy := make([]Person, len(series.CreatedBy))
	for i, c := range series.CreatedBy {
		createdBy[i] = Person{
			ID:       c.ID,
			Name:     c.Name,
			PhotoURL: buildImageURL(c.ProfilePath, "w92"),
		}
	}

	networks := make([]Network, len(series.Networks))
	for i, n := range series.Networks {
		networks[i] = Network{
			ID:            n.ID,
			Name:          n.Name,
			LogoURL:       buildImageURL(n.LogoPath, "w92"),
			OriginCountry: n.OriginCountry,
		}
	}

	seasons := make([]Season, 0, len(series.Seasons))
	for _, s := range series.Seasons {
		// Skip "Specials" season (season 0)
		if s.SeasonNumber == 0 {
			continue
		}
		seasons = append(seasons, Season{
			ID:           s.ID,
			Name:         s.Name,
			Overview:     s.Overview,
			SeasonNumber: s.SeasonNumber,
			EpisodeCount: s.EpisodeCount,
			AirDate:      s.AirDate,
			PosterURL:    buildImageURL(s.PosterPath, "w92"),
			VoteAverage:  s.VoteAverage,
		})
	}

	return &SeriesDetails{
		ID:                  series.ID,
		Name:                series.Name,
		Tagline:             series.Tagline,
		Overview:            series.Overview,
		Genres:              genres,
		PosterURL:           buildImageURL(series.PosterPath, "w300"),
		Status:              series.Status,
		InProduction:        series.InProduction,
		FirstAirDate:        series.FirstAirDate,
		LastAirDate:         series.LastAirDate,
		NumberOfSeasons:     series.NumberOfSeasons,
		NumberOfEpisodes:    series.NumberOfEpisodes,
		EpisodeRunTime:      series.EpisodeRunTime,
		OriginalName:        series.OriginalName,
		OriginalLanguage:    series.OriginalLanguage,
		OriginCountry:       originCountry,
		SpokenLanguages:     spokenLanguages,
		ProductionCompanies: productionCompanies,
		ProductionCountries: productionCountries,
		CreatedBy:           createdBy,
		Networks:            networks,
		Seasons:             seasons,
		WhereToWatch:        buildWhereToWatch(series.WatchProviders, "US"),
		Credits:             buildAggregateCredits(series.AggregateCredits),
	}
}
