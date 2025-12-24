package tmdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

type Series struct {
	ID                  int                 `json:"id"`
	Name                string              `json:"name"`
	Tagline             string              `json:"tagline"`
	Overview            string              `json:"overview"`
	Genres              []Genre             `json:"genres"`
	PosterPath          string              `json:"poster_path"`
	Status              string              `json:"status"`
	InProduction        bool                `json:"in_production"`
	FirstAirDate        string              `json:"first_air_date"`
	LastAirDate         string              `json:"last_air_date"`
	NumberOfSeasons     int                 `json:"number_of_seasons"`
	NumberOfEpisodes    int                 `json:"number_of_episodes"`
	EpisodeRunTime      []int               `json:"episode_run_time"`
	VoteAverage         float64             `json:"vote_average"`
	OriginalName        string              `json:"original_name"`
	OriginalLanguage    string              `json:"original_language"`
	OriginCountry       []string            `json:"origin_country"`
	SpokenLanguages     []SpokenLanguage    `json:"spoken_languages"`
	ProductionCompanies []ProductionCompany `json:"production_companies"`
	ProductionCountries []ProductionCountry `json:"production_countries"`
	CreatedBy           []Creator           `json:"created_by"`
	Networks            []Network           `json:"networks"`
	Seasons             []Season            `json:"seasons"`
	WatchProviders      WatchProviders      `json:"watch/providers"`
	AggregateCredits    AggregateCredits    `json:"aggregate_credits"`
}

type Creator struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
}

type Network struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	LogoPath      string `json:"logo_path"`
	OriginCountry string `json:"origin_country"`
}

type Season struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	SeasonNumber int     `json:"season_number"`
	EpisodeCount int     `json:"episode_count"`
	AirDate      string  `json:"air_date"`
	PosterPath   string  `json:"poster_path"`
	VoteAverage  float64 `json:"vote_average"`
}

func (c *Client) GetSeries(ctx context.Context, id int) (*Series, error) {
	log.Info().Int("id", id).Msg("Getting series from TMDB")
	extras := "watch/providers,aggregate_credits"
	path := fmt.Sprintf("/tv/%d?append_to_response=%s", id, extras)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var series Series
	if err := json.Unmarshal(body, &series); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &series, nil
}
