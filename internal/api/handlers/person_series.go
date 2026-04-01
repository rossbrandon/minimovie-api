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

type PersonSummary struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	PhotoPath string `json:"photoPath,omitempty"`
}

type SeriesSummary struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	PosterPath string `json:"posterPath,omitempty"`
}

type RoleSummary struct {
	Character string `json:"character"`
}

type PersonEpisodeDetail struct {
	EpisodeNumber int    `json:"episodeNumber"`
	Name          string `json:"name"`
	AirDate       string `json:"airDate,omitempty"`
	StillPath     string `json:"stillPath,omitempty"`
}

type PersonSeasonDetail struct {
	SeasonNumber  int                   `json:"seasonNumber"`
	Name          string                `json:"name"`
	AirDate       string                `json:"airDate,omitempty"`
	TotalEpisodes int                   `json:"totalEpisodes"`
	Episodes      []PersonEpisodeDetail `json:"episodes"`
}

type PersonSeriesCredits struct {
	Person            PersonSummary        `json:"person"`
	Series            SeriesSummary        `json:"series"`
	TotalEpisodeCount int                  `json:"totalEpisodeCount"`
	Roles             []RoleSummary        `json:"roles"`
	Seasons           []PersonSeasonDetail `json:"seasons"`
}

func (h *Handlers) GetPersonSeriesCredits(w http.ResponseWriter, r *http.Request) {
	seriesIDStr := chi.URLParam(r, "seriesId")
	seriesID, err := strconv.Atoi(seriesIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid series ID")
		return
	}

	personIDStr := chi.URLParam(r, "personId")
	personID, err := strconv.Atoi(personIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "Invalid person ID")
		return
	}

	data, err := h.tmdbClient.GetSeriesWithSeasons(r.Context(), seriesID)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Series not found")
			return
		}
		log.Error().Err(err).Int("series_id", seriesID).Int("person_id", personID).Msg("failed to fetch series with seasons")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch series data")
		return
	}

	var member *tmdb.AggregateCastMember
	for i, c := range data.AggregateCredits.Cast {
		if c.ID == personID {
			member = &data.AggregateCredits.Cast[i]
			break
		}
	}
	if member == nil {
		httputil.Error(w, http.StatusNotFound, "Person not found in series credits")
		return
	}

	isRegular := false
	for _, c := range data.RegularCredits.Cast {
		if c.ID == personID {
			isRegular = true
			break
		}
	}

	regularInAll := isRegular && member.TotalEpisodeCount >= data.NumberOfEpisodes

	roles := make([]RoleSummary, len(member.Roles))
	for i, r := range member.Roles {
		roles[i] = RoleSummary{Character: r.Character}
	}

	var seasons []PersonSeasonDetail
	for i := 1; i <= data.NumberOfSeasons; i++ {
		sd, ok := data.SeasonDetails[i]
		if !ok {
			continue
		}

		var episodes []PersonEpisodeDetail
		for _, ep := range sd.Episodes {
			if regularInAll {
				episodes = append(episodes, PersonEpisodeDetail{
					EpisodeNumber: ep.EpisodeNumber,
					Name:          ep.Name,
					AirDate:       ep.AirDate,
					StillPath:     ep.StillPath,
				})
				continue
			}

			appeared := false
			if isRegular {
				appeared = true
			}
			for _, gs := range ep.GuestStars {
				if gs.ID == personID {
					appeared = true
					break
				}
			}

			if appeared {
				episodes = append(episodes, PersonEpisodeDetail{
					EpisodeNumber: ep.EpisodeNumber,
					Name:          ep.Name,
					AirDate:       ep.AirDate,
					StillPath:     ep.StillPath,
				})
			}
		}

		if len(episodes) == 0 {
			continue
		}

		seasons = append(seasons, PersonSeasonDetail{
			SeasonNumber:  sd.SeasonNumber,
			Name:          sd.Name,
			AirDate:       sd.AirDate,
			TotalEpisodes: len(sd.Episodes),
			Episodes:      episodes,
		})
	}

	response := PersonSeriesCredits{
		Person: PersonSummary{
			ID:        member.ID,
			Name:      member.Name,
			PhotoPath: member.ProfilePath,
		},
		Series: SeriesSummary{
			ID:         data.ID,
			Name:       data.Name,
			PosterPath: data.PosterPath,
		},
		TotalEpisodeCount: member.TotalEpisodeCount,
		Roles:             roles,
		Seasons:           seasons,
	}

	httputil.JSON(w, http.StatusOK, response)
}
