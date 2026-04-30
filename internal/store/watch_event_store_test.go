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

func TestWatchEventStore_Create_RewatchNumber(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewWatchEventStore(testPool)

	input := WatchEventCreate{
		UserID:    userID,
		MediaType: "movie",
		MediaID:   400,
		Timezone:  "UTC",
		Meta:      ResolvedMedia{Title: "Rewatch", Genres: []string{}},
	}

	first, err := s.Create(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, 1, first.RewatchNumber)

	second, err := s.Create(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, 2, second.RewatchNumber)
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
