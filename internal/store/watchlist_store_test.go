package store

import (
	"context"
	"testing"

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

	item, err := s.Create(ctx, userID, "movie", 100, "want_to_watch", meta)
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

	_, err := s.Create(ctx, userID, "movie", 1, "want_to_watch", ResolvedMedia{Title: "Movie 1", Genres: []string{}})
	require.NoError(t, err)
	_, err = s.Create(ctx, userID, "series", 2, "watched", ResolvedMedia{Title: "Series 1", Genres: []string{}})
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

	_, err := s.Create(ctx, userID, "movie", 42, "want_to_watch", ResolvedMedia{Title: "Exists", Genres: []string{}})
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

	item, err := s.Create(ctx, userID, "movie", 50, "want_to_watch", ResolvedMedia{Title: "Status Test", Genres: []string{}})
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

	_, err := ws.Create(ctx, userID, "movie", 200, "want_to_watch", ResolvedMedia{Title: "Summary Movie", Genres: []string{}})
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

	item, err := s.Create(ctx, userID, "movie", 60, "want_to_watch", ResolvedMedia{Title: "Delete Me", Genres: []string{}})
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

	_, err := s.Create(ctx, userID, "movie", 77, "want_to_watch", ResolvedMedia{Title: "Unique", Genres: []string{}})
	require.NoError(t, err)

	_, err = s.Create(ctx, userID, "movie", 77, "watching", ResolvedMedia{Title: "Unique Dup", Genres: []string{}})
	assert.Error(t, err)
}

func createOtherUser(t *testing.T) string {
	t.Helper()
	s := NewUserStore(testPool)
	result, err := s.UpsertFromOAuth(context.Background(), "google", "other-provider-id-"+t.Name(), "", nil, nil)
	require.NoError(t, err)
	return result.User.ID
}
