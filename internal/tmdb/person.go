package tmdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

type Person struct {
	ID                 int             `json:"id"`
	ImdbID             string          `json:"imdb_id"`
	Name               string          `json:"name"`
	Biography          string          `json:"biography"`
	Birthday           string          `json:"birthday"`
	Deathday           *string         `json:"deathday"`
	Gender             int             `json:"gender"`
	PlaceOfBirth       string          `json:"place_of_birth"`
	ProfilePath        string          `json:"profile_path"`
	KnownForDepartment string          `json:"known_for_department"`
	AlsoKnownAs        []string        `json:"also_known_as"`
	CombinedCredits    CombinedCredits `json:"combined_credits"`
}

func (c *Client) GetPerson(ctx context.Context, id int) (*Person, error) {
	log.Info().Int("id", id).Msg("Getting person from TMDB")
	extras := "combined_credits"
	path := fmt.Sprintf("/person/%d?append_to_response=%s", id, extras)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var person Person
	if err := json.Unmarshal(body, &person); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &person, nil
}
