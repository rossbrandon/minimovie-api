package tmdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

type Collection struct {
	ID         int              `json:"id"`
	Name       string           `json:"name"`
	Overview   string           `json:"overview"`
	PosterPath string           `json:"poster_path"`
	Parts      []CollectionPart `json:"parts"`
}

type CollectionPart struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	Overview    string  `json:"overview"`
	PosterPath  string  `json:"poster_path"`
	ReleaseDate string  `json:"release_date"`
	VoteAverage float64 `json:"vote_average"`
}

func (c *Client) GetCollection(ctx context.Context, id int) (*Collection, error) {
	log.Info().Int("id", id).Msg("Getting collection from TMDB")
	path := fmt.Sprintf("/collection/%d", id)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var collection Collection
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &collection, nil
}
