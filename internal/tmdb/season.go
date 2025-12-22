package tmdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

type SeasonDetails struct {
	ID               int              `json:"id"`
	Name             string           `json:"name"`
	Overview         string           `json:"overview"`
	PosterPath       string           `json:"poster_path"`
	SeasonNumber     int              `json:"season_number"`
	AirDate          string           `json:"air_date"`
	VoteAverage      float64          `json:"vote_average"`
	Episodes         []Episode        `json:"episodes"`
	WatchProviders   WatchProviders   `json:"watch/providers"`
	AggregateCredits AggregateCredits `json:"aggregate_credits"`
}

type Episode struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	EpisodeNumber int     `json:"episode_number"`
	SeasonNumber  int     `json:"season_number"`
	AirDate       string  `json:"air_date"`
	Runtime       int     `json:"runtime"`
	StillPath     string  `json:"still_path"`
	VoteAverage   float64 `json:"vote_average"`
}

func (c *Client) GetSeason(ctx context.Context, seriesID, seasonNumber int) (*SeasonDetails, error) {
	log.Info().Int("series_id", seriesID).Int("season", seasonNumber).Msg("Getting season from TMDB")
	extras := "watch/providers,aggregate_credits"
	path := fmt.Sprintf("/tv/%d/season/%d?append_to_response=%s", seriesID, seasonNumber, extras)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var season SeasonDetails
	if err := json.Unmarshal(body, &season); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &season, nil
}
