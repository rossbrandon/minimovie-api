package tmdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

type PersonChangesResponse struct {
	Results    []PersonChange `json:"results"`
	Page       int            `json:"page"`
	TotalPages int            `json:"total_pages"`
}

type PersonChange struct {
	ID    int  `json:"id"`
	Adult bool `json:"adult"`
}

func (c *Client) GetPersonChanges(ctx context.Context, startDate, endDate string) ([]int, error) {
	log.Info().Str("start_date", startDate).Str("end_date", endDate).Msg("Fetching person changes from TMDB")

	var allIDs []int
	page := 1

	for {
		path := fmt.Sprintf("/person/changes?start_date=%s&end_date=%s&page=%d", startDate, endDate, page)

		body, err := c.get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch person changes page %d: %w", page, err)
		}

		var response PersonChangesResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse person changes response: %w", err)
		}

		for _, change := range response.Results {
			allIDs = append(allIDs, change.ID)
		}

		log.Info().Int("page", page).Int("total_pages", response.TotalPages).Int("results_count", len(response.Results)).Msg("Fetched person changes page")

		if page >= response.TotalPages {
			break
		}
		page++
	}

	log.Info().Int("total_changed_people", len(allIDs)).Msg("Completed fetching person changes from TMDB")
	log.Debug().Ints("changed_ids", allIDs).Msg("Changed IDs")
	return allIDs, nil
}
