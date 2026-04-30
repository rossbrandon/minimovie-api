package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsStore_GetStats(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	ws := NewWatchlistStore(testPool)
	es := NewWatchEventStore(testPool)
	ss := NewStatsStore(testPool)

	_, err := ws.Create(ctx, userID, "movie", 1000, "watched", ResolvedMedia{Title: "Movie A", Genres: []string{}})
	require.NoError(t, err)
	_, err = ws.Create(ctx, userID, "series", 2000, "watched", ResolvedMedia{Title: "Series A", Genres: []string{}})
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Microsecond)
	runtime := 120
	_, err = es.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "movie", MediaID: 1000,
		WatchedAt: &now, Timezone: "UTC",
		Meta: ResolvedMedia{Title: "Movie A", Genres: []string{"Action", "Adventure"}, RuntimeMinutes: &runtime},
	})
	require.NoError(t, err)

	seriesID := 2000
	season := 1
	ep := 1
	epRuntime := 45
	_, err = es.Create(ctx, WatchEventCreate{
		UserID: userID, MediaType: "episode", MediaID: 3000,
		SeriesID: &seriesID, SeasonNumber: &season, EpisodeNumber: &ep,
		WatchedAt: &now, Timezone: "UTC",
		Meta: ResolvedMedia{Title: "Ep1", Genres: []string{"Drama"}, RuntimeMinutes: &epRuntime},
	})
	require.NoError(t, err)

	result, err := ss.GetStats(ctx, userID)
	require.NoError(t, err)

	assert.Equal(t, 1, result.TotalMoviesWatched)
	assert.Equal(t, 1, result.TotalSeriesCompleted)
	assert.Equal(t, 1, result.TotalEpisodesWatched)
	assert.Equal(t, 165, result.EstimatedMinutesWatched)
	assert.NotEmpty(t, result.GenreBreakdown)
	assert.NotEmpty(t, result.MonthlyActivity)
}

func TestStatsStore_GetStats_Empty(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	ss := NewStatsStore(testPool)

	result, err := ss.GetStats(ctx, userID)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalMoviesWatched)
	assert.Equal(t, 0, result.TotalSeriesCompleted)
	assert.Equal(t, 0, result.TotalEpisodesWatched)
	assert.Equal(t, 0, result.EstimatedMinutesWatched)
	assert.Empty(t, result.GenreBreakdown)
	assert.Empty(t, result.MonthlyActivity)
}
