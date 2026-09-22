package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
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
	seriesID, ok := catalog.ParseSlugID(chi.URLParam(r, "seriesId"))
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "Invalid series ID")
		return
	}

	seasonNumStr := chi.URLParam(r, "seasonNumber")
	seasonNum, err := strconv.Atoi(seasonNumStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid season number")
		return
	}

	s, err := h.catalog.Season(r.Context(), seriesID, seasonNum)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Season not found")
			return
		}
		log.Error().Err(err).Int("series_id", seriesID).Int("season_number", seasonNum).Msg("failed to fetch season")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch season")
		return
	}

	details := toSeasonDetails(s.SeasonDetails)
	details.ID = s.ID
	details.Episodes = mapEpisodes(details.Episodes, s.EpisodeIDs)
	applyPeople(details.Credits, s.People, s.AirDate, s.AirDate)

	httputil.JSON(w, http.StatusOK, details)
}

func mapEpisodes(episodes []EpisodeSummary, ids catalog.IDs) []EpisodeSummary {
	kept := episodes[:0]
	for _, ep := range episodes {
		id, ok := ids[ep.EpisodeNumber]
		if !ok {
			continue
		}
		ep.ID = id
		kept = append(kept, ep)
	}
	return kept
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
