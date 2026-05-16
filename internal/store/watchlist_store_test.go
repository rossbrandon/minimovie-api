package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchlistStore_Create(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchlistStore(testPool)

	runtime := 120
	vote := float32(8.5)
	year := 2024
	poster := "/poster.jpg"
	meta := ResolvedMedia{
		Title:          "Test Movie",
		PosterPath:     &poster,
		Genres:         []string{"Action", "Adventure"},
		RuntimeMinutes: &runtime,
		VoteAverage:    &vote,
		ReleaseYear:    &year,
	}

	item, err := s.Create(ctx, uuid.New().String(), userID, "movie", 100, "want_to_watch", meta)
	require.NoError(t, err)
	assert.NotEmpty(t, item.ID)
	assert.Equal(t, "movie", item.MediaType)
	assert.Equal(t, 100, item.MediaID)
	assert.Equal(t, "Test Movie", item.MediaTitle)
	assert.Equal(t, "want_to_watch", item.Status)
	assert.Equal(t, 0, item.WatchCount)
	assert.Equal(t, []string{"Action", "Adventure"}, item.Genres)
	require.NotNil(t, item.RuntimeMinutes)
	assert.Equal(t, 120, *item.RuntimeMinutes)
}

func TestWatchlistStore_List(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchlistStore(testPool)

	_, err := s.Create(ctx, uuid.New().String(), userID, "movie", 1, "want_to_watch", ResolvedMedia{Title: "Movie 1", Genres: []string{}})
	require.NoError(t, err)
	_, err = s.Create(ctx, uuid.New().String(), userID, "series", 2, "watched", ResolvedMedia{Title: "Series 1", Genres: []string{}})
	require.NoError(t, err)

	all, err := s.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	statusFilter := "watched"
	filtered, err := s.List(ctx, userID, &statusFilter, nil)
	require.NoError(t, err)
	assert.Len(t, filtered, 1)
	assert.Equal(t, "Series 1", filtered[0].MediaTitle)

	mtFilter := "movie"
	byType, err := s.List(ctx, userID, nil, &mtFilter)
	require.NoError(t, err)
	assert.Len(t, byType, 1)
	assert.Equal(t, "movie", byType[0].MediaType)
}

func TestWatchlistStore_Check(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchlistStore(testPool)

	_, err := s.Create(ctx, uuid.New().String(), userID, "movie", 42, "want_to_watch", ResolvedMedia{Title: "Exists", Genres: []string{}})
	require.NoError(t, err)

	found, err := s.Check(ctx, userID, "movie", 42)
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "want_to_watch", found.Status)

	notFound, err := s.Check(ctx, userID, "movie", 999)
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestWatchlistStore_UpdateStatus(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchlistStore(testPool)

	item, err := s.Create(ctx, uuid.New().String(), userID, "movie", 50, "want_to_watch", ResolvedMedia{Title: "Status Test", Genres: []string{}})
	require.NoError(t, err)

	updated, err := s.UpdateStatus(ctx, item.ID, userID, "watched")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "watched", updated.Status)
}

func TestWatchlistStore_UpdateSummary(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	ws := NewWatchlistStore(testPool)
	es := NewWatchEventStore(testPool)

	_, err := ws.Create(ctx, uuid.New().String(), userID, "movie", 200, "want_to_watch", ResolvedMedia{Title: "Summary Movie", Genres: []string{}})
	require.NoError(t, err)

	_, err = es.Create(ctx, WatchEventCreate{
		UserID:    userID,
		MediaType: "movie",
		MediaID:   200,
		Timezone:  "UTC",
		Meta:      ResolvedMedia{Title: "Summary Movie", Genres: []string{}},
	})
	require.NoError(t, err)

	err = ws.UpdateSummary(ctx, userID, "movie", 200)
	require.NoError(t, err)

	items, err := ws.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1, items[0].WatchCount)
	assert.Equal(t, "watched", items[0].Status)
}

func TestWatchlistStore_Delete(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	otherUserID := createOtherUser(t)
	s := NewWatchlistStore(testPool)

	item, err := s.Create(ctx, uuid.New().String(), userID, "movie", 60, "want_to_watch", ResolvedMedia{Title: "Delete Me", Genres: []string{}})
	require.NoError(t, err)

	err = s.Delete(ctx, item.ID, userID)
	require.NoError(t, err)

	err = s.Delete(ctx, item.ID, otherUserID)
	assert.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestWatchlistStore_UniqueConstraint(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchlistStore(testPool)

	_, err := s.Create(ctx, uuid.New().String(), userID, "movie", 77, "want_to_watch", ResolvedMedia{Title: "Unique", Genres: []string{}})
	require.NoError(t, err)

	_, err = s.Create(ctx, uuid.New().String(), userID, "movie", 77, "watching", ResolvedMedia{Title: "Unique Dup", Genres: []string{}})
	assert.Error(t, err)
}

// UpdateSummary recomputes episodes_watched and seasons_watched from the
// user's watch_event rows for a series. Aggregation is SUM(season
// episode_count) + COUNT(episode events), distinct season-numbers touched
// for seasons_watched.
// seasons_watched counts only seasons the user has fully completed — via
// season mark OR enough episode events to meet the season's total.
func TestWatchlistStore_UpdateSummary_SeriesAggregation(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)
	we := NewWatchEventStore(testPool)
	sm := NewSeriesMetadataStore(testPool)

	seriesID := 5000
	require.NoError(t, sm.Upsert(ctx, &SeriesMetadata{
		SeriesID:            seriesID,
		Name:                "Show",
		TotalEpisodes:       30,
		TotalSeasons:        3,
		SeasonEpisodeCounts: map[int]int{1: 10, 2: 10, 3: 10},
	}))
	_, err := wl.Create(ctx, uuid.New().String(), userID, "series", seriesID, "want_to_watch", ResolvedMedia{Title: "Show", Genres: []string{}})
	require.NoError(t, err)

	// Season 1 marked — one complete season.
	seasonOne := 1
	episodeCount := 10
	_, err = we.MarkSeason(ctx, WatchEventCreate{
		UserID: userID, MediaType: "season", MediaID: 6001,
		SeriesID: &seriesID, SeasonNumber: &seasonOne, EpisodeCount: &episodeCount,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "S1", Genres: []string{}},
	})
	require.NoError(t, err)

	// One episode in season 2 — not enough to complete.
	seasonTwo := 2
	ep := 1
	_, err = we.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "episode", MediaID: 7001,
		SeriesID: &seriesID, SeasonNumber: &seasonTwo, EpisodeNumber: &ep,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "S2E1", Genres: []string{}},
	})
	require.NoError(t, err)

	require.NoError(t, wl.UpdateSummary(ctx, userID, "series", seriesID))

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 11, items[0].EpisodesWatched, "10 from season mark + 1 episode event")
	assert.Equal(t, 1, items[0].SeasonsWatched, "season 1 complete; season 2 only 1/10")
	assert.Equal(t, "in_progress", items[0].Status, "11/30 episodes — not yet watched")
}

// A season counts as complete when episode marks ≥ its cached episode_count,
// even without an explicit season mark.
func TestWatchlistStore_UpdateSummary_AllEpisodesMakeSeasonComplete(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)
	we := NewWatchEventStore(testPool)
	sm := NewSeriesMetadataStore(testPool)

	seriesID := 5100
	require.NoError(t, sm.Upsert(ctx, &SeriesMetadata{
		SeriesID:            seriesID,
		Name:                "Mini",
		TotalEpisodes:       3,
		TotalSeasons:        1,
		SeasonEpisodeCounts: map[int]int{1: 3},
	}))
	_, err := wl.Create(ctx, uuid.New().String(), userID, "series", seriesID, "want_to_watch", ResolvedMedia{Title: "Mini", Genres: []string{}})
	require.NoError(t, err)

	seasonOne := 1
	for ep := 1; ep <= 3; ep++ {
		episode := ep
		mediaID := 8000 + ep
		_, err := we.Create(ctx, WatchEventCreate{
			UserID: userID, MediaType: "episode", MediaID: mediaID,
			SeriesID: &seriesID, SeasonNumber: &seasonOne, EpisodeNumber: &episode,
			Timezone: "UTC", Meta: ResolvedMedia{Title: "ep", Genres: []string{}},
		})
		require.NoError(t, err)
	}

	require.NoError(t, wl.UpdateSummary(ctx, userID, "series", seriesID))

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 3, items[0].EpisodesWatched)
	assert.Equal(t, 1, items[0].SeasonsWatched, "3/3 episode marks completes the season")
}

// Without cached series_metadata, episode marks can't complete a season —
// we don't know the totals. Season marks still count.
func TestWatchlistStore_UpdateSummary_NoMetadataCannotCompleteViaEpisodes(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)
	we := NewWatchEventStore(testPool)

	seriesID := 5200
	_, err := wl.Create(ctx, uuid.New().String(), userID, "series", seriesID, "want_to_watch", ResolvedMedia{Title: "Cold", Genres: []string{}})
	require.NoError(t, err)

	seasonOne := 1
	for ep := 1; ep <= 5; ep++ {
		episode := ep
		mediaID := 9000 + ep
		_, err := we.Create(ctx, WatchEventCreate{
			UserID: userID, MediaType: "episode", MediaID: mediaID,
			SeriesID: &seriesID, SeasonNumber: &seasonOne, EpisodeNumber: &episode,
			Timezone: "UTC", Meta: ResolvedMedia{Title: "ep", Genres: []string{}},
		})
		require.NoError(t, err)
	}

	require.NoError(t, wl.UpdateSummary(ctx, userID, "series", seriesID))

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 5, items[0].EpisodesWatched)
	assert.Equal(t, 0, items[0].SeasonsWatched, "no cached season totals → can't complete via episode marks")
}

// Full unwatch reverts status to 'want_to_watch'. Without this branch a series
// would stay 'watched' even after all events were removed.
func TestWatchlistStore_UpdateSummary_StatusRevertsOnFullUnwatch(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)
	we := NewWatchEventStore(testPool)

	seriesID := 5500
	_, err := wl.Create(ctx, uuid.New().String(), userID, "series", seriesID, "want_to_watch", ResolvedMedia{Title: "Show2", Genres: []string{}})
	require.NoError(t, err)

	season := 1
	episodeCount := 5
	ev, err := we.MarkSeason(ctx, WatchEventCreate{
		UserID: userID, MediaType: "season", MediaID: 6010,
		SeriesID: &seriesID, SeasonNumber: &season, EpisodeCount: &episodeCount,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "S1", Genres: []string{}},
	})
	require.NoError(t, err)
	require.NoError(t, wl.UpdateSummary(ctx, userID, "series", seriesID))

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "in_progress", items[0].Status, "no series_metadata → in_progress for safety")

	require.NoError(t, we.Delete(ctx, ev.ID, userID))
	require.NoError(t, wl.UpdateSummary(ctx, userID, "series", seriesID))

	items, err = wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "want_to_watch", items[0].Status, "no events → revert to want_to_watch")
	assert.Equal(t, 0, items[0].EpisodesWatched)
	assert.Equal(t, 0, items[0].SeasonsWatched)
}

// List sorts by most recent interaction: COALESCE(last_watched_at, added_at)
// DESC. Marking a long-dormant want-to-watch item should float it above
// items added after it.
func TestWatchlistStore_List_SortsByMostRecentInteraction(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)
	we := NewWatchEventStore(testPool)

	// Item A: created earliest.
	_, err := wl.Create(ctx, uuid.New().String(), userID, "movie", 11, "want_to_watch", ResolvedMedia{Title: "Old", Genres: []string{}})
	require.NoError(t, err)
	// Item B: created later, still want_to_watch.
	_, err = wl.Create(ctx, uuid.New().String(), userID, "movie", 22, "want_to_watch", ResolvedMedia{Title: "Newer", Genres: []string{}})
	require.NoError(t, err)

	// Mark item A as watched — its last_watched_at advances past item B's added_at.
	_, err = we.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "movie", MediaID: 11,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "Old", Genres: []string{}},
	})
	require.NoError(t, err)
	require.NoError(t, wl.UpdateSummary(ctx, userID, "movie", 11))

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, 11, items[0].MediaID, "just-watched item floats to the top")
	assert.Equal(t, 22, items[1].MediaID)
}

// JOIN to series_metadata: series rows without a cached entry get NULL
// totals (not an error). The frontend treats null totals as "no progress
// badge yet", which is the correct degraded state.
func TestWatchlistStore_List_JoinNullForUncachedSeries(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)

	_, err := wl.Create(ctx, uuid.New().String(), userID, "series", 6000, "want_to_watch", ResolvedMedia{Title: "Uncached", Genres: []string{}})
	require.NoError(t, err)

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Nil(t, items[0].TotalEpisodes, "no series_metadata row → null totals, no error")
	assert.Nil(t, items[0].TotalSeasons)

	// Add the metadata; List now picks it up.
	sm := NewSeriesMetadataStore(testPool)
	require.NoError(t, sm.Upsert(ctx, &SeriesMetadata{SeriesID: 6000, TotalEpisodes: 30, TotalSeasons: 3}))

	items, err = wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].TotalEpisodes)
	assert.Equal(t, 30, *items[0].TotalEpisodes)
	require.NotNil(t, items[0].TotalSeasons)
	assert.Equal(t, 3, *items[0].TotalSeasons)

	// Movie rows leave the JOIN columns NULL even when a series row of the
	// same id exists in series_metadata.
	_, err = wl.Create(ctx, uuid.New().String(), userID, "movie", 6000, "want_to_watch", ResolvedMedia{Title: "Movie", Genres: []string{}})
	require.NoError(t, err)

	items, err = wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, item := range items {
		if item.MediaType == "movie" {
			assert.Nil(t, item.TotalEpisodes, "movies never JOIN to series_metadata")
		}
	}
}

// A fully airing series with all known episodes watched stays in_progress —
// the show isn't done yet, so the user isn't either.
func TestWatchlistStore_UpdateSummary_CaughtUpOnAiringSeriesStaysInProgress(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)
	we := NewWatchEventStore(testPool)
	sm := NewSeriesMetadataStore(testPool)

	seriesID := 5300
	require.NoError(t, sm.Upsert(ctx, &SeriesMetadata{
		SeriesID:            seriesID,
		Name:                "Airing",
		TotalEpisodes:       5,
		TotalSeasons:        1,
		SeasonEpisodeCounts: map[int]int{1: 5},
		InProduction:        true,
	}))
	_, err := wl.Create(ctx, uuid.New().String(), userID, "series", seriesID, "want_to_watch", ResolvedMedia{Title: "Airing", Genres: []string{}})
	require.NoError(t, err)

	season := 1
	episodeCount := 5
	_, err = we.MarkSeason(ctx, WatchEventCreate{
		UserID: userID, MediaType: "season", MediaID: 6300,
		SeriesID: &seriesID, SeasonNumber: &season, EpisodeCount: &episodeCount,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "S1", Genres: []string{}},
	})
	require.NoError(t, err)
	require.NoError(t, wl.UpdateSummary(ctx, userID, "series", seriesID))

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "in_progress", items[0].Status)
	assert.Nil(t, items[0].FinishedAt, "still airing → no finish date")
	require.NotNil(t, items[0].StartedAt)
}

// A finished series fully watched flips to 'watched' and sets finished_at.
func TestWatchlistStore_UpdateSummary_FinishedSeriesWatchedSetsDates(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)
	we := NewWatchEventStore(testPool)
	sm := NewSeriesMetadataStore(testPool)

	seriesID := 5400
	require.NoError(t, sm.Upsert(ctx, &SeriesMetadata{
		SeriesID:            seriesID,
		Name:                "Done",
		TotalEpisodes:       3,
		TotalSeasons:        1,
		SeasonEpisodeCounts: map[int]int{1: 3},
		InProduction:        false,
	}))
	_, err := wl.Create(ctx, uuid.New().String(), userID, "series", seriesID, "want_to_watch", ResolvedMedia{Title: "Done", Genres: []string{}})
	require.NoError(t, err)

	season := 1
	episodeCount := 3
	_, err = we.MarkSeason(ctx, WatchEventCreate{
		UserID: userID, MediaType: "season", MediaID: 6400,
		SeriesID: &seriesID, SeasonNumber: &season, EpisodeCount: &episodeCount,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "S1", Genres: []string{}},
	})
	require.NoError(t, err)
	require.NoError(t, wl.UpdateSummary(ctx, userID, "series", seriesID))

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "watched", items[0].Status)
	require.NotNil(t, items[0].StartedAt)
	require.NotNil(t, items[0].FinishedAt)
}

// Movie watch_event sets started_at and finished_at to the same instant.
// Removing the event clears both back to null and reverts status.
func TestWatchlistStore_UpdateSummary_MovieSetsAndClearsDates(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)
	we := NewWatchEventStore(testPool)

	_, err := wl.Create(ctx, uuid.New().String(), userID, "movie", 300, "want_to_watch", ResolvedMedia{Title: "Flick", Genres: []string{}})
	require.NoError(t, err)

	ev, err := we.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "movie", MediaID: 300,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "Flick", Genres: []string{}},
	})
	require.NoError(t, err)
	require.NoError(t, wl.UpdateSummary(ctx, userID, "movie", 300))

	items, err := wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "watched", items[0].Status)
	require.NotNil(t, items[0].StartedAt)
	require.NotNil(t, items[0].FinishedAt)
	assert.Equal(t, *items[0].StartedAt, *items[0].FinishedAt, "movie started=finished")

	require.NoError(t, we.Delete(ctx, ev.ID, userID))
	require.NoError(t, wl.UpdateSummary(ctx, userID, "movie", 300))

	items, err = wl.List(ctx, userID, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "want_to_watch", items[0].Status)
	assert.Nil(t, items[0].StartedAt)
	assert.Nil(t, items[0].FinishedAt)
}

// UpdateStatus is the explicit-toggle path; a user marking 'watched' with no
// events must still get finished_at populated so the UI date label can render.
func TestWatchlistStore_UpdateStatus_HandMarkedWatchedSetsDates(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	wl := NewWatchlistStore(testPool)

	item, err := wl.Create(ctx, uuid.New().String(), userID, "movie", 400, "want_to_watch", ResolvedMedia{Title: "Hand", Genres: []string{}})
	require.NoError(t, err)
	assert.Nil(t, item.StartedAt)
	assert.Nil(t, item.FinishedAt)

	updated, err := wl.UpdateStatus(ctx, item.ID, userID, "watched")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "watched", updated.Status)
	require.NotNil(t, updated.StartedAt)
	require.NotNil(t, updated.FinishedAt)

	reverted, err := wl.UpdateStatus(ctx, item.ID, userID, "want_to_watch")
	require.NoError(t, err)
	require.NotNil(t, reverted)
	assert.Equal(t, "want_to_watch", reverted.Status)
	assert.Nil(t, reverted.FinishedAt, "revert clears finished_at")
}

func createOtherUser(t *testing.T) string {
	t.Helper()
	s := NewUserStore(testPool)
	result, err := s.UpsertFromOAuth(context.Background(), "google", "other-provider-id-"+t.Name(), "", nil, nil)
	require.NoError(t, err)
	return result.User.ID
}
