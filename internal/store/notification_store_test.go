package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationSeenStore_MarkSeen(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewNotificationSeenStore(testPool)

	first, err := s.MarkSeen(ctx, "jti-abc", "google")
	require.NoError(t, err)
	assert.True(t, first)

	second, err := s.MarkSeen(ctx, "jti-abc", "google")
	require.NoError(t, err)
	assert.False(t, second)
}

func TestNotificationSeenStore_DeleteExpired(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	s := NewNotificationSeenStore(testPool)

	_, err := testPool.Exec(ctx,
		`INSERT INTO provider_notifications_seen (jti, provider, received_at)
		 VALUES ('old-jti', 'google', $1)`,
		time.Now().Add(-31*24*time.Hour),
	)
	require.NoError(t, err)

	_, err = s.MarkSeen(ctx, "fresh-jti", "google")
	require.NoError(t, err)

	deleted, err := s.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	fresh, err := s.MarkSeen(ctx, "fresh-jti", "google")
	require.NoError(t, err)
	assert.False(t, fresh)
}
