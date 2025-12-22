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

type EpisodeDetails struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Overview      string   `json:"overview"`
	EpisodeNumber int      `json:"episodeNumber"`
	SeasonNumber  int      `json:"seasonNumber"`
	AirDate       string   `json:"airDate"`
	Runtime       int      `json:"runtime"`
	StillURL      string   `json:"stillUrl"`
	VoteAverage   float64  `json:"voteAverage"`
	VoteCount     int      `json:"voteCount"`
	GuestStars    []Person `json:"guestStars,omitempty"`
	Credits       *Credits `json:"credits,omitempty"`
}

func (h *Handlers) GetEpisode(w http.ResponseWriter, r *http.Request) {
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

	episodeNumStr := chi.URLParam(r, "episodeNumber")
	episodeNum, err := strconv.Atoi(episodeNumStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid episode number")
		return
	}

	episode, err := h.tmdbClient.GetEpisode(r.Context(), seriesID, seasonNum, episodeNum)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Episode not found")
			return
		}
		log.Error().Err(err).Int("series_id", seriesID).Int("season", seasonNum).Int("episode", episodeNum).Msg("failed to fetch episode")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch episode")
		return
	}

	httputil.JSON(w, http.StatusOK, toEpisodeDetails(episode))
}

func toEpisodeDetails(episode *tmdb.EpisodeDetails) *EpisodeDetails {
	guestStars := make([]Person, len(episode.GuestStars))
	for i, g := range episode.GuestStars {
		guestStars[i] = Person{
			ID:       g.ID,
			Name:     g.Name,
			PhotoURL: buildImageURL(g.ProfilePath, "w92"),
			Role:     g.Character,
			Order:    g.Order,
		}
	}

	return &EpisodeDetails{
		ID:            episode.ID,
		Name:          episode.Name,
		Overview:      episode.Overview,
		EpisodeNumber: episode.EpisodeNumber,
		SeasonNumber:  episode.SeasonNumber,
		AirDate:       episode.AirDate,
		Runtime:       episode.Runtime,
		StillURL:      buildImageURL(episode.StillPath, "w300"),
		VoteAverage:   episode.VoteAverage,
		VoteCount:     episode.VoteCount,
		GuestStars:    guestStars,
		Credits:       buildCredits(tmdb.Credits{Crew: episode.Crew}),
	}
}
