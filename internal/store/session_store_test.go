package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_CreateAndGet(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewSessionStore(testPool)

	sess, err := s.Create(ctx, "token-hash-1", userID)
	require.NoError(t, err)
	assert.Equal(t, userID, sess.UserID)
	assert.Equal(t, "token-hash-1", sess.TokenHash)

	sw, err := s.GetByTokenHash(ctx, "token-hash-1")
	require.NoError(t, err)
	require.NotNil(t, sw)
	assert.Equal(t, sess.ID, sw.Session.ID)
	assert.Equal(t, userID, sw.User.ID)
}

func TestSessionStore_GetByTokenHash_Expired(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)

	_, err := testPool.Exec(ctx,
		`INSERT INTO sessions (id, token_hash, user_id, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		uuid.New().String(), "expired-hash", userID, time.Now().Add(-1*time.Hour),
	)
	require.NoError(t, err)

	s := NewSessionStore(testPool)
	sw, err := s.GetByTokenHash(ctx, "expired-hash")
	require.NoError(t, err)
	assert.Nil(t, sw)
}

func TestSessionStore_Delete(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewSessionStore(testPool)

	_, err := s.Create(ctx, "to-delete", userID)
	require.NoError(t, err)

	err = s.Delete(ctx, "to-delete")
	require.NoError(t, err)

	sw, err := s.GetByTokenHash(ctx, "to-delete")
	require.NoError(t, err)
	assert.Nil(t, sw)
}

func TestSessionStore_DeleteByUserID(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewSessionStore(testPool)

	_, err := s.Create(ctx, "sess-a", userID)
	require.NoError(t, err)
	_, err = s.Create(ctx, "sess-b", userID)
	require.NoError(t, err)

	err = s.DeleteByUserID(ctx, userID)
	require.NoError(t, err)

	swA, err := s.GetByTokenHash(ctx, "sess-a")
	require.NoError(t, err)
	assert.Nil(t, swA)

	swB, err := s.GetByTokenHash(ctx, "sess-b")
	require.NoError(t, err)
	assert.Nil(t, swB)
}

func TestSessionStore_DeleteExpired(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewSessionStore(testPool)

	_, err := testPool.Exec(ctx,
		`INSERT INTO sessions (id, token_hash, user_id, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		uuid.New().String(), "old-hash", userID, time.Now().Add(-24*time.Hour),
	)
	require.NoError(t, err)

	_, err = s.Create(ctx, "fresh-hash", userID)
	require.NoError(t, err)

	deleted, err := s.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	sw, err := s.GetByTokenHash(ctx, "fresh-hash")
	require.NoError(t, err)
	assert.NotNil(t, sw)
}
