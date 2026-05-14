package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchEventStore_Create(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)

	now := time.Now().UTC().Truncate(time.Microsecond)
	runtime := 90
	ev, err := s.Create(ctx, WatchEventCreate{
		UserID:    userID,
		MediaType: "movie",
		MediaID:   300,
		WatchedAt: &now,
		Timezone:  "America/Chicago",
		Meta: ResolvedMedia{
			Title:          "Event Movie",
			Genres:         []string{"Drama"},
			RuntimeMinutes: &runtime,
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, ev.ID)
	assert.Equal(t, "movie", ev.MediaType)
	assert.Equal(t, 300, ev.MediaID)
	assert.Equal(t, "Event Movie", ev.MediaTitle)
	assert.Equal(t, "America/Chicago", ev.Timezone)
	assert.Equal(t, 1, ev.RewatchNumber)
	require.NotNil(t, ev.RuntimeMinutes)
	assert.Equal(t, 90, *ev.RuntimeMinutes)
}

// Verifies the (user_id, media_type, media_id) unique constraint with the
// new UPSERT semantics: a second Create for the same tuple refreshes
// media_title, counts, and created_at on the existing row rather than
// returning an error. This is what lets a season re-mark pick up fresh
// episode counts as new content airs.
func TestWatchEventStore_Create_UpsertsOnConflict(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)

	id := "11111111-2222-3333-4444-555555555555"
	now := time.Now().UTC().Truncate(time.Microsecond)
	input := WatchEventCreate{
		ID:        id,
		UserID:    userID,
		MediaType: "movie",
		MediaID:   400,
		WatchedAt: &now,
		Timezone:  "UTC",
		Meta:      ResolvedMedia{Title: "Once", Genres: []string{}},
	}

	first, err := s.Create(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, id, first.ID)
	assert.Equal(t, 1, first.RewatchNumber)
	firstCreated := first.CreatedAt

	// Different id, same (user, media_type, media_id) — UPSERTs the existing row.
	otherInput := input
	otherInput.ID = "99999999-aaaa-bbbb-cccc-dddddddddddd"
	otherInput.Meta.Title = "Twice"
	second, err := s.Create(ctx, otherInput)
	require.NoError(t, err)
	assert.Equal(t, id, second.ID, "row id stays stable across upserts")
	assert.Equal(t, "Twice", second.MediaTitle, "title refreshes on upsert")
	assert.True(t, second.CreatedAt.After(firstCreated) || second.CreatedAt.Equal(firstCreated),
		"created_at is refreshed by the upsert")

	all, err := s.List(ctx, userID, WatchEventFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 1, "upsert must not create additional rows")
	assert.Equal(t, "Twice", all[0].MediaTitle, "upsert wins; row reflects latest values")
}

func TestWatchEventStore_List(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)

	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err := s.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "movie", MediaID: 500,
		WatchedAt: &now, Timezone: "UTC",
		Meta: ResolvedMedia{Title: "Dated Movie", Genres: []string{}},
	})
	require.NoError(t, err)

	_, err = s.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "movie", MediaID: 501,
		Timezone: "UTC",
		Meta:     ResolvedMedia{Title: "Undated Movie", Genres: []string{}},
	})
	require.NoError(t, err)

	all, err := s.List(ctx, userID, WatchEventFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 2)

	dated, err := s.List(ctx, userID, WatchEventFilters{DatedOnly: true})
	require.NoError(t, err)
	assert.Len(t, dated, 1)
	assert.Equal(t, "Dated Movie", dated[0].MediaTitle)

	limited, err := s.List(ctx, userID, WatchEventFilters{Limit: 1})
	require.NoError(t, err)
	assert.Len(t, limited, 1)
}

func TestWatchEventStore_ListBySeriesID(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)

	seriesID := 10
	season := 1
	ep1 := 1
	ep2 := 2
	seriesTitle := "Test Series"

	_, err := s.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "episode", MediaID: 601,
		SeriesID: &seriesID, SeasonNumber: &season, EpisodeNumber: &ep1,
		Timezone: "UTC",
		Meta:     ResolvedMedia{Title: "Ep1", SeriesTitle: &seriesTitle, Genres: []string{}},
	})
	require.NoError(t, err)

	_, err = s.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "episode", MediaID: 602,
		SeriesID: &seriesID, SeasonNumber: &season, EpisodeNumber: &ep2,
		Timezone: "UTC",
		Meta:     ResolvedMedia{Title: "Ep2", SeriesTitle: &seriesTitle, Genres: []string{}},
	})
	require.NoError(t, err)

	otherSeries := 99
	_, err = s.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "episode", MediaID: 700,
		SeriesID: &otherSeries, Timezone: "UTC",
		Meta: ResolvedMedia{Title: "Other", Genres: []string{}},
	})
	require.NoError(t, err)

	events, err := s.ListBySeriesID(ctx, userID, seriesID)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

// MarkSeason clears any pre-existing per-episode rows for the (user, series,
// season) tuple within the same transaction as the season-row UPSERT.
// This is the invariant the watchlist aggregation depends on: SUM of
// season episode_counts + COUNT of episode rows must never double-count.
func TestWatchEventStore_MarkSeason_ClearsPriorEpisodeRows(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)

	seriesID := 555
	seasonNumber := 1
	episodeNumber := 3

	_, err := s.Create(ctx, WatchEventCreate{
		UserID:        userID,
		MediaType:     "episode",
		MediaID:       9001,
		SeriesID:      &seriesID,
		SeasonNumber:  &seasonNumber,
		EpisodeNumber: &episodeNumber,
		Timezone:      "UTC",
		Meta:          ResolvedMedia{Title: "S1E3", Genres: []string{}},
	})
	require.NoError(t, err)

	episodeCount := 10
	_, err = s.MarkSeason(ctx, WatchEventCreate{
		UserID:       userID,
		MediaType:    "season",
		MediaID:      7700,
		SeriesID:     &seriesID,
		SeasonNumber: &seasonNumber,
		EpisodeCount: &episodeCount,
		Timezone:     "UTC",
		Meta:         ResolvedMedia{Title: "Season 1", Genres: []string{}},
	})
	require.NoError(t, err)

	events, err := s.ListBySeriesID(ctx, userID, seriesID)
	require.NoError(t, err)
	require.Len(t, events, 1, "MarkSeason must replace prior episode rows")
	assert.Equal(t, "season", events[0].MediaType)
	require.NotNil(t, events[0].EpisodeCount)
	assert.Equal(t, 10, *events[0].EpisodeCount)
}

// Re-marking a season refreshes episode_count + created_at so the cached
// totals advance as a series releases new content.
func TestWatchEventStore_MarkSeason_ReMarkRefreshesCount(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)

	seriesID := 600
	seasonNumber := 2

	mark := func(count int) *WatchEvent {
		ev, err := s.MarkSeason(ctx, WatchEventCreate{
			UserID:       userID,
			MediaType:    "season",
			MediaID:      8800,
			SeriesID:     &seriesID,
			SeasonNumber: &seasonNumber,
			EpisodeCount: &count,
			Timezone:     "UTC",
			Meta:         ResolvedMedia{Title: "Season 2", Genres: []string{}},
		})
		require.NoError(t, err)
		return ev
	}

	first := mark(8)
	require.NotNil(t, first.EpisodeCount)
	assert.Equal(t, 8, *first.EpisodeCount)

	second := mark(11)
	require.NotNil(t, second.EpisodeCount)
	assert.Equal(t, 11, *second.EpisodeCount, "re-mark UPSERTs with the new count")
	assert.Equal(t, first.ID, second.ID, "row id is stable across re-marks")
}

// Episode events created after a season mark coexist with the season row —
// the aggregation sums them additively (no overlap because the frontend
// only allows episode events for episodes the season mark doesn't cover).
func TestWatchEventStore_EpisodeAfterSeasonCoexists(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)

	seriesID := 700
	seasonNumber := 1
	episodeCount := 10
	_, err := s.MarkSeason(ctx, WatchEventCreate{
		UserID:       userID,
		MediaType:    "season",
		MediaID:      9900,
		SeriesID:     &seriesID,
		SeasonNumber: &seasonNumber,
		EpisodeCount: &episodeCount,
		Timezone:     "UTC",
		Meta:         ResolvedMedia{Title: "Season 1", Genres: []string{}},
	})
	require.NoError(t, err)

	// E11 aired after the season mark — frontend creates an episode event
	// for it.
	episodeNumber := 11
	_, err = s.Create(ctx, WatchEventCreate{
		UserID:        userID,
		MediaType:     "episode",
		MediaID:       9901,
		SeriesID:      &seriesID,
		SeasonNumber:  &seasonNumber,
		EpisodeNumber: &episodeNumber,
		Timezone:      "UTC",
		Meta:          ResolvedMedia{Title: "S1E11", Genres: []string{}},
	})
	require.NoError(t, err)

	events, err := s.ListBySeriesID(ctx, userID, seriesID)
	require.NoError(t, err)
	assert.Len(t, events, 2, "season + episode events coexist")
}

func TestWatchEventStore_Create_RejectsSeasonAndSeries(t *testing.T) {
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)
	seriesID := 100
	seasonNumber := 1

	_, err := s.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "season", MediaID: 1,
		SeriesID: &seriesID, SeasonNumber: &seasonNumber,
		Timezone: "UTC",
		Meta:     ResolvedMedia{Title: "Season", Genres: []string{}},
	})
	require.Error(t, err, "Create must reject media_type=season (must go through MarkSeason)")

	_, err = s.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "series", MediaID: 1,
		Timezone: "UTC",
		Meta:     ResolvedMedia{Title: "Series", Genres: []string{}},
	})
	require.Error(t, err, "Create must reject retired media_type=series")
}

func TestWatchEventStore_MarkSeason_RejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)
	seriesID := 100
	seasonNumber := 1
	episodeNumber := 1

	_, err := s.MarkSeason(ctx, WatchEventCreate{
		UserID: userID, MediaType: "episode", MediaID: 1,
		SeriesID: &seriesID, SeasonNumber: &seasonNumber,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "x", Genres: []string{}},
	})
	require.Error(t, err, "MarkSeason must require media_type=season")

	_, err = s.MarkSeason(ctx, WatchEventCreate{
		UserID: userID, MediaType: "season", MediaID: 1,
		Timezone: "UTC", Meta: ResolvedMedia{Title: "x", Genres: []string{}},
	})
	require.Error(t, err, "MarkSeason must require SeriesID and SeasonNumber")

	_, err = s.MarkSeason(ctx, WatchEventCreate{
		UserID: userID, MediaType: "season", MediaID: 1,
		SeriesID: &seriesID, SeasonNumber: &seasonNumber,
		EpisodeNumber: &episodeNumber,
		Timezone:      "UTC", Meta: ResolvedMedia{Title: "x", Genres: []string{}},
	})
	require.Error(t, err, "MarkSeason must reject EpisodeNumber")
}

func TestWatchEventStore_Delete(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	otherUserID := createOtherUser(t)
	s := NewWatchEventStore(testPool)

	ev, err := s.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "movie", MediaID: 800,
		Timezone: "UTC",
		Meta:     ResolvedMedia{Title: "Delete Event", Genres: []string{}},
	})
	require.NoError(t, err)

	err = s.Delete(ctx, ev.ID, userID)
	require.NoError(t, err)

	err = s.Delete(ctx, ev.ID, otherUserID)
	assert.ErrorIs(t, err, pgx.ErrNoRows)
}
