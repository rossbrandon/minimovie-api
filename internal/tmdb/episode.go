package tmdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

type EpisodeDetails struct {
	ID            int          `json:"id"`
	Name          string       `json:"name"`
	Overview      string       `json:"overview"`
	EpisodeNumber int          `json:"episode_number"`
	SeasonNumber  int          `json:"season_number"`
	AirDate       string       `json:"air_date"`
	Runtime       int          `json:"runtime"`
	StillPath     string       `json:"still_path"`
	VoteAverage   float64      `json:"vote_average"`
	VoteCount     int          `json:"vote_count"`
	Crew          []CrewMember `json:"crew"`
	GuestStars    []CastMember `json:"guest_stars"`
}

func (c *Client) GetEpisode(ctx context.Context, seriesID, seasonNumber, episodeNumber int) (*EpisodeDetails, error) {
	log.Info().Int("series_id", seriesID).Int("season", seasonNumber).Int("episode", episodeNumber).Msg("Getting episode from TMDB")
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", seriesID, seasonNumber, episodeNumber)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var episode EpisodeDetails
	if err := json.Unmarshal(body, &episode); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &episode, nil
}
