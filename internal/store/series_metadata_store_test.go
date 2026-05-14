package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeriesMetadataStore_UpsertAndGet(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewSeriesMetadataStore(testPool)

	got, err := s.Get(ctx, 1234)
	require.NoError(t, err)
	assert.Nil(t, got, "missing row returns nil, nil")

	require.NoError(t, s.Upsert(ctx, &SeriesMetadata{
		SeriesID:      1234,
		Name:          "Breaking Bad",
		TotalEpisodes: 10,
		TotalSeasons:  1,
		InProduction:  false,
		Status:        "Ended",
		LastAirDate:   "2013-09-29",
	}))

	first, err := s.Get(ctx, 1234)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "Breaking Bad", first.Name)
	assert.Equal(t, 10, first.TotalEpisodes)
	assert.Equal(t, 1, first.TotalSeasons)
	assert.Equal(t, "Ended", first.Status)
	assert.Equal(t, "2013-09-29", first.LastAirDate)
	assert.True(t, first.Fetched)

	require.NoError(t, s.Upsert(ctx, &SeriesMetadata{
		SeriesID:      1234,
		Name:          "Breaking Bad",
		TotalEpisodes: 12,
		TotalSeasons:  2,
		InProduction:  true,
		Status:        "Returning Series",
		NextAirDate:   "2026-06-01",
	}))
	second, err := s.Get(ctx, 1234)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 12, second.TotalEpisodes)
	assert.Equal(t, 2, second.TotalSeasons)
	assert.True(t, second.InProduction)
	assert.Equal(t, "Returning Series", second.Status)
	assert.Equal(t, "2026-06-01", second.NextAirDate)
	assert.Equal(t, "", second.LastAirDate, "upsert clears last_air_date when omitted")
	assert.True(t, !second.FetchedAt.Before(first.FetchedAt),
		"fetched_at should refresh on upsert")
}
