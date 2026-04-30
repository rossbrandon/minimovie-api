package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAchievementStore_CreateAndList(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewAchievementStore(testPool)

	err := s.Create(ctx, userID, "first_watch", "movie", 1, "Fight Club")
	require.NoError(t, err)

	err = s.Create(ctx, userID, "binge_watcher", "movie", 2, "Inception")
	require.NoError(t, err)

	list, err := s.ListByUserID(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestAchievementStore_Exists(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewAchievementStore(testPool)

	err := s.Create(ctx, userID, "early_bird", "movie", 1, "Fight Club")
	require.NoError(t, err)

	exists, err := s.Exists(ctx, userID, "early_bird")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = s.Exists(ctx, userID, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestAchievementStore_Unseen(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewAchievementStore(testPool)

	err := s.Create(ctx, userID, "ach_1", "movie", 1, "Fight Club")
	require.NoError(t, err)
	err = s.Create(ctx, userID, "ach_2", "movie", 2, "Inception")
	require.NoError(t, err)

	unseen, err := s.ListUnseen(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, unseen, 2)

	count, err := s.CountUnseen(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	err = s.MarkAllSeen(ctx, userID)
	require.NoError(t, err)

	unseen, err = s.ListUnseen(ctx, userID)
	require.NoError(t, err)
	assert.Empty(t, unseen)

	count, err = s.CountUnseen(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestAchievementStore_UniqueConstraint(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewAchievementStore(testPool)

	err := s.Create(ctx, userID, "duplicate_ach", "movie", 1, "Fight Club")
	require.NoError(t, err)

	err = s.Create(ctx, userID, "duplicate_ach", "movie", 1, "Fight Club")
	require.NoError(t, err)

	list, err := s.ListByUserID(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
