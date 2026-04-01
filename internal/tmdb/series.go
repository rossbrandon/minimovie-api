package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

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

type SeasonSummaryEpisode struct {
	ID            int          `json:"id"`
	Name          string       `json:"name"`
	EpisodeNumber int          `json:"episode_number"`
	SeasonNumber  int          `json:"season_number"`
	AirDate       string       `json:"air_date"`
	StillPath     string       `json:"still_path"`
	GuestStars    []CastMember `json:"guest_stars"`
}

type SeasonSummary struct {
	SeasonNumber int                    `json:"season_number"`
	Name         string                 `json:"name"`
	AirDate      string                 `json:"air_date"`
	Episodes     []SeasonSummaryEpisode `json:"episodes"`
}

type SeriesWithSeasons struct {
	Series
	RegularCredits Credits
	SeasonDetails  map[int]SeasonSummary
}

const maxAppendItems = 20

func (c *Client) GetSeriesWithSeasons(ctx context.Context, id int) (*SeriesWithSeasons, error) {
	log.Info().Int("id", id).Msg("Getting series with seasons from TMDB")

	maxSeasonsFirstBatch := maxAppendItems - 2 // reserve slots for credits + aggregate_credits
	parts := []string{"credits", "aggregate_credits"}
	for i := 1; i <= maxSeasonsFirstBatch; i++ {
		parts = append(parts, fmt.Sprintf("season/%d", i))
	}
	path := fmt.Sprintf("/tv/%d?append_to_response=%s", id, strings.Join(parts, ","))

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var series Series
	if err := json.Unmarshal(body, &series); err != nil {
		return nil, fmt.Errorf("failed to parse series: %w", err)
	}

	var credits Credits
	if creditsRaw, ok := raw["credits"]; ok {
		if err := json.Unmarshal(creditsRaw, &credits); err != nil {
			return nil, fmt.Errorf("failed to parse credits: %w", err)
		}
	}

	seasonDetails := make(map[int]SeasonSummary)
	for i := 1; i <= series.NumberOfSeasons && i <= maxSeasonsFirstBatch; i++ {
		key := fmt.Sprintf("season/%d", i)
		if seasonRaw, ok := raw[key]; ok {
			var sd SeasonSummary
			if err := json.Unmarshal(seasonRaw, &sd); err != nil {
				log.Warn().Err(err).Int("season", i).Msg("failed to parse appended season")
				continue
			}
			seasonDetails[i] = sd
		}
	}

	if series.NumberOfSeasons > maxSeasonsFirstBatch {
		var mu sync.Mutex
		var wg sync.WaitGroup
		for start := maxSeasonsFirstBatch + 1; start <= series.NumberOfSeasons; start += maxAppendItems {
			end := start + maxAppendItems - 1
			if end > series.NumberOfSeasons {
				end = series.NumberOfSeasons
			}
			wg.Add(1)
			go func(start, end int) {
				defer wg.Done()
				var batchParts []string
				for i := start; i <= end; i++ {
					batchParts = append(batchParts, fmt.Sprintf("season/%d", i))
				}
				batchPath := fmt.Sprintf("/tv/%d?append_to_response=%s", id, strings.Join(batchParts, ","))

				batchBody, err := c.get(ctx, batchPath)
				if err != nil {
					log.Warn().Err(err).Int("start", start).Int("end", end).Msg("failed to fetch additional seasons batch")
					return
				}

				var batchRaw map[string]json.RawMessage
				if err := json.Unmarshal(batchBody, &batchRaw); err != nil {
					return
				}

				mu.Lock()
				defer mu.Unlock()
				for i := start; i <= end; i++ {
					key := fmt.Sprintf("season/%d", i)
					if seasonRaw, ok := batchRaw[key]; ok {
						var sd SeasonSummary
						if err := json.Unmarshal(seasonRaw, &sd); err != nil {
							continue
						}
						seasonDetails[i] = sd
					}
				}
			}(start, end)
		}
		wg.Wait()
	}

	return &SeriesWithSeasons{
		Series:         series,
		RegularCredits: credits,
		SeasonDetails:  seasonDetails,
	}, nil
}
