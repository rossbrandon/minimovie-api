package handlers

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/rossbrandon/minimovie-api/internal/catalog"
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
	seriesID, ok := catalog.ParseSlugID(chi.URLParam(r, "seriesId"))
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "Invalid series ID")
		return
	}
	personID, ok := catalog.ParseSlugID(chi.URLParam(r, "personId"))
	if !ok {
		httputil.Error(w, http.StatusBadRequest, "Invalid person ID")
		return
	}

	sr, err := h.catalog.Series(r.Context(), seriesID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			httputil.Error(w, http.StatusNotFound, "Series not found")
			return
		}
		log.Error().Err(err).Int("series_id", seriesID).Int("person_id", personID).Msg("failed to fetch series")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch series data")
		return
	}

	member := seriesCastMember(sr, personID)
	if member == nil {
		httputil.Error(w, http.StatusNotFound, "Person not found in series credits")
		return
	}
	roles := make([]RoleSummary, len(member.Roles))
	for i, r := range member.Roles {
		roles[i] = RoleSummary{Character: r.Character}
	}

	seasons, err := h.personSeasons(r.Context(), sr, member.ID)
	if err != nil {
		log.Error().Err(err).Int("series_id", seriesID).Msg("failed to fetch seasons")
		httputil.Error(w, http.StatusInternalServerError, "Failed to fetch series data")
		return
	}

	httputil.JSON(w, http.StatusOK, PersonSeriesCredits{
		Person: PersonSummary{
			ID:        personID,
			Name:      member.Name,
			PhotoPath: member.ProfilePath,
		},
		Series: SeriesSummary{
			ID:         sr.ID,
			Name:       sr.Name,
			PosterPath: sr.PosterPath,
		},
		TotalEpisodeCount: member.TotalEpisodeCount,
		Roles:             roles,
		Seasons:           seasons,
	})
}

func (h *Handlers) personSeasons(
	ctx context.Context,
	sr *catalog.Series,
	personSourceID int,
) ([]PersonSeasonDetail, error) {
	numbers := make([]int, 0, sr.NumberOfSeasons)
	for n := 1; n <= sr.NumberOfSeasons; n++ {
		numbers = append(numbers, n)
	}
	docs, err := h.catalog.Seasons(ctx, sr.ID, numbers)
	if err != nil {
		return nil, err
	}
	var seasons []PersonSeasonDetail
	for _, n := range numbers {
		if season, ok := personSeason(docs[n], personSourceID); ok {
			seasons = append(seasons, season)
		}
	}
	return seasons, nil
}

func seriesCastMember(sr *catalog.Series, personID int) *tmdb.AggregateCastMember {
	for i, c := range sr.AggregateCredits.Cast {
		if sr.People[c.ID].ID == personID {
			return &sr.AggregateCredits.Cast[i]
		}
	}
	return nil
}

func personSeason(sd *tmdb.SeasonDetails, personSourceID int) (PersonSeasonDetail, bool) {
	if sd == nil {
		return PersonSeasonDetail{}, false
	}
	var episodeCount int
	for _, c := range sd.AggregateCredits.Cast {
		if c.ID == personSourceID {
			episodeCount = c.TotalEpisodeCount
			break
		}
	}
	inAllEpisodes := episodeCount >= len(sd.Episodes)

	var episodes []PersonEpisodeDetail
	for _, ep := range sd.Episodes {
		guest := slices.ContainsFunc(ep.GuestStars, func(g tmdb.CastMember) bool { return g.ID == personSourceID })
		if !inAllEpisodes && !guest {
			continue
		}
		episodes = append(episodes, PersonEpisodeDetail{
			EpisodeNumber: ep.EpisodeNumber,
			Name:          ep.Name,
			AirDate:       ep.AirDate,
			StillPath:     ep.StillPath,
		})
	}
	if len(episodes) == 0 {
		return PersonSeasonDetail{}, false
	}
	return PersonSeasonDetail{
		SeasonNumber:  sd.SeasonNumber,
		Name:          sd.Name,
		AirDate:       sd.AirDate,
		TotalEpisodes: len(sd.Episodes),
		Episodes:      episodes,
	}, true
}
