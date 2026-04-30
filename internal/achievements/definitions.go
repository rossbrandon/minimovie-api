package achievements

import (
	"context"

	"github.com/rossbrandon/minimovie-api/internal/store"
)

type Definition struct {
	ID          string
	Name        string
	Description string
	Check       CheckFunc
}

type CheckFunc func(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (earned bool, mediaType string, mediaID int, mediaTitle string)

var definitionsByID map[string]Definition

func init() {
	definitionsByID = make(map[string]Definition, len(AllDefinitions))
	for _, d := range AllDefinitions {
		definitionsByID[d.ID] = d
	}
}

func GetDefinition(id string) (Definition, bool) {
	d, ok := definitionsByID[id]
	return d, ok
}

var AllDefinitions = []Definition{
	{
		ID:          "opening_credits",
		Name:        "Opening Credits",
		Description: "Watch your first movie",
		Check:       checkOpeningCredits,
	},
	{
		ID:          "side_quest_started",
		Name:        "Side Quest Started",
		Description: "Watch your first episode of a series",
		Check:       checkSideQuestStarted,
	},
	{
		ID:          "century_club",
		Name:        "Century Club",
		Description: "Watch 100 movies",
		Check:       checkCenturyClub,
	},
	{
		ID:          "genre_explorer:bronze",
		Name:        "Genre Explorer (Bronze)",
		Description: "Watch from 5 distinct genres",
		Check:       checkGenreExplorerBronze,
	},
	{
		ID:          "genre_explorer:silver",
		Name:        "Genre Explorer (Silver)",
		Description: "Watch from 10 distinct genres",
		Check:       checkGenreExplorerSilver,
	},
	{
		ID:          "genre_explorer:gold",
		Name:        "Genre Explorer (Gold)",
		Description: "Watch from 15 distinct genres",
		Check:       checkGenreExplorerGold,
	},
	{
		ID:          "critics_pick",
		Name:        "Critic's Pick",
		Description: "Watch 10+ movies rated 8.0+ on TMDB",
		Check:       checkCriticsPick,
	},
	{
		ID:          "time_traveler",
		Name:        "Time Traveler",
		Description: "Watch movies from 5+ different decades",
		Check:       checkTimeTraveler,
	},
	{
		ID:          "marathon_runner",
		Name:        "Marathon Runner",
		Description: "Mark 5+ episodes watched in a single day",
		Check:       checkMarathonRunner,
	},
	{
		ID:          "night_owl",
		Name:        "Night Owl",
		Description: "Mark something watched between midnight and 5am",
		Check:       checkNightOwl,
	},
	{
		ID:          "early_bird",
		Name:        "Early Bird",
		Description: "Mark something watched between 5am and 7am",
		Check:       checkEarlyBird,
	},
}
