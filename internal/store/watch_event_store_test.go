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

// Verifies the (user_id, media_type, media_id) unique constraint collapses
// duplicate Creates: the second call returns ErrDuplicateWatchEvent and no
// second row is inserted. Mirrors the production flow where the handler
// computes a deterministic UUIDv5 from the same tuple and reuses it across
// retries — both PK and the unique constraint enforce the same invariant.
func TestWatchEventStore_Create_Duplicate(t *testing.T) {
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

	// Same explicit id — same PK, duplicate.
	_, err = s.Create(ctx, input)
	assert.ErrorIs(t, err, ErrDuplicateWatchEvent)

	// Different id, same (user, media_type, media_id) — unique constraint, duplicate.
	otherInput := input
	otherInput.ID = "99999999-aaaa-bbbb-cccc-dddddddddddd"
	otherInput.Meta.Title = "Twice"
	_, err = s.Create(ctx, otherInput)
	assert.ErrorIs(t, err, ErrDuplicateWatchEvent)

	all, err := s.List(ctx, userID, WatchEventFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 1, "duplicate inserts must not create additional rows")
	assert.Equal(t, "Once", all[0].MediaTitle, "first insert wins; second is silently skipped")
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
