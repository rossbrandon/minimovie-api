package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/rs/zerolog/log"
)

type SearchResults struct {
	Page         int            `json:"page"`
	TotalPages   int            `json:"total_pages"`
	TotalResults int            `json:"total_results"`
	Results      []SearchResult `json:"results"`
}

type SearchResultMovie struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
}

type SearchResultShow struct {
	Name         string `json:"name"`
	FirstAirDate string `json:"first_air_date"`
}

type SearchResultPerson struct {
	KnownForDepartment string `json:"known_for_department"`
}

type SearchResult struct {
	ID          int    `json:"id"`
	MediaType   string `json:"media_type"`
	Overview    string `json:"overview"`
	PosterPath  string `json:"poster_path"`
	ProfilePath string `json:"profile_path"`
	SearchResultMovie
	SearchResultShow
	SearchResultPerson
}

func (c *Client) SearchMulti(ctx context.Context, query string, page int) (*SearchResults, error) {
	log.Info().Str("query", query).Int("page", page).Msg("Searching TMDB")

	if page < 1 {
		page = 1
	}

	path := fmt.Sprintf("/search/multi?query=%s&page=%d", url.QueryEscape(query), page)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var results SearchResults
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &results, nil
}
