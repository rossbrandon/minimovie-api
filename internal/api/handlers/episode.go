package handlers

import (
	"errors"
	"net/http"
	"sort"
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
	StillPath     string   `json:"stillPath"`
	VoteAverage   float64  `json:"voteAverage"`
	VoteCount     int      `json:"voteCount"`
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

	details := toEpisodeDetails(episode)
	h.enrichCreditsWithAges(r.Context(), details.Credits, episode.AirDate, episode.AirDate)

	httputil.JSON(w, http.StatusOK, details)
}

func toEpisodeDetails(episode *tmdb.EpisodeDetails) *EpisodeDetails {
	mergedCast := mergeEpisodeCast(episode.Credits.Cast, episode.Credits.GuestStars)

	return &EpisodeDetails{
		ID:            episode.ID,
		Name:          episode.Name,
		Overview:      episode.Overview,
		EpisodeNumber: episode.EpisodeNumber,
		SeasonNumber:  episode.SeasonNumber,
		AirDate:       episode.AirDate,
		Runtime:       episode.Runtime,
		StillPath:     episode.StillPath,
		VoteAverage:   episode.VoteAverage,
		VoteCount:     episode.VoteCount,
		Credits:       buildCredits(tmdb.Credits{Cast: mergedCast, Crew: episode.Credits.Crew}),
	}
}

func mergeEpisodeCast(regulars, guests []tmdb.CastMember) []tmdb.CastMember {
	seen := make(map[int]int) // person ID -> index in merged
	merged := make([]tmdb.CastMember, 0, len(regulars)+len(guests))

	for _, c := range regulars {
		seen[c.ID] = len(merged)
		merged = append(merged, c)
	}
	for _, g := range guests {
		if idx, ok := seen[g.ID]; ok {
			if g.Order < merged[idx].Order {
				merged[idx] = g
			}
			continue
		}
		merged = append(merged, g)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Order < merged[j].Order
	})
	return merged
}
