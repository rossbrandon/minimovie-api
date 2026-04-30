package achievements

import (
	"context"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
)

var (
	statusWatched = "watched"
	typeMovie     = "movie"
	typeEpisode   = "episode"
)

// checkOpeningCredits awards when a user has watched their first movie.
func checkOpeningCredits(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	items, err := wl.List(ctx, userID, &statusWatched, &typeMovie)
	if err != nil || len(items) == 0 {
		return false, "", 0, ""
	}
	return true, items[0].MediaType, items[0].MediaID, items[0].MediaTitle
}

// checkSideQuestStarted awards when a user has watched their first episode of a series.
func checkSideQuestStarted(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	events, err := we.List(ctx, userID, store.WatchEventFilters{MediaType: &typeEpisode, Limit: 1})
	if err != nil || len(events) == 0 {
		return false, "", 0, ""
	}
	mt, mid, title := resolveSeriesContext(events[0])
	return true, mt, mid, title
}

// checkCenturyClub awards when a user has watched 100+ movies.
func checkCenturyClub(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	items, err := wl.List(ctx, userID, &statusWatched, &typeMovie)
	if err != nil {
		return false, "", 0, ""
	}
	if len(items) >= 100 {
		return true, items[0].MediaType, items[0].MediaID, items[0].MediaTitle
	}
	return false, "", 0, ""
}

// checkGenreExplorerBronze awards when a user has watched content across 5+ distinct genres.
func checkGenreExplorerBronze(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	return checkGenreExplorer(ctx, userID, we, 5)
}

// checkGenreExplorerSilver awards when a user has watched content across 10+ distinct genres.
func checkGenreExplorerSilver(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	return checkGenreExplorer(ctx, userID, we, 10)
}

// checkGenreExplorerGold awards when a user has watched content across 15+ distinct genres.
func checkGenreExplorerGold(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	return checkGenreExplorer(ctx, userID, we, 15)
}

// checkGenreExplorer is the shared logic for genre explorer tiers. It collects
// all unique genre IDs from a user's watch events and checks against the threshold.
func checkGenreExplorer(ctx context.Context, userID string, we store.WatchEventRepository, threshold int) (bool, string, int, string) {
	events, err := we.List(ctx, userID, store.WatchEventFilters{Limit: 500})
	if err != nil || len(events) == 0 {
		return false, "", 0, ""
	}

	genres := make(map[string]bool)
	for _, ev := range events {
		for _, g := range ev.Genres {
			genres[g] = true
		}
	}

	if len(genres) >= threshold {
		mt, mid, title := resolveSeriesContext(events[0])
		return true, mt, mid, title
	}
	return false, "", 0, ""
}

// checkCriticsPick awards when a user has watched 10+ movies rated 8.0 or higher.
func checkCriticsPick(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	items, err := wl.List(ctx, userID, &statusWatched, &typeMovie)
	if err != nil {
		return false, "", 0, ""
	}

	count := 0
	for _, item := range items {
		if item.VoteAverage != nil && *item.VoteAverage >= 8.0 {
			count++
		}
	}

	if count >= 10 {
		return true, items[0].MediaType, items[0].MediaID, items[0].MediaTitle
	}
	return false, "", 0, ""
}

// checkTimeTraveler awards when a user has watched content spanning 5+ different decades.
func checkTimeTraveler(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	items, err := wl.List(ctx, userID, &statusWatched, nil)
	if err != nil {
		return false, "", 0, ""
	}

	decades := make(map[int]bool)
	for _, item := range items {
		if item.ReleaseYear != nil {
			decade := (*item.ReleaseYear / 10) * 10
			decades[decade] = true
		}
	}

	if len(decades) >= 5 && len(items) > 0 {
		return true, items[0].MediaType, items[0].MediaID, items[0].MediaTitle
	}
	return false, "", 0, ""
}

// checkMarathonRunner awards when a user has watched 5+ episodes in a single calendar day.
// Uses the event's timezone to determine the local day.
func checkMarathonRunner(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	events, err := we.List(ctx, userID, store.WatchEventFilters{DatedOnly: true, Limit: 500})
	if err != nil {
		return false, "", 0, ""
	}

	dayCounts := make(map[string]int)
	dayEvents := make(map[string]store.WatchEvent)
	for _, ev := range events {
		if ev.WatchedAt != nil && ev.MediaType == "episode" {
			loc := loadLocation(ev.Timezone)
			day := ev.WatchedAt.In(loc).Format("2006-01-02")
			dayCounts[day]++
			dayEvents[day] = ev
		}
	}

	for day, count := range dayCounts {
		if count >= 5 {
			ev := dayEvents[day]
			mt, mid, title := resolveSeriesContext(ev)
			return true, mt, mid, title
		}
	}
	return false, "", 0, ""
}

// checkNightOwl awards when a user has logged a watch event between midnight and 5 AM local time.
func checkNightOwl(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	return checkTimeOfDay(ctx, userID, we, 0, 5)
}

// checkEarlyBird awards when a user has logged a watch event between 5 AM and 7 AM local time.
func checkEarlyBird(ctx context.Context, userID string, wl store.WatchlistRepository, we store.WatchEventRepository) (bool, string, int, string) {
	return checkTimeOfDay(ctx, userID, we, 5, 7)
}

// checkTimeOfDay is the shared logic for time-based achievements (NightOwl, EarlyBird).
// It looks for any watch event whose local hour falls within [startHour, endHour).
func checkTimeOfDay(ctx context.Context, userID string, we store.WatchEventRepository, startHour, endHour int) (bool, string, int, string) {
	events, err := we.List(ctx, userID, store.WatchEventFilters{DatedOnly: true, Limit: 500})
	if err != nil {
		return false, "", 0, ""
	}

	for _, ev := range events {
		if ev.WatchedAt != nil {
			loc := loadLocation(ev.Timezone)
			hour := ev.WatchedAt.In(loc).Hour()
			if hour >= startHour && hour < endHour {
				mt, mid, title := resolveSeriesContext(ev)
				return true, mt, mid, title
			}
		}
	}
	return false, "", 0, ""
}

// resolveSeriesContext returns (mediaType, mediaID, title) suitable for storage
// in user_achievement. For episode/season events, it resolves to the parent series.
func resolveSeriesContext(ev store.WatchEvent) (string, int, string) {
	if (ev.MediaType == "episode" || ev.MediaType == "season") && ev.SeriesID != nil {
		title := ev.MediaTitle
		if ev.SeriesTitle != nil {
			title = *ev.SeriesTitle
		}
		return "series", *ev.SeriesID, title
	}
	return ev.MediaType, ev.MediaID, ev.MediaTitle
}

// loadLocation resolves a timezone name to a *time.Location, falling back to UTC
// if the name is empty or invalid.
func loadLocation(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}
