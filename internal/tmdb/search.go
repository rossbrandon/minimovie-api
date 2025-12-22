package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/rs/zerolog/log"
)

type MediaType string

const (
	MediaTypeMovie  MediaType = "movie"
	MediaTypeTV     MediaType = "tv"
	MediaTypePerson MediaType = "person"
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
	ID          int       `json:"id"`
	MediaType   MediaType `json:"media_type"`
	Overview    string    `json:"overview"`
	PosterPath  string    `json:"poster_path"`
	ProfilePath string    `json:"profile_path"`
	SearchResultMovie
	SearchResultShow
	SearchResultPerson
}

func (c *Client) SearchMulti(ctx context.Context, query string, page int) (*SearchResults, error) {
	log.Info().Str("query", query).Int("page", page).Msg("Searching TMDB multi")

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

func (c *Client) SearchMovies(ctx context.Context, query string, page int) (*SearchResults, error) {
	log.Info().Str("query", query).Int("page", page).Msg("Searching TMDB movies")

	if page < 1 {
		page = 1
	}

	path := fmt.Sprintf("/search/movie?query=%s&page=%d", url.QueryEscape(query), page)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var results SearchResults
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	for i := range results.Results {
		results.Results[i].MediaType = MediaTypeMovie
	}

	return &results, nil
}

func (c *Client) SearchSeries(ctx context.Context, query string, page int) (*SearchResults, error) {
	log.Info().Str("query", query).Int("page", page).Msg("Searching TMDB series")

	if page < 1 {
		page = 1
	}

	path := fmt.Sprintf("/search/tv?query=%s&page=%d", url.QueryEscape(query), page)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var results SearchResults
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	for i := range results.Results {
		results.Results[i].MediaType = MediaTypeTV
	}

	return &results, nil
}

func (c *Client) SearchPerson(ctx context.Context, query string, page int) (*SearchResults, error) {
	log.Info().Str("query", query).Int("page", page).Msg("Searching TMDB person")

	if page < 1 {
		page = 1
	}

	path := fmt.Sprintf("/search/person?query=%s&page=%d", url.QueryEscape(query), page)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var results SearchResults
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	for i := range results.Results {
		results.Results[i].MediaType = MediaTypePerson
	}

	return &results, nil
}
