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

type SeasonDetails struct {
	ID           int              `json:"id"`
	Name         string           `json:"name"`
	Overview     string           `json:"overview"`
	PosterPath   string           `json:"posterPath"`
	SeasonNumber int              `json:"seasonNumber"`
	AirDate      string           `json:"airDate"`
	VoteAverage  float64          `json:"voteAverage"`
	Episodes     []EpisodeSummary `json:"episodes"`
	WhereToWatch *WhereToWatch    `json:"whereToWatch,omitempty"`
	Credits      *Credits         `json:"credits,omitempty"`
}

type EpisodeSummary struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	EpisodeNumber int     `json:"episodeNumber"`
	SeasonNumber  int     `json:"seasonNumber"`
	AirDate       string  `json:"airDate"`
	Runtime       int     `json:"runtime"`
	StillPath     string  `json:"stillPath"`
	VoteAverage   float64 `json:"voteAverage"`
}

func (h *Handlers) GetSeason(w http.ResponseWriter, r *http.Request) {
	seriesIDStr := chi.URLParam(r, "seriesId")
	seriesID, err := strconv.Atoi(seriesIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid series ID")
		return
	}

	seasonNumStr := chi.URLParam(r, "seasonNumber")
	seasonNum, err := strconv.Atoi(seasonNumStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid season number")
		return
	}

	season, err := h.tmdbClient.GetSeason(r.Context(), seriesID, seasonNum)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Season not found")
			return
		}
		log.Error().Err(err).Int("series_id", seriesID).Int("season", seasonNum).Msg("failed to fetch season")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch season")
		return
	}

	details := toSeasonDetails(season)
	h.enrichCreditsWithAges(r.Context(), details.Credits, season.AirDate, season.AirDate)

	httputil.JSON(w, http.StatusOK, details)
}

func toSeasonDetails(season *tmdb.SeasonDetails) *SeasonDetails {
	episodes := make([]EpisodeSummary, len(season.Episodes))
	for i, e := range season.Episodes {
		episodes[i] = EpisodeSummary{
			ID:            e.ID,
			Name:          e.Name,
			Overview:      e.Overview,
			EpisodeNumber: e.EpisodeNumber,
			SeasonNumber:  e.SeasonNumber,
			AirDate:       e.AirDate,
			Runtime:       e.Runtime,
			StillPath:     e.StillPath,
			VoteAverage:   e.VoteAverage,
		}
	}

	return &SeasonDetails{
		ID:           season.ID,
		Name:         season.Name,
		Overview:     season.Overview,
		PosterPath:   season.PosterPath,
		SeasonNumber: season.SeasonNumber,
		AirDate:      season.AirDate,
		VoteAverage:  season.VoteAverage,
		Episodes:     episodes,
		WhereToWatch: buildWhereToWatch(season.WatchProviders, "US"),
		Credits:      buildAggregateCredits(season.AggregateCredits),
	}
}
