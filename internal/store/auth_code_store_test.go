package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testEncryptionKey = []byte("test-encryption-key-32-bytes!!")

func init() {
	if len(testEncryptionKey) < 32 {
		padded := make([]byte, 32)
		copy(padded, testEncryptionKey)
		testEncryptionKey = padded
	}
	testEncryptionKey = testEncryptionKey[:32]
}

func TestAuthCodeStore_CreateAndExchange(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewAuthCodeStore(testPool, testEncryptionKey)

	id, err := s.Create(ctx, "token-hash-ac", userID, "my-raw-session-token")
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	ac, err := s.Exchange(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, ac)
	assert.Equal(t, id, ac.ID)
	assert.Equal(t, "token-hash-ac", ac.TokenHash)
	assert.Equal(t, userID, ac.UserID)
	assert.Equal(t, "my-raw-session-token", ac.RawSessionToken)
}

func TestAuthCodeStore_DoubleExchange(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewAuthCodeStore(testPool, testEncryptionKey)

	id, err := s.Create(ctx, "token-hash-double", userID, "session-token")
	require.NoError(t, err)

	_, err = s.Exchange(ctx, id)
	require.NoError(t, err)

	second, err := s.Exchange(ctx, id)
	require.NoError(t, err)
	assert.Nil(t, second)
}

func TestAuthCodeStore_DeleteExpired(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	userID := createTestUser(t)
	s := NewAuthCodeStore(testPool, testEncryptionKey)

	_, err := testPool.Exec(ctx,
		`INSERT INTO auth_code (id, token_hash, encrypted_session_token, user_id, expires_at)
		 VALUES ('expired-ac', 'hash', 'enc', $1, $2)`,
		userID, time.Now().Add(-1*time.Hour),
	)
	require.NoError(t, err)

	freshID, err := s.Create(ctx, "fresh-hash", userID, "fresh-token")
	require.NoError(t, err)

	deleted, err := s.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	ac, err := s.Exchange(ctx, freshID)
	require.NoError(t, err)
	assert.NotNil(t, ac)
}
