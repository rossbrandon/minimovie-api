package tmdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
)

type Movie struct {
	ID                  int                 `json:"id"`
	ImdbID              string              `json:"imdb_id"`
	Title               string              `json:"title"`
	Tagline             string              `json:"tagline"`
	Overview            string              `json:"overview"`
	Genres              []Genre             `json:"genres"`
	PosterPath          string              `json:"poster_path"`
	Status              string              `json:"status"`
	ReleaseDate         string              `json:"release_date"`
	Runtime             int                 `json:"runtime"`
	Budget              int                 `json:"budget"`
	Revenue             int                 `json:"revenue"`
	OriginalTitle       string              `json:"original_title"`
	OriginalLanguage    string              `json:"original_language"`
	OriginCountry       []string            `json:"origin_country"`
	SpokenLanguages     []SpokenLanguage    `json:"spoken_languages"`
	ProductionCompanies []ProductionCompany `json:"production_companies"`
	ProductionCountries []ProductionCountry `json:"production_countries"`
	WatchProviders      WatchProviders      `json:"watch/providers"`
	Credits             Credits             `json:"credits"`
}

func (c *Client) GetMovie(ctx context.Context, id int) (*Movie, error) {
	log.Info().Int("id", id).Msg("Getting movie from TMDB")
	extras := "watch/providers,credits"
	path := fmt.Sprintf("/movie/%d?append_to_response=%s", id, extras)

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var movie Movie
	if err := json.Unmarshal(body, &movie); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &movie, nil
}
